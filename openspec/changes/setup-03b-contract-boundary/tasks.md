# Tasks: setup-03b-contract-boundary

Each task names the outcome that proves it, its role (architect, developer, reviewer, technical-writer, owner) and its
gate. The owner ruled Q1–Q13 on #8 (comment 5929656835), the level name on #4 (comment 5929716018), Q14–Q18 (comment
5929902986), the tier-0 scope for both halves (proposal 5930855353, approved by 5930898291; it releases the Q14–Q18
holds and re-rules Q11), the operator surface (5931143569), and BM25's tier, `composition/cli`, the tier model and the
extended critical list (5932313950), all on 2026-10-01. Each task that waited on a ruling states the ruling it follows.
No task asserts a post-merge fact.

Every task ends in one of three states: done, with its evidence; moved to its home, ticked, with a pointer
("moved to #9, Carried to 04A, item n" names a numbered item in epic #9's body); or a deliberate not-done (`[~]`) with
its reason. "Reviewer re-check" is the independent reviewer's APPROVE of `03e22f1` (change review re-check), whose
R1–R5 corrections landed in `a4465c0`. An unchecked task that says "hold" is one `task spec:queue` reports as blocked:
4.9 (unknown action types).

## 1. Rulings recorded

- [x] 1.1 Follows ruling Q1 (#3) and the Q11 re-ruling: explicit per-package registration in each consumer's
      composition root, no aggregator, no engine-side registry. `design.md` D1 records the ruling, the owner's quality
      bar, the measured consequence (the two aggregator roots drop five packages, −2,758 lines), every starter
      consumer's aggregator import, and `output/websocket` and `processor/graph-clustering` at tier 0.
      Role: technical-writer. Gate: reviewer verdict. Evidence: D1; reviewer re-check.
- [x] 1.2 Follows ruling Q2 (#2): SemEngine runs alongside ADR-106 with no compatibility promise to SemStreams'
      Tier-1 package surface. D2 records it, the package-tier clarification, the adopted mechanical check (task 2.6),
      and that the ADR-106 canary question is moot. Role: technical-writer. Gate: reviewer verdict. Evidence: D2;
      reviewer re-check.
- [x] 1.3 Follows the #4 ruling: the word is "tier". The matrix and this change use "tier N" for a capability level;
      "profile" appears only as the quoted 03A run label (`profile: bm25|neural`); D3 quotes the ruling.
      Role: technical-writer. Gate: reviewer verdict. Evidence: D3; reviewer re-check.
- [x] 1.4 Follows ruling Q5 (#18): defer the applied-input barrier. The deletion row of the matrix records the
      deferral with its consumer consequence (semsource#215), the named-readable-fact constraint on #15's generation
      identity, and L4a as the precedent. Role: technical-writer. Gate: reviewer verdict. Evidence: D6 and the
      Deletion rows; reviewer re-check.
- [x] 1.5 Follows rulings Q6 (#19) and Q7 (#20). The mutation rows of the matrix and D11 record them together:
      `ExpectedRevision` through the typed client with a loud `revision-conflict`; the narrow commit-classification
      repair with lookup and fence deferred; SemConnect's move to the typed client and its classifier mapping on its
      ledger row. Role: technical-writer. Gate: reviewer verdict. Evidence: D7, D8, D11 and the Mutation rows;
      reviewer re-check.
- [x] 1.6 Follows ruling Q8 (#8): SemConnect is a first-wave tier-0 consumer. D9 records the ruling and the matrix
      carries the ADR-102 d5 and ADR-104 identity rows as Keep with harness proving tests, plus the spatial,
      temporal and hierarchy rows SemConnect exercises. Role: technical-writer. Gate: reviewer verdict. Evidence: D9
      and the Identity and Query rows; reviewer re-check.
- [x] 1.7 Follows ruling 5932313950 (BM25 at tier 0) and the Q8 re-measure (pass3 §2): D4 and D9 record tier 0 as
      65 / 140,842 (pass3 set H less `graph/embedding/http_embedder.go`, plus `composition/cli`; measured by the
      writer) in place of 67 / 129,063, with the command, the delta table against 65 / 126,926 and 67 / 129,063, and
      the kept packages that still reach a cut package. Role: technical-writer. Gate: reviewer verdict. Evidence: D4
      ceiling block and delta table row K; reviewer re-check ("re-ran D4's block").
- [x] 1.7b The architect's confirmation of the ceiling at Slice 04A's first `go list -deps`, and the SemConnect ledger
      rows that follow it: moved to #9 (Carried to 04A, items 1 and 2).
- [x] 1.8 Follows ruling Q14 (5929902986): admission by owner mandate. `design.md` opens with the owner's statement
      of purpose and the two-halves admission rule as `AGENTS.md` on `main` states it, and every matrix row names its
      half, its tier and its qualifying consumer. Role: technical-writer. Gate: reviewer verdict. Evidence: "Purpose
      and admission" and the matrix columns; reviewer re-check.
- [x] 1.9 Follows the operator-surface ruling (5931143569): D15 records the area, the tested contracts, no
      `semengine-ui` repository, the no-UI documentation path, and one change-observation primitive with its operator
      and consumer transports (D14). Role: technical-writer. Gate: reviewer verdict. Evidence: D14, D15; reviewer
      re-check.
- [x] 1.10 Follows ruling Q16 (5929902986). D13 records the outcome of Q16's hypothesis: "overlapping in part" —
      SemSource's `sourcelifecycle` implements durable-execution concerns `pkg/lifecycle` does not model; its stated
      reasons were classification plus pin constraints (semsource `replay-source-removal/design.md:53-70`); see #24.
      Role: technical-writer. Gate: reviewer verdict. Evidence: D13; reviewer re-check.

## 2. Boundary and port set

- [x] 2.1 Follows rulings Q4, Q11 (re-ruled), Q14 and Q17. One `docs/admission-ledger.yaml` row per package in the
      tier-0 set (D4) and one per separated package, each with the full pin SHA, disposition per D4, the proving test
      from the matrix, and a `destination` that is internal unless a starter consumer imports the package (D16;
      semteams' set unmeasured): moved to #9 (Carried to 04A, item 1). Each Slice 04A change adds the rows for the
      packages it ports, as `docs/provenance.md` states.
- [x] 2.2 Follows #8 ruling Q2. The Tier-1 cross-check re-measured on the ruled tier-0 set, with each Tier-1-only
      package's disposition: moved to #9 (Carried to 04A, item 1), with the ledger rows it feeds. D2 caveats that
      39 / 23 / 26 were measured on the first-pass 65.
- [x] 2.3 Follows rulings Q14, 5932313950 (tier model) and the Q8 re-measure: the tier-0 definition (D3 tier model, D4
      ceiling 65 / 140,842) is recorded with the reproduction commands (pass3 §2.5 and D4's block) the developer
      re-runs as `go list -deps` once the seam exists. Role: technical-writer. Gate: reviewer verdict. Evidence: D3,
      D4; reviewer re-check.
- [x] 2.4 Follows rulings Q9 and 5932313950 (critical list extended). D10 records the Q9 list plus `processor/rule`,
      `processor/rule/expression`, `pkg/lifecycle`, `graph/clustering`, `processor/graph-clustering`,
      `pkg/graphview`, `output/websocket`, `internal/maxdelivery`, `graph/inference`, `graph/structural`,
      `graph/embedding` (BM25 half) and `composition/cli`, and the preflight skill's gate table names the list as the
      scope of `task cover:check`; no placeholder package is added. Role: technical-writer. Gate: `task docs:check`
      and reviewer verdict. Evidence: D10; `.agents/skills/semengine-preflight/SKILL.md` `cover:check` row; reviewer
      re-check (D10 section 4).
- [x] 2.4b The `cover:check` targets for each critical package, set as the package is ported: moved to #9 (Carried to
      04A, item 7).
- [x] 2.5 Follows #8 ruling Q4 (port-refactor tracking). The `docs/admission-ledger.yaml` header documents the
      `class:port-refactor` convention (D4a): an `adapt` row whose `contract` cites its matrix row, whose
      `proving_tests` names the proving test, and whose `known_risks` names the class and its tracking issue, within
      the ten fields T-B7 accepts. Role: technical-writer. Gate: `task ledger:check`. Evidence: the ledger header;
      `task ledger:check` passes.
- [x] 2.5b Every port-refactor ledger row following that convention: moved to #9 (Carried to 04A, item 1); the issues
      carrying the label are #25–#36 (task 7.2).
- [x] 2.6 Follows #8 ruling Q2 (mechanical check). `design.md` I8 and the drafted `harness-boundaries` delta place
      the SemEngine boundary test (no Go file imports `github.com/c360studio/semstreams` or a path under it) in the
      first 04A change that carries that delta, so the spec never lags the test; this change adds no test.
      Role: technical-writer. Gate: reviewer verdict. Evidence: I8 and the drafted delta; reviewer re-check.
- [x] 2.7 Follows rulings Q4 and Q11 (re-ruled) and the provider re-partition. The `gateway/graph-gateway` and
      `gateway` `defer-exclude` rows (with SemConnect's compile-only `gateway.Gateway` assertion as consumer work) and
      the behind-the-seam rows (dormant carry of `graph/llm`, `model/wire` and `graph/embedding/http_embedder.go`;
      exit condition "tier 0 compiles without the behind-the-seam packages and files"): moved to #9 (Carried to 04A,
      item 1). The design records both in D4.
- [x] 2.8 Follows ruling Q15. The agentic-domain separation rows and the `service/rule_pack_bind.go` and
      `pkg/rulepack` carry rows: moved to #9 (Carried to 04A, item 1). The design records them in D4 and D12; the
      edits are issues #27, #29, #30, #31.
- [x] 2.9 Follows ruling Q16. The tier-0 `pkg/lifecycle` carry row citing the corrected footprint (scope Q1.6): moved
      to #9 (Carried to 04A, item 1). The design records it in D4.
- [x] 2.10 Follows ruling Q1 (D1 adopter path, pass3 §2.6). The D1 `adapt` row listing the boot refusal and the five
      doc-comment sites with their proving test: moved to #9 (Carried to 04A, item 1); the edit is issue #33.
- [x] 2.11 Follows ruling 5932313950 (BM25 at tier 0). The `graph/embedding` and `processor/graph-embedding` rows
      (tier 0 for the BM25 half; `http_embedder.go` behind the tier-1 seam): moved to #9 (Carried to 04A, item 1); the
      edit is issue #35.
- [x] 2.12 Follows ruling Q17 (5929902986). D16 records internal-by-default, the measured consumer import sets
      (semteams' unmeasured), and the three DX gates (compiled example consumer in CI, package-doc lint on exported
      packages, the metric-name drift test), each named against the 04A change that first exports a package.
      Role: technical-writer. Gate: reviewer verdict. Evidence: D16; reviewer re-check. Building the gates: moved to
      #9 (Carried to 04A, item 5).
- [x] 2.13 Follows ruling 5932313950 (`composition/cli` admitted at tier 0). The `composition/cli` ledger row (151
      lines; imports `component`, `composition`, `config`): moved to #9 (Carried to 04A, item 1).
- [x] 2.14 Follows #8 ruling Q9 (confirmation). The package that enforces ADR-102 d5 at the pin is named and is on the
      critical list: `processor/graph-ingest` (`authority_gate.go`, `mutation_runtime.go` emit `authority_foreign`).
      Evidence: change review round 1 on PR #21, finding N5; recorded in D10. Role: reviewer (evidence),
      technical-writer (record). Gate: reviewer verdict.

## 3. Retained-contract matrix

- [~] 3.1 Every matrix row's SemSource "Observed" cell cites an observation name or states "not observed". Partly
      done, deliberately: the #15 row names its two failing observations (`broker_restart_reingested_exact_*`) as the
      repair row, and rows the 03A run observed cite observation names. The rest cite the 03A summary result
      (`bm25 61/63`, supplements) or the source fact (`pins.json`, `run.go:932`) instead; mapping each to an
      observation name changes no decision. Reason recorded for the archive; the observation list is SemSource's
      `pinned-results.json@75a17f7d`.
- [x] 3.2 Every row's proving test, with the role and change that writes a test that does not exist yet: moved to #9
      (Carried to 04A, item 3) — each package's ledger row names its proving tests when it is ported.
- [x] 3.3 SemConnect "Observed" cells still marked `pending #74`, filled from semconnect#74's recorded qualification
      evidence: moved to #9 (Carried to 04A, item 4); that PR is a draft whose evidence can still change.
- [x] 3.4 The tier crosswalk row carries SemSource's config file names verbatim and SemConnect's "no tiers"; the
      Tiers rows and D3 state each tier's guarantees and its fallback. Role: architect (guarantees),
      technical-writer (record). Gate: reviewer verdict. Evidence: D3 tier model, the Tiers and Crosswalk rows;
      reviewer re-check.
- [x] 3.5 find/anchor/ask are recorded as cases mapped to `ResolveMode` values and `Entity/Entities` hydration, with
      their 03A observation names; no mode API is introduced. Role: technical-writer. Gate: reviewer verdict.
      Evidence: D5; reviewer re-check.
- [x] 3.6 Follows ruling Q14. Every matrix row carries a Half, a Tier and a Qualifying consumer; a row with none says
      why ("framework invariant: none", "none (deferred row)", "none; removal row"). semboids and semteams have
      observed columns, "—" where the row was not inventoried. Role: technical-writer. Gate: reviewer verdict.
      Evidence: the matrix; reviewer re-check (M5 resolved).
- [x] 3.7 Follows ruling Q15. The Rules rows (E1 dispatch, E1 action-type refusal, E2 decode, E3 core imports, E4 deny
      and verdict audit, E5 rule-pack contract, semteams' `replace_owned` debt) each carry observed baselines from
      semboids (7/7) and semteams (70/119 executable on the core), and E1–E4 each have a `class:port-refactor` issue
      citing its matrix row and proving test. Role: technical-writer. Gate: reviewer verdict. Evidence: D12, the Rules
      rows, issues #25–#28; reviewer re-check.
- [x] 3.8 Follows ruling Q16. The Entity workflows row records `pkg/lifecycle` kept at tier 0, semboids' `flock.boid`
      workflow as qualifying consumer, revision-fenced CAS present and effect fences absent, and its proving test.
      Role: technical-writer. Gate: reviewer verdict. Evidence: the row and D13; reviewer re-check.
- [x] 3.9 Follows ruling Q18. The Settlement row records graph-ingest's kept order, the first non-agentic `natsclient`
      settlement caller, the `InProgress` heartbeat, and the process-kill-mid-apply proving test on a file stream
      ("state equals one application"). Role: technical-writer. Gate: reviewer verdict. Evidence: the row and D11;
      reviewer re-check (M6 resolved).
- [x] 3.10 Follows ruling Q18. The Parked input row records `MAX_DELIVERY_EVENTS` as already provisioned by
      `config.EnsureStreams` for all four consumers and the port of the `internal/maxdelivery` observer, with its
      proving test. Role: technical-writer. Gate: reviewer verdict. Evidence: the row and D13; reviewer re-check.
- [x] 3.11 Follows the scope ruling (change observation). Three rows record the KV-watch primitive on
      `ENTITY_STATES`/`COMMUNITY_INDEX`, `pkg/graphview` as the view layer, and `output/websocket` as a tier-0
      consumer transport over applied state, with its port change as a `class:port-refactor` (#34); the service's SSE
      KV watch is the operator transport. Role: technical-writer. Gate: reviewer verdict. Evidence: the rows and D14;
      reviewer re-check (M2 resolved).
- [x] 3.12 Follows ruling Q18 (recovery). Three Recovery rows (memory stream, file stream, no stream; one consumer
      each) replace the single acknowledged-write row and I7, and the drafted `graph-ingest-recovery` delta states
      them with a scenario each. Role: technical-writer. Gate: `task spec:check` and reviewer verdict. Evidence: the
      rows, I7, the drafted delta; reviewer re-check.
- [x] 3.13 Follows the operator-surface ruling. The Operator surface rows cite the ruled `path:line` for each
      endpoint, with keep/change/defer, owner, proving test, tier, and readiness in the durable-execution half.
      Role: technical-writer. Gate: reviewer verdict. Evidence: the rows; reviewer re-check.
- [x] 3.14 Follows the operator-surface ruling. The dashboard definition (checked in against the engine's metric
      names) and the drift test that compares them are named in D15 and D16 against the 04A change that first exports
      a package; the flowgraph response has a golden-test row. Role: architect. Gate: reviewer verdict. Evidence: D15,
      D16, the Operator surface rows; reviewer re-check.
- [x] 3.14b Writing the dashboard definition and the drift test: moved to #9 (Carried to 04A, item 5).
- [x] 3.15 Follows the operator-surface ruling. The contract document leads its operator section with the no-UI path
      (the endpoints, Swagger at `/docs`, status and trace, and composition CLI Mermaid). Role: technical-writer.
      Gate: `task docs:check`. Evidence: `docs/contract.md` "Operator endpoints and metrics" (`be2203c`).
- [x] 3.16 Follows pass3 §2.4. The "Registration · aggregator imports" row's SemConnect cell records
      `payloadbuiltins` and `vocabulary/builtins` imported in production at `dff12657`, with semboids' and semteams'
      cells beside it. Role: technical-writer. Gate: reviewer verdict. Evidence: the row; reviewer re-check.

## 4. Repair-before-port rows and the tier-0 gate

- [x] 4.1 Follows rulings Q12 and Q18. #15: the matrix durability row and D11 record repair-before-port, the
      generation-identity design constraint (D6/D11), and the failing-first real-NATS gate evidence #9 requires.
      Role: technical-writer. Gate: reviewer verdict. Evidence: D11 #15 row; reviewer re-check.
- [x] 4.1b The `processor/graph-ingest` ledger row for #15: moved to #9 (Carried to 04A, item 1).
- [x] 4.2 Follows rulings Q12 and Q18. #16: the transport row and D11 record the one-home reserved-subject
      declaration, the boot refusal, and the collision regression. Role: technical-writer. Gate: reviewer verdict.
      Evidence: D11 #16 row; reviewer re-check.
- [x] 4.2b The `graph`, `processor/graph-ingest`, `processor/graph-query`, `composition` ledger rows for #16: moved to
      #9 (Carried to 04A, item 1).
- [x] 4.3 Follows #8 ruling Q12. #17: the deletion row and D11 record the `syncFromKV` repair and the ported SemSource
      reproduction as gate evidence. Role: technical-writer. Gate: reviewer verdict. Evidence: D11 #17 row; reviewer
      re-check.
- [x] 4.3b The `config` ledger row for #17: moved to #9 (Carried to 04A, item 1).
- [x] 4.4 Follows rulings Q6 (#19) and Q7 (#20). The mutation rows, D7, D8 and D11 record the ruled shape and its
      proving tests, including the injected KV `Update` timeout that the caller observes as `CommitUnknown`.
      Role: technical-writer. Gate: reviewer verdict. Evidence: D7, D8, D11; reviewer re-check.
- [x] 4.4b The `pkg/projection` and `processor/graph-ingest` ledger rows for #19 and #20: moved to #9 (Carried to 04A,
      item 1).
- [x] 4.5 Follows #8 ruling Q12. D11 maps the ruling-4 lifecycle debt (SS#1411, SS#1415, SS#1218, SS#1220, SS#1417,
      SS#1145/#1147) to repair shapes; the tier-0 gate binds the figures D11 recomputes from A5 (69 entries / 6
      copies), and the ten entering packages' figures are recorded as unmeasured. Role: technical-writer. Gate:
      reviewer verdict. Evidence: D11; reviewer re-check (section 5).
- [x] 4.5b Each debt item in its ledger row's `known_risks`, and the ten entering packages measured: moved to #9
      (Carried to 04A, items 1 and 8).
- [x] 4.6 Follows rulings Q13 and Q18. D11 records SemStreams PR #1437's `processor/graph-ingest` half as a
      ledger-row candidate with the graph-ingest repair rows; its `agentic` half is not carried (Q4).
      Role: technical-writer. Gate: reviewer verdict. Evidence: D11; reviewer re-check.
- [x] 4.6b The PR #1437 ledger row citing the PR head SHA and its proving test: moved to #9 (Carried to 04A, item 1).
- [x] 4.7 Follows ruling Q18. The settlement repair-before-port row is in D11 with its gate evidence: process kill
      mid-apply on a file stream in the harness, state equal to one application. Role: technical-writer. Gate:
      reviewer verdict. Evidence: D11 settlement row; reviewer re-check.
- [x] 4.7b The `processor/graph-ingest` and `natsclient` ledger rows for settlement: moved to #9 (Carried to 04A,
      item 1).
- [x] 4.8 Follows ruling Q18. The `internal/maxdelivery` row (port, re-home under SemEngine's `internal/`, and the
      composition question D13 leaves to the 04A change that ports it): moved to #9 (Carried to 04A, item 1); the
      edit is issue #36.
- [ ] 4.9 Hold: held on #8 — refuse unknown action types at load vs fail at fire time (owner). The E1 Rules row and
      D12 record the ruled behavior for an action whose type is neither core nor registered, with its proving test and
      the consequence for semteams' five `replace_owned` files; issue #25 carries the edit. Role: owner (ruling),
      technical-writer (record). Gate: reviewer verdict.

## 5. Fusion and graph-tool boundary

- [x] 5.1 `design.md` D5 maps the four layers to packages at the pin, records keep/change/defer for `Engine.Fuse`,
      package-level `fusion.Fuse`, and `searchGraph` with test implications, and marks SS#621 and SS#603 as reproduce-
      first rows. Role: technical-writer. Gate: reviewer verdict. Evidence: D5; reviewer re-check.
- [x] 5.2 The spec-delta drafts for `graph-transport-boundary`, `graph-ingest-recovery`, `config-desired-state`,
      `projection-mutation`, and the `harness-boundaries` modification (T-B8 and I8) are present in `design.md` with
      at least one scenario per requirement, ready for the 04A changes to carry. Role: architect. Gate: `task
      spec:check` and reviewer verdict. Evidence: "Spec deltas drafted for the SETUP 04A changes"; `task spec:check`
      passes; reviewer re-check. Carrying them: moved to #9 (Carried to 04A, item 9).

## 6. Current docs and landing

- [x] 6.1 `docs/repository-map.md` records the approved boundary, port set, tier-0 target, critical list and the
      port-refactor issues; `docs/provenance.md` status names this change; the contract document is
      `docs/contract.md` and the plan is amended for the tier model. Role: technical-writer. Gate: `task docs:check`
      and `task spec:check`. Evidence: those files; both gates pass.
- [ ] 6.2 The independent pre-owner design review's findings are addressed or recorded as declared costs, and the
      reviewer confirms the R1–R5 corrections (`a4465c0`) and reads `docs/contract.md`. Role: technical-writer (apply),
      reviewer (re-check). Gate: reviewer verdict.
- [ ] 6.3 The change is archived as the last content commit of PR #21 with `skip_specs: true` (no spec sync), after
      the owner rules 4.9; the archive commit also updates links that point into this change's directory
      (`docs/contract.md`, `docs/setup-plan.md`, `docs/repository-map.md`, `docs/provenance.md`, the preflight skill,
      the ledger header). Role: technical-writer. Gate: reviewer check of the archive commit.
- [x] 6.4 Follows the scope ruling (sequencing). `design.md` D13 references epic #24 as the home of the
      durable-execution primitive, and the 03B matrix carries only the tier-0 floor rows of that half.
      Role: technical-writer. Gate: reviewer verdict. Evidence: D13; reviewer re-check.

## 7. Bound for SETUP 04A (recorded here, performed before or by the 04A changes)

- [x] 7.1 SETUP 02 harness extension (failpoint injection and a subprocess host for the process-kill and
      broker-restart proving tests; `natsfixture` has `Stop` only), the first task of the first Slice 04A change:
      moved to #9 (Carried to 04A, item 6). The matrix rows that need it (Recovery · file stream, Settlement, Entity
      workflows) and the D11 settlement row reference it.
- [x] 7.2 File the `class:port-refactor` issues from D4a, one per edit, each citing its matrix row and proving test,
      before any 04A change claims the work. Role: technical-writer. Gate: `gh issue list --label
      class:port-refactor` lists one issue per D4a item. Evidence: #25–#36 (E1–E4 as #25–#28; `component` #29;
      `service` #30; `graph-query` #31; capability seam #32; D1 adopter path #33; `output/websocket` #34;
      `graph/embedding` split #35; `internal/maxdelivery` #36), milestone Slice 04A, each `Addresses #8`.
