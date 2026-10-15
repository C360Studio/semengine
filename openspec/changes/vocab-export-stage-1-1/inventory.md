# Inventory: vocab-export-stage-1-1

## Question and scope

What upstream seams in `semengine/vocabulary` and `semengine/vocabulary/export` (`Stage 1.1`, issue `#256`) are required
by the bootstrap designs of `semvocab` (`C360Studio/semvocab#1` Phase 2 and Phase 3) and `semsparql`
(`C360Studio/semsparql#1` §2.2.1, §2.3, and Task 0.2) so that:

1. `semvocab/shacl` can validate compiled `PackSpec` and `ProjectionSpec` structures ahead of time with zero global
   state mutation;
2. `semsparql/irimap` can compute entity and non-entity subject IRIs byte-identically to `vocabulary/export` via an
   exported `export.SubjectIRI(subject, base string) string`; and
3. `vocabulary/export` can serialize unfolded `ProjectionSpec` triple chains containing W3C `BLANK_NODE_LABEL` blank
   nodes (`_:label`) in `Triple.Subject` and `Triple.Object` and full `http://` / `https://` IRIs in `Triple.Predicate`
   across N-Triples, Turtle, and JSON-LD without mangling?

Repositories read (`docs/inventory-scope.md` and owner ruling of 2026-10-15 on `#256`):

- `semengine` (current `main` at `7e2724a4f9e2424c74b34ae47c605df9c72be257`).

## Open pull requests and merge order

Checked via `gh pr list --state open`:

- **PR #255** (`claude/simplify-errs-classify`, draft, `#167`): touches `pkg/errs/`, `natsclient/`, `pkg/projection/`,
  `pkg/lifecycle/`, and `processor/graph-ingest/`.
- **PRs #242–#248** (`dependabot/*`): dependency bumps (`go.mod`, `.github/workflows/`, `package.json`).

None of the open PRs touch `vocabulary/` or `vocabulary/export/`. Merge order is independent; whichever merges second
resolves any adjacent edit in `docs/admission-ledger.yaml`.

## Open issues naming carried packages

Checked via `gh issue list --state open` for `vocabulary` and `vocabulary/export`:

- **#256** (`feat(vocabulary): Stage 1.1 pure PackSpec/ProjectionSpec validation, export.SubjectIRI, and
  blank-node/IRI-predicate export support`): **retired by this PR** (`Closes #256`).
- **#90** (`decision: surface #48 left exported with no reader — vocabulary's domain predicates (Q17) and the untested
  pkg/security`, `status:needs-decision`): **stays** under Bounded port scope (scheduled in `#223` PR 3).
- **#219** (`port-refactor: pkg/types, message, payloadregistry & vocabulary — unify Validate/IsValid, drop message
  aliases, and zero-alloc ParsePredicate`): **stays** under Bounded port scope (scheduled in `#223` PR 3).
- **#223** (`epic: pre-04a-03 simplification wave`): **stays** (tracking epic).
- **#9** (`epic: Slice 04A — tier-0 graph foundation`): **addressed** (`Addresses #9`).

## Current code measurements (`7e2724a4f9e2424c74b34ae47c605df9c72be257`)

1. **`vocabulary/projection.go` (120 lines)**:
   - `validateProjectionSpec(spec *ProjectionSpec) error` is unexported (`:58-89`), accepts any URI scheme via
     `validateNonEmptyIRI`, and checks `NodeClassIRI != ""` and `ValuePredicateIRI == ""` on non-terminal steps
     (`i < len(spec.Steps)-1`), but does not enforce `http://` or `https://` on projection step IRIs or require
     `ValuePredicateIRI != ""` on the terminal step (`i == len(spec.Steps)-1`).
2. **`vocabulary/pack.go` (350 lines)**:
   - `stagePackLocked(pack PackSpec)` (`:210-249`) validates prefixes, predicates, and entity types while staging into a
     copy of the global `predicateRegistry` and `entityTypeRegistry` under `registryMu.Lock()`. It has no exported pure
     validation entry point (`(p *PackSpec) Validate() error` / `ValidatePack(pack *PackSpec) error`), does not reject an
     empty `pack.Name`, and does not reject intra-pack duplicate prefix, predicate, or entity-type keys when they repeat
     the same namespace or overwrite an earlier entry in the same pack.
3. **`vocabulary/export/export.go` (218 lines), `object.go` (429 lines), `ntriples.go` (52 lines), `turtle.go` (120
   lines), `jsonld.go` (148 lines)**:
   - `subjectToIRI(subject, base string) string` (`export.go:122-136`) is unexported and does not trim a trailing `/` on
     `base` or default an empty `base` to `vocabulary.SemEngineBase`.
   - `resolveSubjectIRI` (`export.go:139-144`) rewrites blank-node subjects (`_:b_1`) to
     `{base}/subjects/_:b_1` (or passes them to `SubjectIRIFunc`), and `writeNTriples` / `writeTurtleBody` wrap every
     uncompacted subject in `<...>`.
   - `classifyObject` (`object.go:87-106`), `classifyWithExplicitDatatype` (`object.go:124-137`),
     `classifyByDeclaredDataType` (`object.go:152-191`), and `classifyString` (`object.go:330-348`) treat blank-node
     strings (`_:b_1`) as `xsd:string` literals (`"\"_:b_1\""`) instead of blank-node resources.
   - `resolvePredicateIRI(predicate, base string) string` (`export.go:203-217`) rewrites unregistered predicates that
     are already valid `http://` or `https://` IRIs into `{base}/predicates/{dots->slashes}`.

## Governing decision records

- **ADR-107** (`docs/adr/107-semantic-web-vocabulary-lives-at-the-export-edge.md`): Semantic Web serialization and RDF
  syntax rules live in `vocabulary/export`, while `vocabulary` holds declarative metadata and registration contracts.
- **ADR-102** (`docs/adr/102-entity-id-segment-semantics.md`): decision O-10 governs the canonical 6-segment entity IRI
  path (`{base}/entities/{org}/{platform}/{system}/{domain}/{type}/{instance}`).
