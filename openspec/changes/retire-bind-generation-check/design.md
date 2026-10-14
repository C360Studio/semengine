# Design: retire-bind-generation-check

Owner ruling: #214 comment 6157056960. Inventory: `inventory.md` (revision 2, base `0bbd78f`, INVENTORY PASS). Every
`path:line` below is at `0bbd78f`. Revision 2 of this design folds in the pre-owner design review (PASS, three MEDIUMs
and five NITs).

**Merge order.** #215 merges first. While #214 is open, the known-flake rule (`.agents/protocol.md:205-213`) stops the
two overlapping open pull requests, #194 and #206, and #215 closes #214. No line overlaps (inventory item 4). #206's
own inventory counts this test file's `CreateMemoryStream` and `JetStream()` calls; both drop from 4 to 2. Those are
evidence counts that #206 may leave as they are.

## What a caller observes, and the test that proves it

| Behaviour after the change | Test |
| --- | --- |
| `Start` reads each input stream's creation time once, before it binds the consumer. A zero creation time fails setup before any bind | `TestStreamWithoutCreationTimeIsRefused` (unchanged) |
| Inputs carry that generation; a recreated stream's input at a lower sequence is applied | `TestIntegration_IngestGuard_StreamRecreatedWithLowerSequences`, `TestGuardRecordIsPerGeneration` (unchanged) |
| `Start` no longer fails when the stream is recreated between that read and the bind. An input it delivers at a sequence the old record covers is acknowledged without being applied (declared limitation) | none, by the ruling |
| An input the callback accepts and submits after a failed `Start`'s rollback is negatively acknowledged, never acknowledged or terminated: the stopped pool refuses it (`component.go:1139-1141`, `internal/dispatch/keyed_pool.go:216-220`, `:323-324`) | `TestFailedConsumerSetupCallbackNaksAfterRollback` (unchanged); `TestFailedStartDrainsAndNaksHeldInput` (flattened). Shown to fail by one by-hand mutation (task 2.1) |
| An input the callback accepts while a consumer setup is still running is submitted at once. If that setup then fails inside natsclient, it may be applied | none; "Settlement order" no longer promises otherwise |

## Options (the main choice is ruled; these are its shape)

**A. Where the declared limitations live.** None of the three has a current home: the only current mention is a
summary in the ledger (`docs/admission-ledger.yaml:3240-3242`) that points at an archived design.

- A1, recommended: in the spec's replay-protection requirement (renamed, D3). This follows the precedent of
  ruling 6113959050's other half, written into `projection-mutation/spec.md:47-51`. A spec is verified against code
  and is what reviews hold code to. README, `docs/contract.md` and the doc comments stop saying "never" and point to
  it. Cost: the operator-facing pages send readers to a spec.
- A2: the graph-ingest `README.md` (a contract line under AGENTS.md). The spec's "SHALL never be dropped" would still
  need correcting, so the list would in effect have two homes.
- A3: `docs/contract.md` (consumer-facing). Same objection; that page carries no other declared limitation (F1 is not
  on it).

**B. `onlyDisposition`** (`guard_generation_test.go:310`) keeps one caller, in the integration-tagged
`failed_start_callbacks_integration_test.go:447`. B1: leave it (zero edit; the untagged file then holds a helper none
of its tests use). B2, recommended: move it beside its caller. B3: inline it (repeats the ten lines T2 already
inlines at `:290-302`).

**C. Which tests stay.** C1, recommended: delete exactly what holds the check (inventory item 2). Flatten
`TestFailedStartDrainsAndNaksHeldInput` to its remaining case, keeping its name. C2: rewrite the integration test to
assert the limitation. That would pin a misbehaviour on Docker timing, and the ruling names the test as going. C3:
keep a unit test proving there is no hold. That tests an absence and shows a caller nothing.

## Decisions

- **D1 Code.** In `setupJetStreamConsumer`, delete `component.go:1542-1546`, `:1597-1610` and `:1641-1651`. Make
  `msgCtx` `_` again (`:1551`; `revive.toml` enables `unused-parameter`). Rewrite the first-read comment
  (`:1475-1478`) to say what graph-ingest does: one read before the bind, a generation every input carries, and the
  window as a declared limitation. Add no claim about how nats-server or nats.go behaves. Nothing else changes.
- **D2 Tests** (C1, B2). Delete `TestIntegration_IngestGuard_StreamRecreatedBeforeBindFailsStart`. Delete
  `TestConsumerInputHeldUntilGenerationConfirmed` with `heldEntity`, `heldInputComponent`, `heldInputMsg` and
  `handlerOnItsOwnGoroutine`. Delete the subtest "generation changed". Remove the imports this leaves unused
  (`errors`, `testing/synctest`, `strings`). Rewrite the flattened test's doc (`:324-330`) without the hold. T2's
  doc (`:223-228`) names the test's own `heldMetadataMsg` hold and stays.
- **D3 Text** (A1). Make the spec delta in `specs/graph-ingest-recovery/spec.md`:
  - "Settlement order" takes back R4's wording (task 4.16, archived `tasks.md:686-687`, `notes/cp4i-R1.md:64-66`).
    The Nak is promised only for an input submitted after the rollback, "never acknowledged or terminated". 4.17's
    "or applied" goes from the requirement and from the scenario's THEN: no surviving test observes it, and adding a
    check is not this change's job. The scenario's WHEN and THEN are R4's text word for word.
  - The requirement keeps one thing 4.17 added: the promise covers any failed `Start`, not only a failed consumer
    setup. That does not rest on the hold, and the case "a later registration fails" holds it.
  - "Replay protection is generation-aware" drops 4.17's two sentences and its scenario. "SHALL never be dropped"
    becomes a statement about the carried generation, and the requirement gains the limitation list. The list names no
    count and the recovery sentence names no position, so one deletion takes out any bullet.
  - OpenSpec 1.14's archive refuses a MODIFIED block that drops a scenario. A MODIFIED draft failed `openspec validate
    --strict` with "omits scenario(s) the current spec still has". So the requirement is REMOVED and ADDED again as
    "Replay protection uses the generation read before the bind", after the precedent of
    `archive/2026-10-13-natsfixture-picked-port`. Measured on a copy: the delta validates, and a trial archive applies
    it (+1, ~1, -1). The new requirement lands last in the spec, still "below" Settlement order.
    `guard_generation_test.go:3-4` is the one line in the tree that cites the old name.

  Then correct `README.md:27-34`, `docs/contract.md:79-80`, `keyed_ingest.go:34-35` and `:81-84` to stop promising
  more than that, each pointing to the spec. Correct the ledger's `processor/graph-ingest` row (`:3165-3170`
  contract, `:3240-3242` limitations, `:3251-3254` proving tests) and re-pin `:3166` and `:3283`.
- **D4 Untouched.** The spec's Purpose (it states how the record is keyed, which is unchanged). The archived
  ingest-kernel change (history). `waitForStreamInput` and `consumeStream`, whose removal is #139 item A
  (#139 comment 6157272349). natsclient.

## Premises (measured)

- Before 4.17, graph-ingest read the generation once: `7e07fea`'s patch, hunk `@@ -1452`.
- Once the second read goes, nothing in `setupJetStreamConsumer` fails after `consumeStream` returns:
  `component.go:1656` and `:1661` return nothing.
- Tests older than 4.17 do not need the hold: `task verify` passed on `f9ac34f` (archived `tasks.md:689`).
- A recreated stream gets a generation that differs from the old one on the pinned server.
  - After a broker restart: `TestIntegration_IngestGuard_StreamRecreatedWithLowerSequences` would fail otherwise.
  - Deleted and created again on a live server: in every passing run of the deleted
    `TestIntegration_IngestGuard_StreamRecreatedBeforeBindFailsStart` (30 local runs, and every CI run in "Flake
    evidence" that carried it), `Start` failed at `:273` because the two reads differed.
- The only signal of the limitations is `semengine_graph_ingest_redeliveries_dropped_total`
  (`keyed_ingest.go:275-277`), which counts and acknowledges with no log line.
- No consumer imports SemEngine today: semsource `go.mod:8`, semconnect `go.mod:6`.

## Probe rule

The added text states graph-ingest's own behaviour. The library mechanisms behind each limitation stay in the rulings,
which cite source readings, not probes. A broker restart inside the window (SemSource's path, #15) is named as not
measured. Recovery after a restored stream is "not promised".

## Not covered

- A pull that outlives its consumer's deletion and is later answered by a same-named consumer on a new stream
  (archived `design.md:607-608`). It has no ruling, no probe and no named trigger: it needs another party to create
  a consumer under graph-ingest's own name. The owner declined a spec bullet for it when accepting this design
  (2026-10-13, PR #215); it is recorded here only.
- A stream recreated while graph-ingest runs (archived `notes/4.3.md:58-59`): graph-ingest stops receiving. This was
  filed as #216 in design review. This change does no work on #216 and does not edit the spec scenario "Memory
  transport lost at broker restart" (`spec.md:21-25`) or `docs/contract.md:75`; #216 names them.

## Declared cost

A failed `Start` after a real bind may leave an input its consumer fetched unacknowledged until the acknowledgement
wait passes (30 s by default, `component.go:1435`), and only then be delivered again. This is #214's unconfirmed
candidate (#214's body, "Not yet measured"). It is not promised and not measured, and it is a delay,
not a loss. Under the File step (`.agents/protocol.md:126`) it lives here. It becomes an issue only if it is measured
as a defect.

## Flake evidence (for the pull request body)

| | Command | Result |
| --- | --- | --- |
| Before, local | `scripts/test-integration.sh '-run=^TestIntegration_IngestGuard_StreamRecreatedBeforeBindFailsStart$' -count=30 -failfast=false ./processor/graph-ingest`, on tree `d2ba1a7` (branch `claude/retire-bind-generation-check` at its claim commit `27e7703`) | 0 failures in 30 runs, 16.5 s, macOS Docker Desktop. Local only: the runner's evidence is in `.evidence/`, which git ignores |
| Before, CI | job Verify, every run since #93 merged (2026-10-12T21:12:43Z), all branches, cancelled runs not counted | 1 failure in 26 Verify runs (run 38480011965, at `guard_generation_integration_test.go:286`). The branches of some runs may not have carried the test |
| After | the same command | the test no longer exists: nothing runs. The behaviour it held is retired, not fixed |
