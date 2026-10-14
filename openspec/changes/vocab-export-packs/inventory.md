# Inventory: vocab-export-packs

## Question and scope

What changes in `semengine` (`Stage 1`) are required so that `semvocab` can publish BFO 2020, CCO, and Defense and
Intelligence Enterprise Knowledge Model (DIEKM) ontology packs (in Go and declarative JSON/YAML) and `semsparql` can
serve a read-only W3C SPARQL 1.1 Protocol endpoint over `semengine`'s graph without bringing OWL/RDF machinery or
domain ontologies into `semengine` core?

Repositories read (`docs/inventory-scope.md` and owner ruling of 2026-10-14 on `#234`):

- `semengine` (current tree at `8b800149dffd22b10418466427bf778ef2bff313`).
- `semstreams` at the frozen pin `8b99efe9c66a4faa4fa509f9f62cc6bad8392128` (`vocabulary/`, `vocabulary/export/`,
  `vocabulary/bfo/`, `vocabulary/cco/`, and `docs/adr/107-semantic-web-vocabulary-lives-at-the-export-edge.md`).

## Open pull requests and merge order

Checked via `gh pr list --state open`:

- **PR #230** (`claude/test-infra-smaller-fixture`): touches `internal/harness/natsfixture/*` and
  `docs/admission-ledger.yaml`.
- **PR #231** (`gemini/cache-collapse`): touches `internal/cache/*` and `docs/admission-ledger.yaml`.

Neither open PR touches `vocabulary/`, `vocabulary/export/`, `docs/adr/107-*`, or `docs/inventory-scope.md`. Merge
order is independent; whichever merges second resolves any adjacent append in `docs/admission-ledger.yaml`.

## Open issues naming carried packages

Checked via `gh issue list --state open` for `vocabulary` and `vocabulary/export`:

- **#234** (`vocabulary: port vocabulary/export and add ontology pack & projection metadata seams`): **retired by this
  PR** (`Closes #234`).
- **#237** (`vocabulary, vocabulary/export: enforce Turtle PN_PREFIX syntax and fail-closed batch prefix registration`):
  **retired by this PR** (`Closes #237`).
- **#238** (`vocabulary/export: namespace aliases can emit undeclared Turtle prefixes`): **retired by this PR**
  (`Closes #238`).
- **#239** (`vocabulary/export: xsd:dateTime serialization truncates fractional seconds`): **retired by this PR**
  (`Closes #239`).
- **#90** (`vocabulary: remaining domain predicates in predicates.go`, `status:needs-decision`): **stays** under
  Bounded port scope (scheduled in `#223` PR 3).
- **#219** (`port-refactor: pkg/types, message, payloadregistry & vocabulary`): **stays** under Bounded port scope
  (scheduled in `#223` PR 3).
- **#223** (`epic: pre-04a-03 simplification wave`): **stays** (tracking epic).
- **#9** (`epic: Slice 04A — tier-0 graph foundation`): **addressed** (`Addresses #9`).

## Pin probe: `vocabulary/export` (`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`)

1. **Line counts (non-test `.go`)**: `doc.go` (28) + `export.go` (217) + `jsonld.go` (119) + `ntriples.go` (51) +
   `object.go` (410) + `prefix.go` (163) + `turtle.go` (119) = **1,107 lines** across 7 files (well under the 3,000-line
   single-PR bound). 8 test files (`datatype_prop_test.go`, `datatype_roundtrip_test.go`, `export_test.go`,
   `jsonld_test.go`, `ntriples_test.go`, `object_test.go`, `prefix_test.go`, `turtle_test.go`) plus 2 Rapid seed files
   under `testdata/rapid/TestPropDeclaredIntegerNeverFabricates/`.
2. **Test-text check (`time.Sleep`, `.Skip(`, `//go:build`)**: 0 matches across `vocabulary/export/`.
3. **Unit tests under `-race -count=1 -cpu 1`**: `ok github.com/c360studio/semstreams/vocabulary/export 4.123s`.
4. **Shuffled 5x repeat (`-count=5 -cpu 1 -shuffle=on`)**:
   `ok github.com/c360studio/semstreams/vocabulary/export 0.344s`.

## Governing decision records at the pin

- **ADR-107** (`docs/adr/107-semantic-web-vocabulary-lives-at-the-export-edge.md` at `8b99efe9`): binding on
  `vocabulary` (`PredicateDatatype`, `WithDatatype`) and `vocabulary/export` (typed literal formatting, IRI object
  emission, and keeping Semantic Web syntax at the export edge). Ported in this change to
  `docs/adr/107-semantic-web-vocabulary-lives-at-the-export-edge.md` with a SemEngine port header and ledger row.
- **ADR-102** (`docs/adr/102-entity-id-segment-semantics.md`, already ported): decision O-10 governs
  `vocabulary/export/export.go:127-130` (`buildEntityIRI` canonical 6-segment path order).
- **ADR-074** (`docs/adr/074-canonical-predicate-contract.md` at `8b99efe9`): governs 3-level dotted predicate grammar,
  already implemented in `vocabulary/predicate_syntax.go`.

## Shape sweep of `vocabulary/export` at the pin

| ID | Pin location | Observation | Disposition |
| --- | --- | --- | --- |
| S1 | `export.go:55,118`, `prefix.go:36,95`, and 5 test files | References `vocabulary.SemStreamsBase` (`https://semstreams.semanticstream.ing`), renamed in SemEngine (`#69`) to `vocabulary.SemEngineBase` (`https://semengine.semanticstream.ing`). | **Required** (build/verify gate): update to `vocabulary.SemEngineBase` and update default base IRI strings in tests. |
| S2 | `export_test.go:174,258,270`, `jsonld_test.go:72`, `turtle_test.go:175` | Calls `vocabulary.ClearRegistry()` without restoring built-in `init()` registrations. | **Required** (test isolation): precede `ClearRegistry()` with `defer vocabulary.SnapshotRegistry()()`. |
| S3 | `vocabulary/export/*.go` | Checked for context roots, goroutines, I/O, `GenericJSON` payload construction, and dead exported surface. | **None needed**: 0 context roots, 0 goroutines, 0 I/O, 0 `GenericJSONPayload` uses (`jsonld.go` serializes a standard JSON-LD document map), and all exported symbols (`Serialize`, `SerializeToString`, `Format` (`NTriples`, `Turtle`, `JSONLD`), `Option` (`WithBaseIRI`, `WithSubjectIRIFunc`), and `Register`) are live contract surface. |
| S4 | `prefix.go:47,108`, `object.go:257,290` | Review findings `#237` (Turtle `PN_PREFIX` validation and atomic batch prefix registration `RegisterPrefixes`), `#238` (deterministic prefix tie-breaking in `compact` for namespace aliases), and `#239` (`time.RFC3339Nano` fractional-second preservation for `xsd:dateTime`). | **Required** (closes `#237`, `#238`, `#239` in this PR). |

## Disposition of `vocabulary/bfo` and `vocabulary/cco`

At the pin, `vocabulary/bfo` and `vocabulary/cco` have no caller in the ported packages and only one reference in the
remaining tier-0 set (`pkg/fusion/fusionvocab/signals.go:16` in Change 5). Per the owner's ruling on `#234`, upper and
domain ontologies (`bfo`, `cco`, DIEKM) live in the sister repository `semvocab`, not in `semengine` core.
`docs/admission-ledger.yaml` records `vocabulary/bfo` and `vocabulary/cco` as `defer-exclude` (homed in `semvocab`).
