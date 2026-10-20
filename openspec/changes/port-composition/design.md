# Design: port-composition

Pin `8b99efe9`. Base `24a3355`. Inventory: `inventory.md` (INVENTORY PASS). Owner rulings 1-4, 2026-10-20 (#9 comment
6295357983).

## Inventory correction (probe, 2026-10-20)

This is a throwaway test in the pin copy, since removed. One component declaring `tcp:0.0.0.0:8080` on an input and an
output yields one `connection_pattern_error`: "network port conflict on tcp:0.0.0.0:8080 ... both trying to bind". It
comes from the pin flowgraph's bind refusal, not from composition. The inventory's "no finding" read `validate.go`
alone and was wrong. On the tree, `Declare` refuses the component first (`component/registry.go:626`), so the outcome
becomes `port_declaration_error`. The same probe also ran an explicit stream on `graph.ingest.query.entity` and a
JetStream output on `graph.>`. Its one error is `config_invalid` (the derived stream has no bounds). Nothing names a
reserved subject, so the pin does not refuse a reserved-subject capture.

## Options: how the clash rule reaches composition

- O1, text compare as at the pin: a second spelling of the rule, which #338 rules out. The probe shows it admits four
  pairs the registry refuses.
- O2, `PortFacts.Clashes(PortFacts) bool`: one method, but composition would re-spell claim selection, holder order and
  the refusal text.
- **O3 (chosen):** `func (d Declaration) ConflictWith(held Declaration) error`. It returns nil, or a plain error with
  the registry's current text (`resource conflict: component '<d>' claims <c>, held by component '<held>' as <h>`) for
  the first clash. It visits `held`'s claims, then `d`'s, each inputs first and then outputs in declaration order, the
  order `checkResourceConflictsLocked` already uses (`component/registry.go:579-593`). The registry iterates holders
  in name order and wraps the error with `errs.WrapInvalid` (same text and class as today). Composition puts
  `err.Error()` in the finding message. The rule, the claim selection, the order and the text each have one home.
  Cost: one exported method. The removed `ExclusiveResources` field offsets it.
- O4, offline admission on `Registry`: the largest surface, overlapping `CreateComponent` (#91).
- O5, no offline check: breaks ADR-100 decision 3 (the same findings offline and at boot).

## Decision

- **`component`.** Add `ConflictWith`. `checkResourceConflictsLocked` calls it for each holder in name order.
  `exclusiveClaims` and `claimsClash` stay unexported. Remove `Declaration.ExclusiveResources` (`:70`, written
  `:315`, `:619`, cloned `:708`); `exclusiveResources` (`:626`) becomes a self-clash check that returns only an error.
  Tests that read the field (`registry_boot_admission_test.go:62,87-88,499`; `registry_declare_test.go:86-87`) read
  the facts' `ResourceID()` instead.
- **`composition`**: ported from the pin files except `assert.go`, with these changes.
  - Exclusive check: `validate.go:58-65,:139-155` call `declaration.ConflictWith(earlier)` over the declarations
    already accepted. Components are visited in name order, so the holder named is the first in name order. A
    refused declaration stays out of the graph, as at the pin.
  - Flowgraph adapts (inventory sweep item 4): drop the `IssueOptionalInterfaceUnused` case (`analyze.go:195`). Drop
    the suggestion "Give each network listener its own address" (`:49`). Reword the `TypeConnectionPatternError`
    comment (`findings.go:24`) to name only KV-writer and graph-mutation conflicts. Convert `registration.Type` with
    `string(...)` (`validate.go:111`, `catalog.go:67,72`).
  - #16: described under "#16 refusal" below.
- **Dropped:** `AssertValid` and its test (ruling 1). `shipped_configs_test.go`: its imports are not admitted
  (inventory), so the example-config test replaces it (ruling 4).
- **Simple leftovers, fixed now** (owner: simple leftovers now). One edge comparator, used by `graph.go:103-121` and
  `mermaid.go:24-42`. `portViews` (`graph.go:63-65`) calls `interfaceType` (`analyze.go:283`). `displayNames` drops
  `robotics`, `graph-processor`, `context-processor` (no factory at the pin) and `udp` (not admitted). Comments
  citing deleted SemStreams paths (`findings.go:66-68`, `analyze.go:212`) state the rule instead. `doc.go` names
  `service`'s component manager as the boot caller to come, not as present code.
- **Optional, kept.** The pre-checks in `declareInstance` (sweep 1b) stay: dropping them means classifying `Declare`'s
  errors by text (shape 5). The `index < len(facts)` guards stay until #280 item 2 is decided with `service`.
- **No new issue.** The only cross-package leftover is #280, which is already filed.

## #16 refusal

`Analyze` reads the reserved subjects from `graph.QueryVerbs()` (`graph/query_verbs.go:25`) and
`graphmutation.SubjectFamily` (`internal/graphmutation/protocol.go:18`); it never spells them, under the guard
`internal/harness/contract/reservedsubjects_test.go:43`. It checks two kinds of filter:

- each subject of an explicit `streams` entry (`config/streams.go:57`), with finding component `config`;
- each stream subject of a JetStream output, from which provisioning derives a stream, with the instance and port.

A filter is refused when one concrete subject matches both the filter and a reserved subject. The test is overlap, not
cover: `*.mutation.create` captures `graph.mutation.create` though neither pattern covers the other. The result is one
`reserved_subject_capture` error per filter, naming the stream, the filter and the first reserved subject it captures
(query verbs in table order, then the mutation family). Whether `internal/flowgraph`'s `matchNATSPattern` answers
overlap for two wildcard patterns is settled by the enumeration test below. If it does, it is exported there
(internal, so no public surface); if not, a small overlap function is written there. Framework streams (`logs.>`,
`health.>`, `metrics.>`, `flows.>`, `governance.verdict.>`, `$JS.EVENT...`; `config/streams.go:97-174`) overlap no
reserved subject and are not inputs to `Analyze`. #16 stays open: the real-NATS PubAck regression and refusal before
readiness come with `service`; graph-query's subjects come with change 4 (#106).

## Ruling to implementation

| Ruling / required finding | Implementation | Proving test |
| --- | --- | --- |
| #338; sweep 1a | `component.Declaration.ConflictWith`; `composition/validate.go` exclusive check | `TestDeclarationConflictWithMatchesAdmission`, `TestValidateExclusiveClashMatchesAdmission`, pin `TestValidateReportsExclusiveResourceConflict` |
| Ruling 3 | `component/registry.go:70,315,619,708` field removed | build; `TestRegistryRefusesOneComponentClaimingAnAddressTwice` |
| Self-clash outcome (probe) | `Declare` refusal becomes a finding | `TestValidateSelfClashIsPortDeclarationError` |
| Ruling 1; T-B1 | `assert.go` not ported | `TestImportGraph` |
| Ruling 2; 03B D11 `:481` | reserved-subject check in `Analyze` | `TestValidateRefusesStreamCapturingReservedSubject`, `TestReservedSubjectOverlapMatchesEnumeration` |
| Ruling 4; pin spec `:347` | `config/example_config.json` rewritten | `TestValidateExampleConfigHasNoErrorFindings`, `TestLoader_ExampleConfig` |
| Sweep 4 (flowgraph, type) | `analyze.go:49,195`, `findings.go:24`, three `string()` | build; pin `TestValidateFindingsVocabularyIsClosed` |
| ADR-100 decisions 1-3 | package as ported | the pin's validate, catalog and mermaid tests |

## Generated checks

- **Reserved overlap: generated check.** It is a subject grammar with wildcards on both sides, an input format with
  many interacting cases. The check enumerates every filter of 1 to 4 tokens over `{graph, ingest, query, mutation,
  entity, z, *, >}`, with `>` only last. The oracle, written in the test, is a concrete-subject matcher (`*` is one
  token, `>` is one or more). It decides a capture by testing every concrete subject of the reserved pattern up to 5
  tokens over `{graph, ingest, query, mutation, entity, batch, prefix, z}`. That witness set is enough because a
  filter of at most 4 tokens that overlaps the family has a witness of at most 5 tokens. The test counts captured and
  clear filters, so both sides are shown to run.
- **Clash parity: examples enough.** Composition calls `ConflictWith` only. Its combinations (transport, wildcard and
  spelled hosts, port, zone) are already proven in `component` (`TestRegistryRefusesNetworkClaimsThatCannotBothBind`,
  `TestRegistryAdmitsNetworkClaimsThatCanBothBind`). Composition's table runs the four probe pairs, same-text and two
  controls through both `CreateComponent` and `Validate`, with literal expected text.

## Proving tests and mutants

These are the carried pin tests (validate, catalog, mermaid; helpers adapted to `types.ComponentType`) plus the new
tests above. Predicted detections:

- M1: `ConflictWith` compares `ResourceID()` text.
- M2: `ConflictWith` visits `d`'s claims before `held`'s.
- M3: composition's exclusive check removed.
- M4: the reserved check removed.
- M5: overlap replaced by `SubjectCovers(filter, reserved)`.
- M6: `SubjectFamily` dropped from the reserved set.
- M7: explicit `streams` not checked.
- M8: JetStream outputs not checked.
- M9: `reserved_subject_capture` severity becomes warning.
- M10: the shared edge comparator drops the `ConnectionID` key.

Predicted survivor: S1, the suggestion text reworded.

## Pin ADRs

ADR-100 binds (inventory). It is carried as `docs/adr/100-compositions-are-validated-diagrams-are-projections.md`,
with a port note in the ADR-066 form and an `adapt` ledger row: markdownlint wrapping, and the note says decision 4's
CLI is `composition/cli` (change 7), decision 5 names SemStreams packages SemEngine never had, and ADR-026, ADR-029,
ADR-094, ADR-095 and ADR-096 are not ported. No decision changes. Decision 3 is strengthened, since the offline
exclusive judgment now matches admission. Changed at spec level, not ADR level: `AssertValid` dropped (pin spec
`:219`, ruling 1); a 14th finding type; the self-clash outcome. ADR-094 and ADR-096 govern `service`. No pin guidance
to carry.

## Ledger, coverage, issues, order

- Row `composition`: `repair-before-port` (03B D11, #16 share), destination `composition`. Contract: the
  `composition-validation` spec and the reserved-subject requirement of `graph-transport-boundary`, ADR-100 decisions
  1-3. Dependencies: `component`, `config`, `graph`, `internal/flowgraph`, `internal/graphmutation`, `types`; no
  goroutine, no I/O. `known_risks` lists the drops and changes above.
- Row `component`: contract gains `ConflictWith`; `known_risks` records the removed field.
- Coverage: `composition` is on 03B D10's ruled list (`design.md:458`), so add `"composition unit"` to
  `scripts/cover-check.sh`. Pin baseline 89.6%; gate 80%.
- Issues: `Closes #338`. These stay open: #16 (composition share only; its other items need `service` and change 4),
  #280 (with `service`), #9 (`Addresses #9`). The PR body says "retires" or "stays open" and never puts a closing
  word near #16.
- Merge order: before the `service` component-manager PR. #341 edits `config/config.go`, `load_test.go`, `README.md`
  and `doc.go`, and this PR edits `config/example_config.json` and `config_test.go`: no shared file, either order.
  #343 is docs only.

## Owner questions

- **Q1. Rewrite `config/example_config.json`?** Recommended: yes. It names five factories (udp, robotics, websocket,
  graph, objectstore), and none of them is registered in SemEngine; the only factory registered in the tree is
  graph-ingest. Validating it as it stands gives five unknown-factory errors, so ruling 4's test cannot pass on this
  file. The rewrite keeps the platform, NATS and services blocks and declares one graph-ingest component. Its only
  reader, `TestLoader_ExampleConfig` (`config/config_test.go:238`), is updated. Answer no: the test validates a new
  file under `composition/testdata/` instead, and the stale example stays as it is.
- **Q2. Add a 14th finding type, `reserved_subject_capture`?** Recommended: yes. A product can tell this refusal apart
  from a wiring fault. Answer no: it is reported as `connection_pattern_error`, and the vocabulary stays at the pin's
  13 types.
