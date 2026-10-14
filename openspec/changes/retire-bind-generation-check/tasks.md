# Tasks: retire-bind-generation-check

The developer runs on Opus: the change is on the settlement and acknowledgement path. Read `design.md` (one page),
`proposal.md` and the spec delta; cite only those. Simple, idiomatic Go: cut, never add, no new hooks, no `context`
values. Run `task test:unit` after each code task and `task verify` on a clean tree before the review stop. Choices
the design does not spell out go in `notes/<task>.md`, each with a one-line PR comment that links it.

## 1. Record (orchestrator)

- [x] 1.1 Owner ruling #214 comment 6157056960; inventory revision 2 with INVENTORY PASS; the design's pre-owner
  review. The owner accepted `design.md` on 2026-10-13, declining the fourth limitation bullet (PR #215).

## 2. Code and text (developer)

- [x] 2.1 Remove the check and the tests that assert it (design D1, D2). In `processor/graph-ingest/component.go`:
  - delete `:1542-1546`, `:1597-1610` and `:1641-1651`;
  - make the callback's `msgCtx` `_` (`:1551`);
  - rewrite the first-read comment (`:1475-1478`) with no claim about nats-server or nats.go.

  In the tests:
  - delete `TestIntegration_IngestGuard_StreamRecreatedBeforeBindFailsStart`;
  - delete `TestConsumerInputHeldUntilGenerationConfirmed` and its four helpers (`heldEntity`, `heldInputComponent`,
    `heldInputMsg`, `handlerOnItsOwnGoroutine`);
  - delete the subtest "generation changed" of `TestFailedStartDrainsAndNaksHeldInput`, flatten that test to its
    remaining case under the same name, and rewrite its doc without the hold;
  - move `onlyDisposition` beside its one caller in `failed_start_callbacks_integration_test.go`;
  - remove the imports this leaves unused;
  - in `guard_generation_test.go:3-4`, cite the requirement by its new name, "Replay protection uses the generation
    read before the bind".

  Checks:
  - `task test:unit`;
  - `task lint`;
  - `task test:integration -- '-run=^(TestFailedConsumerSetupCallbackNaksAfterRollback|TestFailedStartDrainsAndNaksHeldInput)$'
    -count=10 -failfast=false ./processor/graph-ingest` (the two tests whose Nak path changes);
  - one by-hand mutation (`docs/testing.md`, "Show that the test can fail"; #129): with the Nak in the
    submit-failure branch (`component.go:1629-1631` before the edit) deleted, one run of
    `TestFailedConsumerSetupCallbackNaksAfterRollback` and `TestFailedStartDrainsAndNaksHeldInput` fails both.
  Record the commands and results in `notes/2.1.md`.
- [x] 2.2 Correct the promises (design D3). Write nothing that restates the spec's list: point to `graph-ingest-recovery`,
  "Replay protection uses the generation read before the bind".
  - `processor/graph-ingest/README.md:27-34`: say the generation is read before `Start` binds the consumer. Replace
    "never dropped" with the pointer.
  - `docs/contract.md:79-80`: re-sending applies the current data once the ingest component has started again. Name
    the exceptions by the pointer, in plain words.
  - `processor/graph-ingest/keyed_ingest.go:34-35` and `:81-84`: the same two corrections as doc comments.

  Check: `task docs:check`.
- [x] 2.3 Admission ledger, row `processor/graph-ingest` (design D3). Edit four places:
  - the contract sentence at `:3165-3170`: one read before the bind; 4.17's check retired by #214 comment 6157056960;
  - the declared limitations at `:3240-3242`: add #214 comment 6157056960 and point to the spec instead of design D8;
  - `proving_tests` at `:3251-3254`: drop the two deleted tests;
  - the line pins `:3166`, and `:3283` in row `processor/graph-ingest/component.go`, re-pinned to the code after 2.1.

  Check: `task ledger:check`.
- [ ] 2.4 Run `task verify` on a clean tree at the last commit; record the commit and the result in a PR comment. In
  the PR body:
  - keep `Closes #214`;
  - add the design's "Flake evidence" table (the protocol's "A pull request that closes a flake shows the
    reproduction");
  - add `implemented-by:`.

  #216 was filed in design review; file nothing else. Under the File step, the design's "Declared cost" and "Not
  covered" sections are the record. This change does no work on #216 and does not edit `spec.md:21-25` or
  `docs/contract.md:75`.

## 3. Review and close

- [ ] 3.1 Hold: Codex's review of record on the 2.4 commit. Fix the findings, and request again after any content
  commit. Once Codex's newest review record approves, add the `reviewed-by:` line to the PR body, naming `codex` and
  the persona from the record.
- [ ] 3.2 Archive the change and sync `openspec/specs/graph-ingest-recovery/spec.md` as the last content commit.
  Check: `task spec:check`.

Task 3.2 is completed by the archive commit. The archive check, the CI run before merging and
`task merge:check -- 215` are recorded on PR #215 after it; no task claims them.
