# Tasks: setup-03b-contract-boundary

Each task names the outcome that proves it. The owner ruled Q1–Q13 on #8 (comment 5929656835), the level name on #4
(comment 5929716018), the tier-0 scope for both halves (proposal 5930855353, approved by 5930898291; it rules Q14–Q18
and re-rules Q11) and the operator surface (5931143569), all on 2026-10-01. Each task that waited on a ruling now
states the ruling it follows. An unchecked task that says "hold" is one `task spec:queue` reports as blocked; exactly
one does (4.9). No task asserts a post-merge fact.

## 1. Rulings recorded

- [ ] 1.1 Follows ruling Q1 (#3) and the Q11 re-ruling: explicit per-package registration in each consumer's
      composition root, no aggregator, no engine-side registry. `design.md` D1 records the ruling, the owner's quality
      bar, the measured consequence (the two aggregator roots drop five packages, −2,758 lines), every starter
      consumer's aggregator import, and `output/websocket` and `processor/graph-clustering` at tier 0.
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
      carries the ADR-102 d5 and ADR-104 identity rows as Keep with harness proving tests, plus the spatial,
      temporal and hierarchy rows SemConnect exercises.
- [ ] 1.7 Follows the Q8 re-measure (pass3 §2, `inventory-3-pass3.md`): D4 and D9 record tier 0 as 62 packages /
      134,356 lines (set I; SemConnect at `dff12657` including `graph-index-spatial` and `graph-index-temporal`,
      semboids at `8c03cc53`) in place of 67 / 129,063, with the delta table against 65 / 126,926 and 67 / 129,063
      and the eight kept packages that still reach a cut package; the SemConnect ledger rows follow it.
- [ ] 1.8 Follows ruling Q14 (5930898291): admission by owner mandate. `design.md` opens with the owner's statement
      of purpose and the two-halves admission rule as `AGENTS.md` on `main` states it, and every matrix row names its
      half and its qualifying consumer.
- [ ] 1.9 Follows the operator-surface ruling (5931143569): D15 records the area, the tested contracts, no
      `semengine-ui` repository, the no-UI documentation path, and its reconciliation with D14's websocket transport.

## 2. Boundary and port set

- [ ] 2.1 Follows rulings Q4, Q11 (re-ruled), Q14 and Q17. `docs/admission-ledger.yaml` holds one row per package in
      the tier-0 set (D4, 62 packages) and one per separated package, each with the full pin SHA
      `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, disposition per D4, the proving test from the matrix, and a
      `destination` that is internal unless a starter consumer imports the package (D16); `task ledger:check` passes.
- [ ] 2.2 Follows #8 ruling Q2. The ledger cross-check against `release/tier1-packages.txt` is recorded in `design.md`
      (39 overlap, 23 Tier-1-only, 26 set-only at the pin) with each Tier-1-only package's disposition.
- [ ] 2.3 Follows rulings Q14 and the Q8 re-measure. The tier-0 definition (D4, 62 / 134,356) is recorded with the
      reproduction commands (pass3 §2.5) that the developer re-runs as `go list -deps` once the seam exists.
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
- [ ] 2.7 Follows rulings Q4 and Q11 (re-ruled) and the provider re-partition. `gateway/graph-gateway` and `gateway`
      carry `defer-exclude` rows naming their re-admission condition and SemConnect's compile-only `gateway.Gateway`
      assertion as consumer work. The behind-the-seam rows (`graph/llm`, `model/wire`, `graph/embedding`,
      `processor/graph-embedding`) record the dormant carry through the first green tier-0 extraction and the
      capability seam as the last task of 04A, exit condition "tier 0 compiles without the behind-the-seam packages".
- [ ] 2.8 Follows ruling Q15. The agentic-domain separations carry rows naming their re-admission condition: not
      porting `service/milestone_service.go`, dropping the agentic label predicates from `graph-query`, and the
      `component` adapt (dropping `Dependencies.ToolRegistry` and `ToolRegistryReader`) recorded as a consequence of
      rule-core edit E3, not as an independent edit. `service/rule_pack_bind.go` and `pkg/rulepack` carry `carry` rows.
- [ ] 2.9 Follows ruling Q16. `pkg/lifecycle` carries a tier-0 `carry` row; the `LifecycleManager` field and its six
      pass-through lines and three imports in `component` and `service` stay, and the row cites the corrected
      footprint (scope Q1.6).
- [ ] 2.10 Follows ruling Q1 (D1 adopter path, pass3 §2.6). The D1 ledger row (`adapt`) lists the boot refusal at
      `processor/graph-ingest/component.go:739` and the five doc-comment sites, each with the per-package call it
      should name, and its proving test (the refusal text names a symbol that exists in the SemEngine tree).
- [ ] 2.11 Follows the provider re-partition (5930898291). The `graph/embedding` and `processor/graph-embedding` rows
      record "behind the seam: criterion applied, owner may override" and the alternative ceiling (pass3 set H,
      64 / 140,911).
- [ ] 2.12 Follows ruling Q17. D16 records internal-by-default, the measured consumer import sets, and the three DX
      gates (compiled example consumer in CI, package-doc lint on exported packages, the metric-name drift test), each
      named against the 04A change that first exports a package.

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
- [ ] 3.6 Follows ruling Q14. Every matrix row carries a "Half" (graph, durable execution, or both) and a
      "Qualifying consumer"; semboids and semteams have observed columns, "—" where the row was not inventoried.
- [ ] 3.7 Follows ruling Q15. The Rules rows (E1 dispatch, E1 action-type refusal, E2 decode, E3 core imports, E4 deny
      and verdict audit, E5 rule-pack contract, semteams' `replace_owned` debt) each carry observed baselines from
      semboids (7/7) and semteams (70/119 executable on the core), and E1–E4 each have a `class:port-refactor` ledger
      row candidate citing its matrix row and proving test.
- [ ] 3.8 Follows ruling Q16. The Entity workflows row records `pkg/lifecycle` kept at tier 0, semboids' `flock.boid`
      workflow as qualifying consumer, revision-fenced CAS present and effect fences absent, and its proving test.
- [ ] 3.9 Follows ruling Q18. The Settlement row records graph-ingest's kept order, the first non-agentic `natsclient`
      settlement caller, the `InProgress` heartbeat, and the process-kill-mid-apply proving test on a file stream.
- [ ] 3.10 Follows ruling Q18. The Parked input row records `MAX_DELIVERY_EVENTS` as already provisioned by
      `config.EnsureStreams` for all four consumers and the port of the `internal/maxdelivery` observer, with its
      proving test.
- [ ] 3.11 Follows the scope ruling (change observation). Three rows record the KV-watch primitive on
      `ENTITY_STATES`/`COMMUNITY_INDEX`, `pkg/graphview` as the view layer, and `output/websocket` as a tier-0
      transport over applied state — not a UI, and not a relay of ingest input.
- [ ] 3.12 Follows ruling Q18 (recovery). Three Recovery rows (memory stream, file stream, no stream; one consumer
      each) replace the single acknowledged-write row and I7, and the drafted `graph-ingest-recovery` delta states
      them with a scenario each.
- [ ] 3.13 Follows the operator-surface ruling. The Operator surface rows cite the ruled `path:line` for each
      endpoint, with keep/change/defer, owner, proving test, and readiness in the durable-execution half.
- [ ] 3.14 Follows the operator-surface ruling. The dashboard definition (checked in against the engine's metric
      names) and the drift test that compares them are named against the 04A change that ports `metric`; the
      flowgraph response has a golden-test row.
- [ ] 3.15 Follows the operator-surface ruling. The documentation outline for the contract document leads with the
      no-UI path (composition CLI Mermaid, Swagger at `/docs`, status and trace endpoints).
- [ ] 3.16 Follows pass3 §2.4. The "Registration · aggregator imports" row's SemConnect cell records
      `payloadbuiltins` and `vocabulary/builtins` imported in production at `dff12657`, with semboids' and semteams'
      cells beside it.

## 4. Repair-before-port rows and the tier-0 gate

- [ ] 4.1 Follows rulings Q12 and Q18. #15: the matrix durability row and the `processor/graph-ingest` ledger row
      record repair-before-port, the generation-identity design constraint (D6/D11), and the failing-first real-NATS
      gate evidence #9 requires.
- [ ] 4.2 Follows rulings Q12 and Q18. #16: the transport row and the `graph`, `processor/graph-ingest`,
      `processor/graph-query`, `composition` ledger rows record the one-home reserved-subject declaration, the boot
      refusal, and the collision regression.
- [ ] 4.3 Follows #8 ruling Q12. #17: the deletion row and the `config` ledger row record the `syncFromKV` repair and
      the ported SemSource reproduction as gate evidence.
- [ ] 4.4 Follows rulings Q6 (#19) and Q7 (#20). The mutation rows and the `pkg/projection`, `processor/graph-ingest`
      ledger rows record the ruled shape (D7, D8) and its proving tests, including the injected KV `Update` timeout
      that the caller observes as `CommitUnknown`.
- [ ] 4.5 Follows #8 ruling Q12. Ruling-4 lifecycle debt: SS#1411 (`component`), SS#1415 (`config`), SS#1218
      (`service`, `pkg/errs`), SS#1220 (`service`), SS#1417 (ported tests), SS#1145/#1147 (lifecycle-suite floor)
      each appear in the relevant ledger row's `known_risks` with the repair shape from D11; the tier-0 gate binds
      68 cleanup-baseline entries and 6 `lifecycleUsed` copies, and the nine entering packages' figures are recorded
      as unmeasured.
- [ ] 4.6 Follows rulings Q13 and Q18. With the graph-ingest repair rows (#15, #20, settlement), SemStreams PR #1437's
      `processor/graph-ingest` half is recorded as a ledger-row candidate citing the PR head SHA and its proving test;
      its `agentic` half is not carried (Q4).
- [ ] 4.7 Follows ruling Q18. The settlement repair-before-port row (D11) is on the `processor/graph-ingest` and
      `natsclient` ledger rows with its gate evidence: process kill mid-apply on a file stream in the harness.
- [ ] 4.8 Follows ruling Q18. The `internal/maxdelivery` ledger row records the port, its re-home under SemEngine's
      `internal/`, and the composition question D13 leaves to the 04A change that ports it.
- [ ] 4.9 Hold: held on #8 — refuse unknown action types at load vs fail at fire time (owner). The E1 Rules row and
      the E1 ledger row record the ruled behavior for an action whose type is neither core nor registered, with its
      proving test and the consequence for semteams' five `replace_owned` files.

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
- [ ] 6.4 Follows the scope ruling (sequencing). `design.md` D13 references epic #24 as the home of the
      durable-execution primitive, and the 03B matrix carries only the tier-0 floor rows of that half.
