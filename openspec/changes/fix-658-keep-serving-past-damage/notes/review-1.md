# Review round 1 notes

Codex's implementation review at `17d5b13070fecf27a7d6819ad0d5dd642baf18d6` asked for changes: two HIGH findings on keys
that are not literal entity IDs, and one MEDIUM on a comment and these notes. This file records the fixes, the tests
shown failing first, the wrong changes run against them, and the check of graph-lexical and graph-clustering.

## HIGH 1: graph-index records a deleted noncanonical key damaged again

A value under `legacy-key` (a key KV accepts that is not a six-part entity ID) is damaged. Deleting it cleared the
follower's record on delivery, but the lane then rejected the key before reading current state and recorded it again
at the delete's revision, which `Complete` at that same revision cannot clear.

Fix: `reconcileNoncanonical` (`processor/graph-index/index.go:220`), called from `reconcileEntity` (`:188`). It reads
the key's current state, as every other key's reconcile does; a key that is absent (deleted or purged, or an older
write handled after the delete) records nothing, and one that holds a value is recorded damaged at the read entry's
revision. A failed read marks the entity failed, as the canonical path does, and the repair loop reads it again. No
owner filter is built from the key and `deleteFromIndexes` is never called for it: no row was ever built under it.

A key the shared validator `natsclient.ValidateKVLiteralKey` refuses (longer than 1,024 bytes, more than 64 tokens) is
not read, since that validator gates every `Get`. For such a key the delivered event decides: `indexWork` now carries
`deleted` (`:49`, set in `deliver` at `:126`), so its delete records nothing and a write records it at the delivered
revision.

Test: `TestRemovedNoncanonicalKeyIsNoLongerDamaged` (`processor/graph-index/damaged_integration_test.go:238`), four
subtests: `legacy-key` and a 1,025-byte key, each deleted and purged through the running component. Each waits for the
damage (floor), removes the key, writes another entity and reads `byName` with `min_revision` that write's revision, so
the reply follows the removal's completion; it requires `damaged_entities` absent and `Health` healthy.

## HIGH 2: a noncanonical key withholds every graph-embedding query

`removeDamaged` called `removeRecord`, whose `Storage.DeleteEmbedding` refuses a key that is not an entity ID before
any I/O. The refusal was marked `delete_failed`, which withholds `search` and `similar` with `index_not_ready` for
ever, since no retry makes the key valid.

Fix: `deleteRecord` (`processor/graph-embedding/hop1.go:134`). `Storage` keeps records only under literal entity IDs
(every write validates the ID, `graph/embedding/storage.go:255`, `:303`, `:352`), so a key that is not one has no
record: `deleteRecord` returns nil for it and calls `DeleteEmbedding` otherwise. Both delete sites use it: `removeRecord`
(`:122`, the damage, tombstone and textless paths) and `readFailed`'s absent-key branch (`:93`). `Storage`'s validator
is unchanged, and a real delete failure is still marked `delete_failed` and withholds.

Test: `TestNoncanonicalKeyKeepsSearchServing` (`processor/graph-embedding/damaged_integration_test.go:129`):

- "stored before start" and "written after start": `legacy-key` holds bytes that do not decode beside one titled entity
  A. After the floor passes its revision, `search` answers A alone with `damaged_entities` 1 and `Health` names the key
  with no unresolved failure. The key is then deleted; after the floor passes a later write, `search` counts no damaged
  entity and `Health` is healthy.
- "absent when reconciled": `reconcileEntity` is called directly for `legacy-key`, which was never stored, as a
  coalesced batch or a repair pass reads a key that has gone. No delivery reaches this branch deterministically (the
  coalescer drops a key's pending update on its delete), so the test calls the unexported method; nothing in
  production code changed for it. `search` then answers.

## MEDIUM: the `Complete` order described backwards

`awaitHealthy`'s comment (`processor/graph-clustering/component_integration_test.go:188`) and `5.4.md`, "Follower
order", now say what `Follower.Complete` does (`graph/follower/follower.go:349-353`): it clears the damage record before
it raises the floor. `awaitHealthy` still waits rather than reading once; its comment says the wait keeps the check from
resting on that order. The uncovered-order note in `4.2.md` stands.

## Failing first

On `17d5b130` (the new tests added, production code unchanged):

```text
--- FAIL: TestRemovedNoncanonicalKeyIsNoLongerDamaged (0.42s)
    --- FAIL: TestRemovedNoncanonicalKeyIsNoLongerDamaged/delete (0.30s)
        damaged_integration_test.go:258: byName Alpha after the delete: damaged_entities = 1; want the key absent
        damaged_integration_test.go:260: Health after the delete of legacy-key = {Healthy:false ... ErrorCount:2 LastError:1 damaged entity: legacy-key (noncanonical_entity_id); write a valid value for each key or delete it ...}; want healthy
    --- FAIL: TestRemovedNoncanonicalKeyIsNoLongerDamaged/purge (0.12s)
        damaged_integration_test.go:258: byName Alpha after the purge: damaged_entities = 1; want the key absent
```

That run's test had only the `legacy-key` subtests, then named `delete` and `purge`; the 1,025-byte key's subtests
fail the same way with `17d5b130`'s `index.go` mapped in (first row of the table below).

```text
--- FAIL: TestNoncanonicalKeyKeepsSearchServing/written_after_start (0.28s)
    damaged_integration_test.go:175: search {"query":"gadget"}: index not ready: unresolved index write failures
--- FAIL: TestNoncanonicalKeyKeepsSearchServing/absent_when_reconciled (0.25s)
    damaged_integration_test.go:185: search after reconciling the absent legacy-key: index not ready: unresolved index write failures; Health = {Healthy:false ... ErrorCount:1 LastError:1 unresolved failures: delete_failed 1 ...}
```

"stored before start" hangs on `17d5b130`: its `awaitAnswer` keeps asking while the reply is `index_not_ready`, which is
both the defect and the refusal before the vector cache's first snapshot, so only `go test -timeout` ends it (run with
`-timeout 60s`: `panic: test timed out after 1m0s`, in `awaitAnswer` at `damaged_integration_test.go:166`).

## Wrong changes

`task mutate:check` runs no integration-tagged test, so each wrong change is a copy under `/tmp` mapped in with
`go test -overlay <json> -tags integration -race -count=1 -cpu 1 -timeout 3m`; nothing in the tree was edited
(SHA-256 of both production files checked before and after):

| Wrong change | Test run | Outcome |
| --- | --- | --- |
| graph-index `index.go` as at `17d5b130` | `TestRemovedNoncanonicalKeyIsNoLongerDamaged` | detected: all four subtests, `damaged_entities = 1` and Health naming the key |
| `reconcileNoncanonical` without the `work.deleted` check | same | detected: both over-long key subtests; both `legacy-key` subtests pass |
| `deleteRecord` without the entity-ID check | `.../written_after_start`, `.../absent_when_reconciled` | detected: both, `index not ready: unresolved index write failures` |
| `readFailed` calls `DeleteEmbedding` directly | `TestNoncanonicalKeyKeepsSearchServing` | detected: "absent when reconciled" alone; the other two pass |

## graph-lexical and graph-clustering

Neither has the defect, so neither changed. Neither validates the key on the way to removing what it holds:
graph-lexical's damage and delete paths both reach `applier.drop` (`processor/graph-lexical/apply.go:152`, `:332`),
whose `lexical.Index.Delete` returns nothing; graph-clustering's `Maintainer.Delete`
(`graph/clustering/maintainer.go:157`) returns nil for an ID it holds no member for, and `raise` skips an ID the parser
refuses. Each completes the delete's own revision (`apply.go:340`; `processor/graph-clustering/component.go:298`),
which clears the record reported at the earlier revision, and neither reports damage for a delete.

Measured with one test per package, added through `-overlay` and not committed: `legacy-key` holds bytes that do not
decode beside a valid entity; the test requires the damage named (graph-lexical: `search` counts 1), deletes the key,
waits for a later write (graph-lexical: `min_revision`; graph-clustering: `awaitApplied` then a tick) and requires
`Health` healthy (graph-lexical: `damaged_entities` 0). Both passed. The graph-lexical body, using the package's test
helpers:

```go
const a, bad = "c360.lex.test.north.pump.a", "legacy-key"
client := fixtureClient(t)
states := ensureEntityStates(t, client)
c := startLexical(t, client, metric.NewMetricsRegistry(), nil)
put(t, states, a, text{name: "pump"})
reply := searchPast(t, client, `{"query":"pump"}`, putRaw(t, states, bad, []byte("{not json")))
// require reply.DamagedEntities == 1 and Health naming bad
states.Delete(t.Context(), bad)
reply = searchPast(t, client, `{"query":"pump"}`, put(t, states, a, text{name: "pump"}))
// require reply.DamagedEntities == 0 and Health healthy
```

The graph-clustering body: `startClustering(t, client, tickByHand)`, `put` a valid `acme.ops.test.alpha.doc.a1` and
`legacy-key` with `not an entity state`, `awaitApplied` and `tick`, require `Health` naming `1 damaged entity:
legacy-key`; delete it, `put` a1 again, `awaitApplied` and `tick`, require `Health` healthy.

## Not covered

- No spec scenario names the two new tests yet; the requirements they prove ("A damaged entity is left out and
  counted", graph-index and graph-embedding deltas) already cover a noncanonical key and a delete without restart.

## Round 2

Codex's re-review at `355f0e742657ab49e2b46e8240db1e97cb74684b` asked for changes: one HIGH finding.

### HIGH: a captured coalesced write records a deleted over-long key again

The round-1 fallback for a key the shared KV key validator refuses carried `deleted` on `indexWork` and recorded a
write at its delivered revision. With `coalesce_ms` above zero, `revisionCoalescer.fire` takes the pending set and
unlocks before handing it to the lanes, so the key's delete could be delivered, clear the record and complete before
that batch reached the lane; the older write then recorded the deleted key again, and nothing later cleared it.

Fix (`53204baaa4be3711b24f6a5415e957676b02eb6e`): `deliver` (`processor/graph-index/index.go:123`) checks the key
with `natsclient.ValidateKVLiteralKey` (`:130`) before any coalescing or lane, and `settleUnreadable` (`:156`)
settles a refused key there, in delivery order: a write is recorded damaged at its revision, a delete or purge records
nothing (its delivery already cleared the record), and the revision completes. Such a key never enters the coalescer,
a lane or the repair set, so the round-1 `deleted` field is removed and `reconcileNoncanonical` (`:240`) always reads
current state. The shared validator is unchanged and the follower's record stays the one damage owner. The round-1
section above describes the `deleted` fallback this replaces.

Test: `TestUnreadableKeyRemovalOutlastsCoalescedWrite` (`processor/graph-index/damaged_integration_test.go:282`),
subtests `delete` and `purge` (both tombstones take the same delivery path; both run). It builds graph-index through
its production JSON with `coalesce_ms` 3600000, so only the test fires the coalescer, and `workers` 1, so one lane
takes every key in submit order. It writes the 1,025-byte key, then valid entity A, and waits for A's revision in the
coalescer's pending set: delivery is one entry at a time in order, so the key's write was delivered before it. The test
then fires the coalescer by hand with its own callback wrapped (`:303`): the wrapper hands the lanes every key but the
over-long one at once and holds that key's write until the test releases it. The delete or purge then runs against
real NATS; the test waits for the floor to reach the bucket's last sequence, requires no damage recorded, releases the
held write, and writes A again through the same lane. A `byName` reply at that write's revision must count no damaged
entity, and `Health` must be healthy.

The wrapper is a test-only replacement of the coalescer's existing unexported callback field, set in the test; no
production code changed for it, and no production caller can reach it. With the fix the held list is empty, since the
key never reaches the coalescer; on the old code it holds the key's write, which is the interleaving under test. No
ordering through production calls alone holds a taken batch back from its lane, so the wrapper is the pause.

Shown failing on `355f0e74` (the new test added, production code unchanged; the key and the Health timestamps
elided as `...`):

```text
--- FAIL: TestUnreadableKeyRemovalOutlastsCoalescedWrite (0.45s)
    --- FAIL: TestUnreadableKeyRemovalOutlastsCoalescedWrite/delete (0.30s)
        damaged_integration_test.go:368: byName Alpha after the delete: damaged_entities = 1; want the key absent
        damaged_integration_test.go:370: Health after the delete = {Healthy:false ... ErrorCount:1 LastError:1 damaged entity: kkk...k (noncanonical_entity_id); write a valid value for each key or delete it ... Status:degraded}; want healthy
    --- FAIL: TestUnreadableKeyRemovalOutlastsCoalescedWrite/purge (0.14s)
        damaged_integration_test.go:368: byName Alpha after the purge: damaged_entities = 1; want the key absent
        damaged_integration_test.go:370: Health after the purge = {Healthy:false ... Status:degraded}; want healthy
```

The Health line was then shortened to print the status and error count, since the key is over a thousand bytes.

Wrong changes, each a copy under `/tmp` mapped in with
`go test -overlay <json> -tags integration -race -count=1 -cpu 1 -timeout 3m -run
'TestUnreadableKeyRemovalOutlastsCoalescedWrite|TestRemovedNoncanonicalKeyIsNoLongerDamaged' ./processor/graph-index`
(SHA-256 of `index.go` checked before and after, matching):

| Wrong change | Outcome |
| --- | --- |
| `index.go` as at `355f0e74` | detected: both new subtests, `damaged_entities = 1` (`:368`), Health degraded (`:371`); `TestRemovedNoncanonicalKeyIsNoLongerDamaged` passes |
| `settleUnreadable` records a delete damaged too | detected: new subtests at `:357` (damage once the removal completed), both over-long subtests of `TestRemovedNoncanonicalKeyIsNoLongerDamaged` at `:267` |
| `settleUnreadable` records nothing for a write | detected: both over-long subtests of `TestRemovedNoncanonicalKeyIsNoLongerDamaged` at `:256` (no damage after the write) |
