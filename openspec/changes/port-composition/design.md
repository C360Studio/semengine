# Design: port-composition

Pin `8b99efe9`. Base `24a3355`. Inventory: `inventory.md` (INVENTORY PASS). Owner rulings 1-4, 2026-10-20 (#9 comment
6295357983).

## Inventory correction (probe, 2026-10-20)

This was a throwaway test in the pin copy, since removed. One component declaring `tcp:0.0.0.0:8080` on an input and
an output yields one `connection_pattern_error` ("network port conflict on tcp:0.0.0.0:8080 ... both trying to
bind"). The error comes from the pin flowgraph's bind refusal, not from composition. On the tree, `Declare` refuses
the component first (`component/registry.go:626`), so the outcome becomes `port_declaration_error`. The same probe
also ran an explicit stream on `graph.ingest.query.entity` beside a JetStream output on `graph.>`. Its one error is
`config_invalid` (the derived stream has no bounds) and nothing names a reserved subject: the pin does not refuse a
reserved-subject capture.

## Options: how the clash rule reaches composition

- O1, the pin's text compare. This is a second spelling of the rule, which #338 rules out, and the probe shows it
  admits four pairs the registry refuses.
- O2, `PortFacts.Clashes(PortFacts) bool`. One method, but composition would re-spell claim selection, holder order
  and the refusal text.
- **O3 (chosen):** `func (d Declaration) ConflictWith(held Declaration) error`.
  - It returns nil, or a plain, unwrapped error for the first clash reading `resource conflict: component '<d>' claims
    <c>, held by component '<held>' as <h>`.
  - It visits `held`'s claims, then `d`'s, each side inputs first and then outputs in declaration order. This is the
    order `checkResourceConflictsLocked` uses now (`component/registry.go:579-593`).
  - The registry iterates holders in name order and passes the error to `errs.WrapInvalid` as it does now. The
    refusal stays invalid-class and contains the text after the errs prefix (`pkg/errs/errs.go:364`; existing tests
    check `strings.Contains`, `component/registry_boot_admission_test.go:203,372`).
  - Composition uses the unwrapped `err.Error()` as the finding message. The rule, the claim selection, the order and
    the text each have one home.
  - Cost: one exported method, offset by removing the `ExclusiveResources` field.
- O4, offline admission on `Registry`. The largest surface, and it overlaps `CreateComponent` (#91).
- O5, no offline check. This breaks ADR-100 decision 3 (the same findings offline and at boot).

## Decision

- **`component`.** Add `ConflictWith`. `checkResourceConflictsLocked` calls it for each holder in name order.
  `exclusiveClaims` and `claimsClash` stay unexported. Remove `Declaration.ExclusiveResources` (declared at `:70`,
  written at `:315` and `:619`, cloned at `:708`). `exclusiveResources` (`:626`) becomes a self-clash check that
  returns only an error. The tests that read the field (`registry_boot_admission_test.go:62,87-88,499` and
  `registry_declare_test.go:86-87`) read the facts' `ResourceID()` instead.
- **`composition`**: ported from the pin files except `assert.go`.
  - Exclusive check: `validate.go:58-65,:139-155` call `declaration.ConflictWith(earlier)` over the declarations
    already accepted. Components are visited in name order, so the holder named is the first in name order. A
    refused declaration stays out of the graph, as at the pin.
  - Flowgraph adapts (inventory sweep item 4):
    - drop the `IssueOptionalInterfaceUnused` case (`analyze.go:195`);
    - drop the suggestion "Give each network listener its own address" (`:49`);
    - reword the `TypeConnectionPatternError` comment (`findings.go:24`) to name only KV-writer and graph-mutation
      conflicts;
    - convert `registration.Type` with `string(...)` (`validate.go:111`, `catalog.go:67,72`).
  - #16: see "#16 refusal" below.
- **Dropped.** `AssertValid` and its test (ruling 1). `shipped_configs_test.go`, whose imports are not admitted
  (inventory); the example-config test replaces it (ruling 4).
- **Simple leftovers, fixed now** (the owner's rule for simple leftovers):
  - one edge comparator, used by both `graph.go:103-121` and `mermaid.go:24-42`;
  - `portViews` (`graph.go:63-65`) calls `interfaceType` (`analyze.go:283`);
  - `displayNames` drops `robotics`, `graph-processor` and `context-processor` (no factory at the pin) and `udp`
    (not admitted);
  - the comments citing deleted SemStreams paths (`findings.go:66-68`, `analyze.go:212`) state the rule instead;
  - `doc.go` names `service`'s component manager as the boot caller to come, not as present code.
- **Optional, kept.** The pre-checks in `declareInstance` (sweep 1b) stay, because dropping them means classifying
  `Declare`'s errors by their text (shape 5). The `index < len(facts)` guards stay until #280 item 2 is decided with
  `service`.
- **No new issue.** The only cross-package leftover is #280, which is already filed.

## #16 refusal

**Reserved subjects.** `Analyze` reads them from `graph.QueryVerbs()` (`graph/query_verbs.go:25`) and
`graphmutation.SubjectFamily` (`internal/graphmutation/protocol.go:18`). It never spells them, under the guard
`internal/harness/contract/reservedsubjects_test.go:43`.

**Which streams.** Provisioning decides which streams exist, in `resolveStreamDeclarations`
(`config/stream_bounds.go:218-290`), from three sources:

1. **Framework constants** (`config/streams.go:97-174`: `logs.>`, `health.>`, `metrics.>`, `flows.>`,
   `governance.verdict.>`, `$JS.EVENT.ADVISORY...`). None overlaps a reserved subject, and none reaches `Analyze`.
2. **The operator's `streams` map**, which also overrides a constant by name (`:241-257`). `Analyze` receives it, so
   its finding component is `config` and the stream is named by its map key.
3. **JetStream outputs.** These are read from each enabled component's raw `ports.outputs` (`:258-290`;
   `extractPortsFromConfig`, `config/streams.go:354`) and named by `derivePortStream` (`:370-383`): the port's
   stream name, else `config.DeriveStreamName(subjects[0])` (`config/streams.go:66`). `Analyze` reads the
   declarations' JetStream output facts instead. It names each stream with the same rule, calling
   `config.DeriveStreamName`, so there is no second naming rule. Its finding component is the instance and port.

The declarations cover every stream that source 3 provisions under one premise: a ported factory either declares the
`ports` block's outputs or refuses the block. Refusing it is the "Unknown configuration keys are refused"
requirement (`openspec/specs/component-registration/spec.md:61`). graph-ingest, the tree's one factory, refuses any
output other than its `ENTITY_STATES` write (`processor/graph-ingest/component.go:292-312`). A refusal is a
`port_declaration_error`, so no declaration is checked. Not covered: a factory that accepts a `ports` block but
declares different outputs. No factory in the tree does that.

**The test is overlap, not cover.** A filter is refused when one concrete subject matches both the filter and a
reserved subject. This departs from 03B D11 `:481`, which says "cover" (ruling table): `*.mutation.create` captures
requests on `graph.mutation.create`, yet neither pattern covers the other. The tree has no overlap function.
`SubjectCovers` is directional (`internal/flowgraph/flowgraph.go:275`). `matchNATSPattern` handles two wildcard
patterns by testing each as a pattern over the other's tokens (`:353-356`), so it answers false for
`*.mutation.create` against `graph.mutation.>`. The port adds `SubjectsOverlap(a, b string) bool` to
`internal/flowgraph`; it is internal, so it adds no public surface.

**What is reported.** One `reserved_subject_capture` error per filter, naming the stream, the filter and the first
reserved subject it captures (query verbs in table order, then the mutation family).

**#16 stays open.** The real-NATS PubAck regression and refusal before readiness come with `service`; graph-query's
subjects come with change 4 (#106).

## Ruling to implementation

| Ruling / required finding | Implementation | Proving test |
| --- | --- | --- |
| #338; sweep 1a | `component.Declaration.ConflictWith`; the exclusive check in `composition/validate.go` | `TestDeclarationConflictWithMatchesAdmission`, `TestDeclarationConflictWithAdmitsNonClashingClaims`, `TestValidateExclusiveClashMatchesAdmission`, pin `TestValidateReportsExclusiveResourceConflict` |
| Ruling 3 | field removed at `component/registry.go:70,315,619,708` | build; `TestRegistryRefusesOneComponentClaimingAnAddressTwice` |
| Self-clash outcome (probe) | the `Declare` refusal becomes a finding | `TestValidateSelfClashIsPortDeclarationError` |
| Ruling 1; T-B1 | `assert.go` not ported | `TestImportGraph` |
| Ruling 2; 03B D11 `:481` | reserved-subject check in `Analyze`; `internal/flowgraph.SubjectsOverlap` | `TestValidateRefusesStreamCapturingReservedSubject`, `TestValidateDoesNotRefuseNeighbouringSubjects`, `TestReservedSubjectOverlapMatchesEnumeration` |
| Departs from D11 "cover" (`*.mutation.create`) | overlap, not cover | `TestValidateRefusesStreamCapturingReservedSubject` (`*.mutation.create` row) |
| Ruling 4; pin spec `:347` | `config/example_config.json` rewritten | `TestValidateExampleConfigHasNoErrorFindings`, `TestLoader_ExampleConfig` |
| Sweep 4 (flowgraph, type) | `analyze.go:49,195`, `findings.go:24`, three `string()` conversions | build; pin `TestValidateFindingsVocabularyIsClosed` |
| ADR-100 decisions 1-3 | the package as ported | the pin's validate, catalog and mermaid tests |

## Generated checks

- **Reserved overlap: a generated check.** This is a subject grammar with wildcards on both sides, an input format
  with many interacting cases.
  - Filters: every filter of 1 to 4 tokens over `{graph, ingest, query, mutation, entity, batch, prefix, z, *, >}`,
    with `>` only last. These include the exact `graph.ingest.query.batch` and `.prefix` filters and filters that
    capture several reserved subjects, which exercise the first-in-table-order rule.
  - Oracle, written in the test: a concrete-subject matcher (`*` matches one token, `>` one or more). It decides a
    capture by testing every concrete subject of the reserved pattern of up to 5 tokens over `{graph, ingest, query,
    mutation, entity, batch, prefix, z}`. Five tokens is enough, since a filter of at most 4 tokens that overlaps the
    mutation family has a witness of at most 4 tokens.
  - The test checks `SubjectsOverlap` and the reported first reserved subject against the oracle, and counts captured
    and clear filters, so both branches are shown to run.
- **Clash parity: examples enough.** Composition only calls `ConflictWith`. Its combinations (transport, wildcard
  and spelled hosts, port, zone) are proven in `component` (`TestRegistryRefusesNetworkClaimsThatCannotBothBind`,
  `TestRegistryAdmitsNetworkClaimsThatCanBothBind`). Composition's table runs the four probe pairs, the same-text
  pair and two controls through both `CreateComponent` and `Validate`. Expected text is written as literals: the
  finding message equals it, and the `CreateComponent` error contains it.

## Proving tests and mutants

The carried pin tests (validate, catalog and mermaid, with helpers adapted to `types.ComponentType`) plus the new
tests above. `TestDeclarationConflictWithMatchesAdmission` includes a case with two claims on each side whose only
clash is between the candidate's second claim and the holder's first.

Predicted detections:

- M1: `ConflictWith` compares `ResourceID()` text.
- M2: `ConflictWith` visits `d`'s claims before `held`'s; detected by the two-claims case.
- M3: composition's exclusive check removed.
- M4: the reserved check removed.
- M5: `SubjectsOverlap` replaced by `SubjectCovers(filter, reserved)`.
- M6: `SubjectFamily` dropped from the reserved set.
- M7: explicit `streams` not checked.
- M8: JetStream outputs not checked.
- M9: `reserved_subject_capture` severity becomes warning.
- M10: the shared edge comparator drops the `ConnectionID` key.

Predicted survivor: S1, a suggestion reworded.

## Pin ADRs

ADR-100 binds (inventory). It is carried as `docs/adr/100-compositions-are-validated-diagrams-are-projections.md`,
with a port note in the ADR-066 form and an `adapt` ledger row. The changes are markdownlint wrapping and a note
saying:

- decision 4's CLI is `composition/cli` (change 7);
- decision 5 names SemStreams packages SemEngine never had;
- ADR-026, ADR-029, ADR-094, ADR-095 and ADR-096 are not ported.

No decision changes, and decision 3 is strengthened, since the offline exclusive judgment now matches admission. The
changes are at spec level, not ADR level: `AssertValid` dropped (pin spec `:219`, ruling 1), a 14th finding type, the
self-clash outcome and overlap for #16. ADR-094 and ADR-096 govern `service`. There is no pin guidance to carry.

## Ledger, coverage, issues, order

- **Row `composition`:** `repair-before-port` (03B D11, #16 share), destination `composition`.
  - Contract: the `composition-validation` spec, the reserved-subject requirement of `graph-transport-boundary`, and
    ADR-100 decisions 1-3.
  - Dependencies: `component`, `config`, `graph`, `internal/flowgraph`, `internal/graphmutation`, `types`; no
    goroutine, no I/O.
  - `known_risks`: the drops and changes above, and the not-covered factory case.
- **Row `component`:** the contract gains `ConflictWith`; `known_risks` records the removed field.
- **Row `component/flowgraph`:** the contract gains `SubjectsOverlap`.
- **Coverage:** `composition` is on 03B D10's ruled list (`design.md:458`), so add `"composition unit"` to
  `scripts/cover-check.sh`. Pin baseline 89.6%; the gate is 80%.
- **Issues:** `Closes #338`. These stay open:
  - #16: this PR carries only the composition share; the rest needs `service` and change 4;
  - #280: decided with `service`;
  - #9: `Addresses #9`.

  The PR body says "retires" or "stays open" and never puts a closing word near #16.
- **Merge order:** before the `service` component-manager PR. #341 edits `config/config.go`, `load_test.go`,
  `README.md` and `doc.go`; this PR edits `config/example_config.json` and `config_test.go`. They share no file, so
  either order works. #343 is docs only.

## Owner questions

- **Q1. Rewrite `config/example_config.json`?** Recommended: yes.
  - The file names five factories (udp, robotics, websocket, graph, objectstore). None of them is registered in
    SemEngine; the only factory registered in the tree is graph-ingest. Validated as it stands, it gives five
    unknown-factory errors, so ruling 4's test cannot pass on this file.
  - The rewrite keeps the platform, NATS and services blocks and declares one graph-ingest component. Its only
    reader, `TestLoader_ExampleConfig` (`config/config_test.go:238`), is updated.
  - Answer no: the test validates a new file under `composition/testdata/`, and the stale example stays.
- **Q2. Add a 14th finding type, `reserved_subject_capture`?** Recommended: yes, so a product can tell this refusal
  apart from a wiring fault. Answer no: it is reported as `connection_pattern_error`, and the vocabulary stays at the
  pin's 13 types.
