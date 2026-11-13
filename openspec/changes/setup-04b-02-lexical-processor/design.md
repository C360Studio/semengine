# Design: setup-04b-02-lexical-processor

## Context

Facts: `inventory.md` (INVENTORY PASS at `5582eb9e`, pin fixed in `f7a1b001`), cited by section, and two probes in
`notes/probes/`, each with source and log: `objectstore-watch-probe` and `body-size-probe`. Nothing feeds
`graph/lexical` (§1). Four rules decide an entity's text, none with the Name/Body split `lexical.Document` takes (§2a).
`graph/follower` and `pkg/graphview` keep in-memory state from a KV watch (§2b). The only text verb is the embedding
`search`, with a cosine `Similarity` and no pre-limit count (§3). SemSource offloads every passage and code body (§9).
The index costs about 60 B per held word (#623). Revision 3 answers PR #629's design read and two reviews.

## Goals / Non-Goals

Goals: a tier 0 component that keeps a word index current from `ENTITY_STATES`, rebuilt on every start, gated until
caught up, with one verb change 3 can use; one text rule shared with graph-embedding; a first build whose time
under a blocking store does not grow with the entity count; measured memory and rebuild cost; #628's first item.
Non-goals: changes 3 and 4; a stored index; the per-entity word record (D6 places it).

## Decisions

**D1. Placement: its own component, `graph-lexical`** (chosen), the fourth follower plug-in, with its own key and
responder: its memory, rebuild and faults are its own, and it runs at every tier. Rejected: inside graph-index (a
second apply style; a word-index fault would withhold eight structural verbs; no store registry, §2b, §4); inside
graph-embedding (off at tier 0 by the plan; a mode flag is what ADR-108 D6 rejects); nothing (hashed vectors stay).
Cost: a product registers one more factory (§9).

**D2. Mechanism: a follower plug-in that applies Value, with one body path** (chosen). The follower gives the key,
the floor that makes `indexed_revision` true, reset, lost-feed, order and generation checks, and `QueryReady` (§2b
table). `Deliver` runs on the follower's goroutine (`graph/follower/follower.go:75-76`) and does no I/O: it decodes,
selects text (D3), `Put`s the inline text, and completes the revision unless the entity has an offloaded body, which
goes to the body path (D5). Rejected: `pkg/graphview` (no key, reset, order or generation check; a copy of every
entity); both (two gates, §7); re-read through a pool (a `Get` per entity per rebuild).

The order rule for the two writers: a delivery at or below the revision the index holds for its key is ignored; a
body read for revision R is indexed only if the key is still at R when it returns, else dropped, and R completes
either way. A generated check proves it, because two paths write one key and their order is the risk: random
interleavings of deliveries, deletes, body outcomes and late returns against a reference model, which also checks
that every delivered revision completes. Named witness first: a read in flight while R+1 is applied.

**D3. One text rule: `graph/entitytext`** (chosen), a pure package (no NATS client: `go list -deps ./storage | grep
-c nats-io` is 0) holding graph-embedding's suffix rule, defaults and body read, moved. Two views:

- `Values`: string values whose predicate ends in a configured suffix, relationships skipped, in triple order.
  graph-embedding joins them as today, so its bytes, dedup keys and vectors do not change.
- `Document`: Name is every value of a vocabulary label predicate, whatever its suffix, and every `.name` or `.title`
  value; Body is every other suffix-selected value, then the offloaded body. An entity `byName` finds is then found
  by its name's words; graph-embedding's parity is change 4's question.

Rejected: the rule in `graph/lexical` (the tier 1 home would import a word index for its text); a fifth extractor.
The cap: `max_text_len` keeps graph-embedding's meaning, applies to Body only, never cuts Name, and cuts the body's
tail. graph-embedding keeps 4,000; graph-lexical takes 16,000. On SemSource at `4ca2072c` (`body-size-probe.log`):
4,000 cuts 34 of 4,766 Go symbol bodies (2.27% of code text), 16,000 cuts 1 (0.04%); no doc fenced block exceeds
1,507 characters. A cut is counted in a gauge and the first is logged. File paths (R3) are deferred: a product adds
its path predicate's suffix; SemSource's `-test` question is measured with its runner (#620).

**D4. The verb: a new subject, `graph.lexical.query.search`** (chosen). Request
`graph.LexicalSearchRequest{Query, Limit, Scope, MinRevisionField}`, strict; limit 0 means 10, 1 to 100, else
refused (the existing similarity bounds); `lexical.ParseQuery` refusals answer `invalid_request` naming the rule
(#624 stays with change 3). Reply, enveloped: `LexicalSearchData{Results []LexicalHit{EntityID, Score}, Matched,
BodiesNotIndexed}`. `Matched` counts matches in scope before the limit (change 3's `EntitiesTruncated`); prefixes are
local search, no scope is global. `BodiesNotIndexed` counts the whole index, not the scope or the hits.
`indexed_revision` is the follower's floor and `min_revision` is honoured as graph-index's are. Rejected: serving
`graph.embedding.query.search` (two responders at tier 1; an unbounded score in `Similarity`); `semantic` forwarding
here by config (two meanings for one verb). Fusion's "cosine similarity" doc (`pkg/fusion/retrieval.go:57`) stays
true; when fusion reads the word index (change 4), it becomes "the mode's own score, on that mode's scale".

**D5. Readiness, restart and offloaded bodies.** The verb answers `index_not_ready` until the floor reaches the
replay's target, fatal after a reset, `index_not_ready` after a lost feed (§4). An offloaded entity's revision
completes only when its body is indexed, dropped (D2) or in a state below, so a first build that reads ready has read
every body it could.

The body path is one goroutine under the run context; each read runs under `context.WithTimeout(runCtx, 10s)`, so
`Stop` ends it. Probe (nats.go v1.55.0, server 2.15.0): a healthy read takes 0.42 to 0.46 ms mean, 1.07 ms p99; with
the server paused a read returns at its deadline (2.002 s at 2 s, 2.967 s at 3 s) with `nats: timeout`, which wraps
no context error (`jetstream/object.go:1454-1458`, `:1473-1480`); a plain cancel surfaced after 4.95 s. So a `Stop`
during a read waits at most that read's deadline. `graph/entitytext` decides "deadline passed" by the read's own
context having ended, never by the store's error.

The bound: any read failure on a store other than a missing object stalls that store until the next repair pass;
its queued entities complete as `unread` without a read. A first build under a blocking store takes the inline
replay plus one deadline per store, whatever the entity count. A store that ignores its context is declared: the key
reads degraded after 30 s without a completion (`follower.go:90-95`), and `Stop` returns its context's error.

| State | Cause | Retried | Key and `Health` |
| --- | --- | --- | --- |
| queued | not yet read | next | floor waits; healthy |
| unread | any failure but missing, or store stalled | every repair pass | failure `body_unread`: degraded, named in `Health` (`follower.go:382-384`, `:508-512`) |
| missing | `storage.ErrObjectNotFound` (`storage/objectstore/store.go:148-153`) | not until the entity changes | healthy; first logged |
| no store | no store under the instance | each pass, a registry lookup | healthy; first logged |

None withholds the verb. Stores register before graph-lexical starts: store providers start in their own phase first
(`service/component_manager_lifecycle.go:37-54`, `:123`; `service/component_manager.go:293-301`), so `no store` means
a store outside the process or a configuration fault. Repair passes run 30 s apart and never overlap; one costs the
healthy reads plus one deadline per store. The live watch: `Deliver` does no I/O; in the probe a watch reader at
1 ms per entry got all 25,000 entries of a 20,000-write burst in order; past nats.go's subscription limits is not
promised. A broker restart is the follower's, shared with the three other plug-ins; the smoke test proves no lost
feed (D8), and reads during the outage stall their store until a pass after it. Cost at SemSource's size, arithmetic:
32k bodies at 0.46 ms is about 15 s of reads plus 2 s of `Put`; task 2.6 measures it against the budget.

graph-embedding through the same reader (#551): a body read that times out fails the record `content_error`, as any
body read failure does today (`graph/embedding/worker.go:417-421`), withholds nothing, and frees the lane for its
next record; `TestHungStoreFreesWorkerLane` proves it.

**D6. The per-entity word record: an extension point, not built.** In the body path before a read:
`lexical.Index.Words(id)`, `PutWords(id, name, body)` (change 1 D7), a tokenizer version, and a bucket
`LEXICAL_WORDS` of `{source revision, tokenizer version, name words, body words}` per entity, written after `Put`. A
start restores matching records and skips their reads. The verb, reply, key and config do not change. When: task 2.6.

**D7. Memory (#623): measure, then move to word numbers** (task 2.2), before real data: #623's harness before and
after, the layout change 1's `sequence-probe` measured at 472 B per 20-word entity. Rejected: accepting 60 B, which
fits SemSource's corpus (bodies average about 600 characters) but not a million entities. Gauges report held entities
and words; no caller, test or scorecard answer changes.

**D8. Smoke (#628, first item).** `stackConfig`, `componentFactories` and `indexKeys` gain `graph-lexical` (§4). A
check asks for a word one fixed name holds, a word every base's name holds, and a deleted entity's word, waiting on
the reply's `indexed_revision`. Restart tests compare whole replies, rank and score (R1). The broker-restart tests
count eight components and graph-lexical's key with no lost feed. The fixture has no bodies; task 2.4 proves them.

**D9. Docs.** The README says word search helps entities with text only; sensor and drone telemetry gain nothing and
are reached by prefix, predicate, relationships, name and alias, and group. It states the verb, body states, cap,
file paths, memory and restart cost.

**D10. Three pull requests, one change.** A: task 2.1 (`Closes #551`). B: task 2.2 (`Closes #623`). C: the rest
(`Addresses #628`), with the archive. Cost: the `entity-text` and `graph-embedding` deltas reach `openspec/specs`
with C. Adoption list (D3 is a primitive): `graph/clustering/terms.go:57` (change 4),
`processor/graph-query/localsearch.go:95` (change 3), filed as one issue at landing.

## Risks / Trade-offs

- The first build waits for every reachable body; other hosts than the probe's are unmeasured until task 2.6.
- A transient store fault reads degraded until a repair pass reads the bodies; that is the honest state.
- Search holds the read lock while it scores, so a common-word query on a large index delays writes (§2c).
- Each process holds its own index. graph-embedding is edited (D3), guarded by a byte-equality test.

## Migration Plan

None for data. A product adds `graph-lexical` to its composition; graph-embedding's keys and output are unchanged.

## Open Questions

Ruled on PR #629: bodies read every start; no silent switch of SemSource's NL path; `docs/setup-plan.md:429-430`
corrected; the 10-second deadline, derived from the run context.

1. Owner: the first-build budget. Recommended: 60 s for 32k offloaded entities on one machine, about four times the
   arithmetic; if task 2.6 exceeds it, the word record (D6) is the next change.
2. Owner: graph-lexical's default cap of 16,000 characters (D3), as measured; lower only if task 2.2 says so.
