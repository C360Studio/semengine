# Tasks: setup-03b-contract-boundary

Each task names the outcome that proves it. The owner ruled Q1–Q13 on #8 (comment 5929656835) and the level name
on #4 (comment 5929716018), both on 2026-10-01; each task that waited on a ruling now states the ruling it follows.
An unchecked task that says "hold" is one `task spec:queue` reports as blocked. No task asserts a post-merge fact.

## 1. Rulings recorded

- [ ] 1.1 Follows ruling Q1 (#3): explicit per-package registration in each consumer's composition root, no aggregator,
      no engine-side registry. `design.md` D1 records the ruling, the owner's quality bar, and the measured
      consequence (production set unchanged at 65; `output/websocket` and `processor/graph-clustering` defer-exclude
      per Q11).
- [ ] 1.2 Follows ruling Q2 (#2): SemEngine runs alongside ADR-106 with no compatibility promise to SemStreams'
      Tier-1 package surface. D2 records it, the package-tier clarification, the adopted mechanical check (task 2.6),
      and that the ADR-106 canary question is moot.
- [ ] 1.3 Follows the #4 ruling: the word is "tier". The matrix and this change use "tier N" for a capability level;
      "profile" appears only as the quoted 03A run label (`profile: bm25|neural`); D3 quotes the ruling.
- [ ] 1.4 Follows ruling Q5 (#18): defer the applied-input barrier. The deletion row of the matrix records the
      deferral with its consumer consequence (semsource#215), the named-readable-fact constraint on #15's generation
      identity, and L4a as the precedent.
- [ ] 1.5 Follows rulings Q6 (#19) and Q7 (#20). The mutation rows of the matrix and D11 record them together:
      `ExpectedRevision` through the typed client with a loud `revision-conflict`; the narrow commit-classification
      repair with lookup and fence deferred; SemConnect's move to the typed client and its classifier mapping on its
      ledger row.
- [ ] 1.6 Follows ruling Q8 (#8): SemConnect is a first-wave tier-0 consumer. D9 records the ruling and the matrix
      carries the ADR-102 d5 and ADR-104 identity rows as Keep with harness proving tests.
- [ ] 1.7 Hold: held on #8 Q8-remeasure. SemConnect's import delta is re-measured at the semconnect#74 head (the branch
      also imports `processor/graph-index-spatial` and `processor/graph-index-temporal`); D9 records the resulting
      ceiling in place of 67 / 129,063, and the SemConnect ledger rows follow it.

## 2. Boundary and port set

- [ ] 2.1 Hold: held on #8 Q14 (port-set approval) and Q17 (each row's `destination` fixes an exported package path);
      otherwise follows rulings Q4 and Q11. `docs/admission-ledger.yaml` holds one row per package in the ruled port
      set, each with the full pin SHA `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, disposition per D4, and the proving
      test from the matrix; `task ledger:check` passes.
- [ ] 2.2 Follows #8 ruling Q2. The ledger cross-check against `release/tier1-packages.txt` is recorded in `design.md`
      (39 overlap, 23 Tier-1-only, 26 set-only at the pin) with each Tier-1-only package's disposition.
- [ ] 2.3 Hold: held on #8 Q14. The tier-0 definition: the closure target (D4, about 49 packages) is recorded with
      the command the developer re-runs after the file-level adapts.
- [ ] 2.4 Follows #8 ruling Q9. The critical package list (D10) is recorded in `design.md` and in the preflight skill's
      gate table as the scope of `task cover:check` once those packages exist; no placeholder package is added to
      satisfy the gate. The package that enforces ADR-102 d5 at the pin is named, with its file and line, and is
      confirmed on the list.
- [ ] 2.5 Follows #8 ruling Q4 (port-refactor tracking). The `docs/admission-ledger.yaml` header documents the
      `class:port-refactor` convention (D4a): an `adapt` row whose `contract` cites its matrix row, whose
      `proving_tests` names the proving test, and whose `known_risks` names the class, within the ten fields T-B7
      accepts; every port-refactor row in 2.1 follows it and its issue carries the existing `class:port-refactor`
      label.
- [ ] 2.6 Follows #8 ruling Q2 (mechanical check). `design.md` I8 and the drafted `harness-boundaries` delta place
      the SemEngine boundary test (no Go file imports `github.com/c360studio/semstreams` or a path under it) in the
      first 04A change that carries that delta, so the spec never lags the test; this change adds no test.
- [ ] 2.7 Follows #8 rulings Q4 and Q11. `gateway/graph-gateway` and `gateway` carry `defer-exclude` rows naming
      their re-admission condition. The six higher-tier graph libraries' rows record the dormant carry through the
      first green tier-0 extraction and the capability seam as the last task of 04A, exit condition "tier 0 compiles
      without the six libraries".
- [ ] 2.8 Hold: held on #8 Q15. The agentic-group separations (dropping `component.Dependencies.ToolRegistry` and
      `ToolRegistryReader`, not porting `service/rule_pack_bind.go` or `service/milestone_service.go`, dropping the
      agentic label predicates from `graph-query`) carry rows naming their re-admission condition.
- [ ] 2.9 Hold: held on #8 Q16. The `pkg/lifecycle` separation (removing the `LifecycleManager` field and its calls
      from `component` and `service`) carries a row naming its re-admission condition.

## 3. Retained-contract matrix

- [ ] 3.1 Every matrix row's SemSource "Observed" cell cites an observation name from `docs/testing/setup-03a/pinned-
      results.json@75a17f7d` or states "not observed"; the two failing observations are marked as the #15 repair row,
      not as accepted behavior.
- [ ] 3.2 Every row has an owner and a proving test; rows whose proving test does not yet exist name the 04A change
      that adds it.
- [ ] 3.3 Follows ruling Q8 and semconnect#74's recorded qualification evidence (head `dff12657`). Every SemConnect
      "Observed" cell still marked `pending #74` is filled from that evidence or states "not observed".
- [ ] 3.4 The tier crosswalk row carries SemSource's config file names verbatim and SemConnect's "no tiers"; the
      fallback-ladder row states each tier's guarantees and its fallback per the #4 ruling.
- [ ] 3.5 find/anchor/ask are recorded as cases mapped to `ResolveMode` values and `Entity/Entities` hydration, with
      their 03A observation names; no mode API is introduced.

## 4. Repair-before-port rows and the tier-0 gate

- [ ] 4.1 Hold: held on #8 Q18 (settlement row); otherwise follows ruling Q12. #15: the matrix durability row and the
      `processor/graph-ingest` ledger row record repair-before-port, the generation-identity design constraint (D6/D11),
      and the failing-first real-NATS gate evidence #9 requires.
- [ ] 4.2 Hold: held on #8 Q18 (settlement row); otherwise follows ruling Q12. #16: the transport row and the `graph`,
      `processor/graph-ingest`, `processor/graph-query`, `composition` ledger rows record the one-home reserved-subject
      declaration, the boot refusal, and the collision regression.
- [ ] 4.3 Follows #8 ruling Q12. #17: the deletion row and the `config` ledger row record the `syncFromKV` repair and
      the ported SemSource reproduction as gate evidence.
- [ ] 4.4 Follows rulings Q6 (#19) and Q7 (#20). The mutation rows and the `pkg/projection`, `processor/graph-ingest`
      ledger rows record the ruled shape (D7, D8) and its proving tests, including the injected KV `Update` timeout
      that the caller observes as `CommitUnknown`.
- [ ] 4.5 Follows #8 ruling Q12. Ruling-4 lifecycle debt: SS#1411 (`component`), SS#1415 (`config`), SS#1218
      (`service`, `pkg/errs`), SS#1220 (`service`), SS#1417 (ported tests), SS#1145/#1147 (lifecycle-suite floor)
      each appear in the relevant ledger row's `known_risks` with the repair shape from D11; the tier-0 gate binds
      68 cleanup-baseline entries and 6 `lifecycleUsed` copies.
- [ ] 4.6 Hold: held on #8 Q18 (settlement row); otherwise follows ruling Q13. When the graph-ingest repair rows are
      designed, SemStreams PR #1437's `processor/graph-ingest` half is recorded as a ledger-row candidate citing the PR
      head SHA and its proving test; its `agentic` half is not carried (Q4).

## 5. Fusion and graph-tool boundary

- [ ] 5.1 `design.md` D5 maps the four layers to packages at the pin, records keep/change/defer for `Engine.Fuse`,
      package-level `fusion.Fuse`, and `searchGraph` with test implications, and marks SS#621 and SS#603 as reproduce-
      first rows.
- [ ] 5.2 The spec-delta drafts for `graph-transport-boundary`, `graph-ingest-recovery`, `config-desired-state`,
      `projection-mutation`, and the `harness-boundaries` modification (T-B8 and I8) are present in `design.md` with
      at least one scenario per requirement, ready for the 04A changes to carry.

## 6. Current docs and landing

- [ ] 6.1 `docs/repository-map.md` records the approved boundary, port set, tier-0 target, and critical list;
      `docs/provenance.md` status names this change; `task docs:check` and `task spec:check` pass locally.
- [ ] 6.2 The independent pre-owner design review's findings are addressed or recorded as declared costs.
- [ ] 6.3 The change is archived as the last content commit of PR #21 with `skip_specs: true` (no spec sync).
