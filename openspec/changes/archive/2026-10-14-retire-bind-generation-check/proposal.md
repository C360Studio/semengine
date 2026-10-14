# Proposal: retire-bind-generation-check

## Why

Issue #214 is labelled `class:flake`, so every merge waits on it. The flaky test,
`TestIntegration_IngestGuard_StreamRecreatedBeforeBindFailsStart`, holds a check that PR #93 added for Codex's interim
review R3 (task 4.17). After graph-ingest binds its consumer, it reads the input stream's creation time (its
generation) a second time. It holds every delivery until that read returns, and it fails `Start` when the two reads
differ. The case it covers is an input stream deleted and created again between graph-ingest's first read and its
consumer's creation.

The owner ruled on 2026-10-13 (#214 comment 6157056960) to retire the check. The case needs the input stream deleted
and recreated while graph-ingest starts. The code does not choose that condition, and no named workload does it, so
under the owner's rule of 2026-10-12 (#185) its fix is a doc line, never new mechanism. The case becomes a declared
limitation beside the two already ruled on #91 (comments 6112550268 and 6113959050). The ruling replaces the part of
6112550268 that let task 4.17 proceed.

## What changes

- graph-ingest reads each input stream's generation once, before it binds the consumer, as it did before task 4.17.
  The second read goes, and so do the hold on deliveries and the `Start` failure on a mismatch. The tests that hold
  them go too, the flaky one included.
- The `graph-ingest-recovery` spec loses the promises that rested on the hold. "Settlement order" goes back to the
  qualification of task 4.16 (Codex R4): a negative acknowledgement is promised for an input submitted after a failed
  `Start`'s rollback. "Replay protection is generation-aware" is replaced by "Replay protection uses the generation
  read before the bind", which states the declared limitations in one place.
- Lines that promise more than the code does are corrected: the graph-ingest `README.md`, `docs/contract.md`, two doc
  comments and the admission-ledger row. Each points to the spec rather than repeating the list.

## Impact

`processor/graph-ingest`: `component.go` (`setupJetStreamConsumer`), `keyed_ingest.go` (comments), `README.md`, three
test files. `docs/contract.md`, `docs/admission-ledger.yaml` (the `processor/graph-ingest` row and one pin in the
`processor/graph-ingest/component.go` row). Spec `graph-ingest-recovery`. There is no exported surface, configuration,
metric or dependency change. The `waitForStreamInput` and `consumeStream` fields stay; their removal is #139 item A
(#139 comment 6157272349). Closes #214.
