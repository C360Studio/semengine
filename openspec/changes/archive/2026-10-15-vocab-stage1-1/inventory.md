# Inventory: vocab-stage1-1

base: 7e2724a4f9e2424c74b34ae47c605df9c72be257

## Problem Statement

Issue `C360Studio/semengine#256` (`feat(vocabulary): Stage 1.1 pure PackSpec/ProjectionSpec validation, export.SubjectIRI,
and blank-node/IRI-predicate export support`, which `Addresses #9` and unblocks `C360Studio/semvocab#1` and
`C360Studio/semsparql#1`) reports three follow-up gaps across `vocabulary` and `vocabulary/export` after PR `#235`
(`vocab-export-packs`):

1. **Side-effect-free validation seams in `vocabulary` (`semvocab#1` Phase 2):**
   - `PrefixSpec.Validate() error` is exported (`vocabulary/pack.go:23`), whereas projection validation is unexported
     (`validateProjectionSpec(spec *ProjectionSpec) error` at `vocabulary/projection.go:58`) and pack validation is
     coupled to `RegisterPack` (`stagePackLocked(pack PackSpec)` at `vocabulary/pack.go:210`), which acquires
     `registryMu.Lock()` (`vocabulary/pack.go:191`) and stages against the live `predicateRegistry` and
     `entityTypeRegistry`.
   - External tooling (`semvocab` CLI `semvocab check` and `semvocab/shacl.Generate`) needs to validate decoded
     `PackSpec` and `ProjectionSpec` values without mutating or reading global registry state.

2. **Exported subject IRI helper in `vocabulary/export` (`semsparql#1` §2.2.1 & Task 0.2):**
   - `vocabulary/export` maps 6-part entity IDs (`{org}.{platform}.{system}.{domain}.{type}.{instance}`) to
     `{base}/entities/{org}/{platform}/{system}/{domain}/{type}/{instance}` and non-entity subjects to
     `{base}/subjects/{dot-to-slash}` via unexported `subjectToIRI(subject, base string) string`
     (`vocabulary/export/export.go:121`).
   - External SPARQL translation (`semsparql/irimap`) must construct and match subject IRIs byte-identically to
     `vocabulary/export` without an exported helper.

3. **Blank-node (`_:label`) and full-IRI predicate handling in `vocabulary/export` (`semsparql#1` §2.3 &
   `semvocab#1` Phase 3):**
   - When `semsparql` unfolds a reified `ProjectionSpec` chain into `[]message.Triple` for `CONSTRUCT`, `DESCRIBE`, or
     `POST /export` serialization via `export.Serialize`, intermediate nodes use blank-node identifiers (`"_:b_1"`,
     `"_:b_2"`) as `Triple.Subject` and `Triple.Object` (with empty `Datatype` or `Datatype: "@id"`), and predicates
     carry full IRIs (`"http://www.w3.org/1999/02/22-rdf-syntax-ns#type"`, `step.PredicateIRI`,
     `step.ValuePredicateIRI`) that are not registered as 3-part dotted predicate names.
   - Today in `vocabulary/export`:
     - `subjectToIRI` (`vocabulary/export/export.go:121-136`) rewrites `"_:b_1"` to `"{base}/subjects/_:b_1"`, and
       `writeNTriples` (`vocabulary/export/ntriples.go:29`) / `formatTurtleTerm` (`vocabulary/export/turtle.go:86-91`)
       wrap subjects in `<...>`.
     - `classifyWithExplicitDatatype` (`vocabulary/export/object.go:124-137`) and `classifyString`
       (`vocabulary/export/object.go:330-348`) treat `"_:b_1"` as an `xsd:string` literal (`"\"_:b_1\""`).
     - `resolvePredicateIRI` (`vocabulary/export/export.go:203-217`) looks up
       `vocabulary.GetPredicateMetadata(predicate)` and falls back to replacing `.` with `/` under
       `{base}/predicates/{path}`, even when `predicate` is already an `http://` or `https://` IRI.

## Surface Inventory

### Category 1: The Claimed Gap

Commands run:

- `gopls workspace_symbol -matcher=fuzzy Validate`
- `gopls workspace_symbol -matcher=fuzzy SubjectIRI`
- `git grep -n 'ValidateProjectionSpec\|ValidatePack\|SubjectIRI\|subjectToIRI\|resolvePredicateIRI\|isAbsoluteIRI'`

#### 1A. `vocabulary/projection.go` (ProjectionSpec validation)

- `vocabulary/projection.go:12` — `type ProjectionStep struct {`
- `vocabulary/projection.go:15` — `PredicateIRI string`
- `vocabulary/projection.go:20` — `NodeClassIRI string`
- `vocabulary/projection.go:25` — `ValuePredicateIRI string`
- `vocabulary/projection.go:32` — `type ProjectionSpec struct {`
- `vocabulary/projection.go:33` — `Steps []ProjectionStep`
- `vocabulary/projection.go:39` — `func WithProjection(spec ProjectionSpec) Option {`
- `vocabulary/projection.go:49` — `func cloneProjectionSpec(spec *ProjectionSpec) *ProjectionSpec {`
- `vocabulary/projection.go:58` — `func validateProjectionSpec(spec *ProjectionSpec) error {`
- `vocabulary/projection.go:62` — `if len(spec.Steps) == 0 {`
- `vocabulary/projection.go:66` — `if err := validateNonEmptyIRI(step.PredicateIRI, ...)`
- `vocabulary/projection.go:69` — `if step.NodeClassIRI != "" {`
- `vocabulary/projection.go:74` — `if step.ValuePredicateIRI != "" {`
- `vocabulary/projection.go:79` — `if i < len(spec.Steps)-1 {`
- `vocabulary/projection.go:80` — `if step.NodeClassIRI == "" {`
- `vocabulary/projection.go:83` — `if step.ValuePredicateIRI != "" {`
- `vocabulary/projection.go:91` — `func validateNonEmptyIRI(iri, field string) error {`

Measured state in `vocabulary/projection.go`:

- Neither `(s ProjectionSpec) Validate() error` nor `ValidateProjectionSpec` is exported
  (`git grep -n 'ValidateProjectionSpec'` returned 0 matches).
- Premise observation on terminal `ValuePredicateIRI`: `vocabulary/projection.go:22-25` (`// ValuePredicateIRI is the
  optional datatype- or object-property IRI that`) and `openspec/specs/vocabulary-packs/spec.md:57-60` currently allow an
  empty `ValuePredicateIRI` on the terminal step (`vocabulary/projection.go:74-86` only forbids `ValuePredicateIRI != ""`
  when `i < len(spec.Steps)-1`), whereas issue `#256` item 1 states that `ValuePredicateIRI` is forbidden on
  intermediate steps and required on the terminal step. In `vocabulary/projection_test.go:14-26`, the 2-step literal
  chain sets `ValuePredicateIRI` on the terminal step at `vocabulary/projection_test.go:23`
  (`ValuePredicateIRI: "http://www.ontologyrepository.com/CommonCoreOntologies/has_decimal_value",`).

#### 1B. `vocabulary/pack.go` (PackSpec validation)

- `vocabulary/pack.go:14` — `type PrefixSpec struct {`
- `vocabulary/pack.go:23` — `func (p PrefixSpec) Validate() error {`
- `vocabulary/pack.go:80` — `type PredicateSpec struct {`
- `vocabulary/pack.go:99` — `type EntityTypeSpec struct {`
- `vocabulary/pack.go:109` — `type PackSpec struct {`
- `vocabulary/pack.go:183` — `func RegisterPack(pack PackSpec, opts ...PackOption) error {`
- `vocabulary/pack.go:210` — `func stagePackLocked(pack PackSpec) (...)`
- `vocabulary/pack.go:251` — `func stagePredicateSpec(i int, spec PredicateSpec, ...)`
- `vocabulary/pack.go:318` — `func stageEntityTypeSpec(i int, spec EntityTypeSpec, ...)`

Measured state in `vocabulary/pack.go`:

- Neither `(p *PackSpec) Validate() error` nor `ValidatePack` is exported (`git grep -n 'ValidatePack'` returned 0
  matches).
- `stagePackLocked` (`vocabulary/pack.go:210-249`) seeds `stagedPredicates` from `predicateRegistry`
  (`vocabulary/pack.go:222-226`) and `stagedEntityTypes` from `entityTypeRegistry` (`vocabulary/pack.go:238-241`) under
  `registryMu.Lock()` (`vocabulary/pack.go:191`).
- `stagePackLocked` (`vocabulary/pack.go:211-220`) rejects conflicting duplicate prefixes within a pack
  (`existing != pfx.Namespace`), does not check `pack.Name == ""`, and merges repeated `PredicateSpec.Name` or
  `EntityTypeSpec.DomainType` entries within the same pack into `stagedPredicates` / `stagedEntityTypes` rather than
  rejecting intra-pack duplicates.

#### 1C. `vocabulary/export` (`SubjectIRI`, blank nodes, and full-IRI predicates)

- `vocabulary/export/export.go:117` — `func defaultSubjectIRI(subject string) string {`
- `vocabulary/export/export.go:121` — `func subjectToIRI(subject, base string) string {`
- `vocabulary/export/export.go:139` — `func resolveSubjectIRI(subject string, opts *options) string {`
- `vocabulary/export/export.go:153` — `func groupTriples(triples []message.Triple, opts *options) (...)`
- `vocabulary/export/export.go:185` — `func scanPrefixes(groups map[string][]resolvedTriple, pm *prefixMap) {`
- `vocabulary/export/export.go:203` — `func resolvePredicateIRI(predicate, base string) string {`
- `vocabulary/export/object.go:57` — `objectLiteral objectKind = iota`
- `vocabulary/export/object.go:58` — `objectResource`
- `vocabulary/export/object.go:87` — `func classifyObject(t message.Triple, opts *options) classifiedObject {`
- `vocabulary/export/object.go:124` — `func classifyWithExplicitDatatype(t message.Triple, opts *options) ...`
- `vocabulary/export/object.go:330` — `func classifyString(s string, opts *options) classifiedObject {`
- `vocabulary/export/object.go:354` — `func isAbsoluteIRI(s string) bool {`
- `vocabulary/export/ntriples.go:29` — `if _, err := fmt.Fprintf(w, "<%s> <%s> %s .\n", ...)`
- `vocabulary/export/ntriples.go:38` — `func formatNTriplesObject(obj classifiedObject) string {`
- `vocabulary/export/turtle.go:52` — `subjectStr := formatTurtleTerm(subIRI, pm)`
- `vocabulary/export/turtle.go:86` — `func formatTurtleTerm(iri string, pm *prefixMap) string {`
- `vocabulary/export/turtle.go:94` — `func formatTurtleObject(obj classifiedObject, pm *prefixMap) string {`
- `vocabulary/export/jsonld.go:37` — `node := map[string]any{"@id": subIRI}`
- `vocabulary/export/jsonld.go:89` — `func jsonldObjectValue(obj classifiedObject, pm *prefixMap) any {`

Measured state in `vocabulary/export`:

- `export.SubjectIRI` does not exist (`gopls workspace_symbol -matcher=fuzzy SubjectIRI` returned only
  `vocabulary/export/export.go:117` `defaultSubjectIRI`, `:121` `subjectToIRI`, `:139` `resolveSubjectIRI`, and `:69`
  `WithSubjectIRIFunc`).
- `subjectToIRI` (`vocabulary/export/export.go:121-136`) does not default an empty `base` to `vocabulary.SemEngineBase`
  or trim trailing `/` from `base`.
- No blank-node validation or formatting exists (`git grep -n '_:' vocabulary/` returned 0 matches).
- `resolvePredicateIRI` (`vocabulary/export/export.go:203-217`) has no pass-through for pre-resolved `http://` or
  `https://` IRIs.

### Category 2: Every Current Spelling of the Fact Being Modeled

Commands run:

- `gopls references vocabulary/projection.go:58:6`
- `gopls references vocabulary/projection.go:91:6`
- `gopls references vocabulary/pack.go:23:19`
- `gopls references vocabulary/pack.go:183:6`
- `gopls references vocabulary/pack.go:210:6`
- `gopls references vocabulary/export/export.go:121:6`
- `gopls references vocabulary/export/export.go:139:6`
- `gopls references vocabulary/export/export.go:203:6`
- `gopls references vocabulary/export/object.go:354:6`

1. **Pack and projection validation in `vocabulary`:**
   - `vocabulary/pack.go:23` — `func (p PrefixSpec) Validate() error {`
   - `vocabulary/projection.go:58` — `func validateProjectionSpec(spec *ProjectionSpec) error {`
   - `vocabulary/registry.go:375` — `func validatePredicateMetadataLocked(meta PredicateMetadata) error {`
   - `vocabulary/registry.go:379` — `func validatePredicateMetadataIn(meta PredicateMetadata, ...)`
   - `vocabulary/entity_type.go:128` — `func validateEntityTypeMetadata(meta EntityTypeMetadata) error {`
   - `vocabulary/pack.go:210` — `func stagePackLocked(pack PackSpec) (...)`
2. **IRI syntax validation across `vocabulary` and `vocabulary/export`:**
   - `vocabulary/projection.go:91` — `func validateNonEmptyIRI(iri, field string) error {`
   - `vocabulary/export/object.go:354` — `func isAbsoluteIRI(s string) bool {`
3. **Subject IRI formatting in `vocabulary/export`:**
   - `vocabulary/export/export.go:117` — `func defaultSubjectIRI(subject string) string {`
   - `vocabulary/export/export.go:121` — `func subjectToIRI(subject, base string) string {`
   - `vocabulary/export/export.go:139` — `func resolveSubjectIRI(subject string, opts *options) string {`
4. **Turtle / N-Triples identifier grammar checks in `vocabulary` and `vocabulary/export`:**
   - `vocabulary/pack.go:30` — `func validatePNPrefix(prefix string) error {`
   - `vocabulary/pack.go:51` — `func isPNCharsBase(r rune) bool {`
   - `vocabulary/pack.go:68` — `func isPNChars(r rune) bool {`
   - `vocabulary/export/prefix.go:148` — `func isValidPNLocal(local string) bool {`
   - `vocabulary/export/prefix.go:166` — `func isPNLocalStart(r rune) bool {`
   - `vocabulary/export/prefix.go:170` — `func isPNLocalChar(r rune) bool {`
   - `vocabulary/export/prefix.go:178` — `func isPNLocalBase(r rune) bool {`

### Category 3: Adjacent Claims on the Territory

Commands run:

- `gh pr list -R C360Studio/semengine --state open --json number,title,changedFiles,files`
- `gh issue list -R C360Studio/semengine --state open --search "vocabulary" --limit 200`
- `gh issue list -R C360Studio/semengine --state open --search "export" --limit 200`

1. **Current specs:**
   - `openspec/specs/vocabulary-packs/spec.md:55` — `### Requirement: Reification and projection path metadata on
     predicates`
   - `openspec/specs/vocabulary-packs/spec.md:69` — `### Requirement: Declarative JSON and YAML vocabulary packs load
     and register fail-closed`
2. **ADRs:**
   - `docs/adr/107-semantic-web-vocabulary-lives-at-the-export-edge.md:11` — `> - vocabulary and vocabulary/export are
     ported here; upper and domain ontologies (bfo, cco, DIEKM) live in the`
   - `docs/adr/107-semantic-web-vocabulary-lives-at-the-export-edge.md:59` — `**Semantic-web vocabulary belongs at the
     export edge, where interoperability is the job. It does not belong in the`
   - `docs/adr/102-entity-id-segment-semantics.md:47` — `gated-DAG family re-slots under agent, reserved set {agent,
     ops, graph}; O-10 the export IRI follows the canonical`
   - `docs/adr/102-entity-id-segment-semantics.md:132` — `value and the vocabulary export IRI path
     (vocabulary/export/export.go:123-126); both follow the canonical order`
3. **Admission ledger rows:**
   - `docs/admission-ledger.yaml:453` — `- source_path: vocabulary`
   - `docs/admission-ledger.yaml:459` — `destination: vocabulary`
   - `docs/admission-ledger.yaml:2306` — `- source_path: vocabulary/export`
   - `docs/admission-ledger.yaml:2312` — `destination: vocabulary/export`
4. **Open pull requests:**
   - `#255` (`fix(errs): unclassified errors fail closed; commit-unknown is not retryable (#167)`, draft): 18 changed
     files in `natsclient`, `pkg/errs`, `pkg/lifecycle`, `pkg/projection`,
     `processor/graph-ingest/commit_ambiguity_integration_test.go`, and
     `openspec/changes/simplify-04a-04-errs-classify/`. Zero file or capability overlap with `vocabulary` or
     `vocabulary/export`.
   - `#242`–`#248` (Dependabot PRs): dependency bumps in `go.mod`, `go.sum`, `package.json`, `.github/workflows/ci.yml`.
     Zero overlap with `vocabulary` or `vocabulary/export`.
5. **Open issues naming `vocabulary` or `vocabulary/export`:**
   - `#256` (`feat(vocabulary): Stage 1.1 pure PackSpec/ProjectionSpec validation, export.SubjectIRI, and
     blank-node/IRI-predicate export support`): target of this claim.
   - `#219` (`port-refactor: pkg/types, message, payloadregistry & vocabulary — unify Validate/IsValid, drop message
     aliases, and zero-alloc ParsePredicate`): concerns `IsValidPredicate` and `ParsePredicate` allocation in
     `vocabulary/predicate_contract.go` under epic `#223`.
   - `#90` (`decision: surface #48 left exported with no reader — vocabulary's domain predicates (Q17) and the untested
     pkg/security`, `status:needs-decision`): concerns domain predicate constants in `vocabulary/` under epic `#223`.
   - `#9` (`epic: Slice 04A — tier-0 graph foundation`): parent epic (`Addresses #9`).
   - `#223` (`epic: pre-04a-03 simplification wave`): tracking epic for `#219` and `#90`.
   - `#259` (`feat(graph-ingest): fixed-position wildcard pattern on graph.ingest.query.prefix (semsparql type scans)`):
     companion issue in `graph` and `processor/graph-ingest`.
   - Hits naming the word "vocabulary" or "export" rather than the package: `#189`, `#132`, `#161`, `#31`, `#110`,
     `#27`, `#134`, `#105`, `#94`, `#138`, `#85`, `#258`, `#36`, `#257`, `#78`, `#106`, `#199`, `#153`, `#169`, `#197`,
     `#135`, `#139`.

### Category 4: Who It Is For

- Pack and projection validation in `vocabulary`:
  - Consumer caller: `semvocab` (`docs/inventory-scope.md:21`), specifically `semvocab/shacl` (`semvocab#1` Phase 2) and
    `cmd/semvocab check`.
  - In-repo callers: `vocabulary/pack.go:210` (`stagePackLocked`) and `vocabulary/registry.go:388`
    (`validatePredicateMetadataIn`).
- Subject IRI mapping in `vocabulary/export`:
  - Consumer caller: `semsparql` (`docs/inventory-scope.md:22`), specifically `semsparql/irimap` (`semsparql#1` §2.2.1 &
    Task 0.2).
  - In-repo callers: `vocabulary/export/export.go:117` (`defaultSubjectIRI`) and `vocabulary/export/export.go:121`
    (`subjectToIRI`).
- Blank-node (`_:label`) and full-IRI predicate serialization in `vocabulary/export`:
  - Consumer callers: `semsparql` (`docs/inventory-scope.md:22`, `semsparql#1` §2.3 `UnfoldEntity` for `CONSTRUCT` /
    `DESCRIBE` / `POST /export`) and `semvocab` (`docs/inventory-scope.md:21`, `semvocab#1` Phase 3 external W3C SHACL
    validation).

### Category 5: The Problem Shape

1. **Admit-or-refuse value validation (`Validate() error`):**
   - Closest existing instance in the tree: `PrefixSpec.Validate() error` at `vocabulary/pack.go:23-28`.
2. **W3C Turtle / N-Triples token grammar validation:**
   - Closest existing instances in the tree: `validatePNPrefix` (`vocabulary/pack.go:30-49`) and `isValidPNLocal`
     (`vocabulary/export/prefix.go:148-164`).
3. **Same-class collision table:**
   - Not triggered: no durable state, communication channel, or runtime-coordination primitive is involved.

### Category 6: The Pin's Decision Records

- No new package is carried from the SemStreams pin in this change (`vocabulary` and `vocabulary/export` are already
  ported in `docs/admission-ledger.yaml:453` and `:2306`).
- Governing ADRs already in `docs/adr/`:
  - `docs/adr/102-entity-id-segment-semantics.md` (`:47`, `:132`: 6-part canonical entity ID order in
    `vocabulary/export/export.go`).
  - `docs/adr/107-semantic-web-vocabulary-lives-at-the-export-edge.md` (`:11-12`, `:59-70`: RDF serialization and
    ontology pack boundaries).

### Shape Sweep of `vocabulary/pack.go`, `vocabulary/projection.go`, and `vocabulary/export`

1. **A second spelling of one thing:**
   - `vocabulary/projection.go:91` (`validateNonEmptyIRI`) and `vocabulary/export/object.go:354` (`isAbsoluteIRI`) both
     check IRI character exclusions (`r <= 0x20` and ``<>"{}|^`\``), while `validateNonEmptyIRI` accepts any RFC 3986
     scheme and `isAbsoluteIRI` accepts `http://`, `https://`, and `urn:`.
2. **A copied shell:** None (`vocabulary` and `vocabulary/export` have no component lifecycle).
3. **A layered interface with one implementation:** None.
4. **Exported surface with no reader:** Covered by open issue `#90` (domain predicate constants in `vocabulary/`) and
   `#219` (`IsValidPredicate` in `vocabulary/predicate_contract.go`).
5. **Classification by error text:** None.

## Adopter Seam Inventory

External adopters (`docs/inventory-scope.md:21-22`): `C360Studio/semvocab` (`semvocab#1`) and `C360Studio/semsparql`
(`semsparql#1`).

1. **What must they know today?**
   - To validate a `PackSpec` today, an adopter must call `vocabulary.RegisterPack` (`vocabulary/pack.go:183`), which
     acquires `registryMu.Lock()` (`vocabulary/pack.go:191`) and mutates `predicateRegistry` and `entityTypeRegistry`
     unless the caller wraps the call in `vocabulary.SnapshotRegistry()()`. To validate a standalone `ProjectionSpec`
     today, an adopter must either re-implement `validateProjectionSpec` (`vocabulary/projection.go:58`) or call
     `vocabulary.Register` (`vocabulary/registry.go:301`), which panics on invalid input and mutates `predicateRegistry`.
   - To construct subject IRIs matching `vocabulary/export`, `semsparql/irimap` must know the unexported formula in
     `subjectToIRI` (`vocabulary/export/export.go:121-136`): 6-part `message.ParseEntityID` subjects become
     `{base}/entities/{org}/{platform}/{system}/{domain}/{type}/{instance}` and all other non-empty subjects become
     `{base}/subjects/{dots->slashes}`.
   - When `semsparql` passes unfolded reified triples (`_:b_...` subjects/objects and full HTTP(S) predicate IRIs) to
     `export.Serialize` today, the adopter must know that `vocabulary/export` has no blank-node or full-IRI predicate
     path.
2. **What happens if they do nothing today?**
   - Calling `RegisterPack` during `semvocab/shacl` ahead-of-time compilation mutates process-global `vocabulary` state;
     calling `Register` with an invalid `ProjectionSpec` panics.
   - If `semsparql/irimap` passes a base IRI with a trailing `/` or empty string into a copy of `subjectToIRI`, it
     produces double slashes (`https://example.org//entities/...`) or relative paths (`/entities/...`).
   - Passing `_:b_1` as `Triple.Subject` to `export.Serialize` today emits
     `<https://semengine.semanticstream.ing/subjects/_:b_1>`; passing `_:b_1` as `Triple.Object` emits the string
     literal `"_:b_1"`; and passing `"http://www.w3.org/1999/02/22-rdf-syntax-ns#type"` as `Triple.Predicate` emits
     `<https://semengine.semanticstream.ing/predicates/http://www/w3/org/1999/02/22-rdf-syntax-ns#type>`.
3. **Where do they find out today?**
   - Nowhere at compile time or runtime for `vocabulary/export` (silent mangling of blank nodes and IRI predicates in
     serialized RDF output, discovered only when downstream RDF/SHACL parsers reject or misinterpret the graph); runtime
     global state mutation or panic for `vocabulary` validation.
4. **What SHOULD they have to know (and the gap between 1 and 4)?**
   - Adopters should not have to snapshot/restore global registries or catch panics to validate a `PackSpec` or
     `ProjectionSpec`, should not have to duplicate `subjectToIRI` in `semsparql/irimap`, and should not have to write a
     second RDF serializer to emit unfolded `_:label` blank nodes and full-IRI predicates. Closing that gap is the scope
     of this design.
5. **Prefer observation to prediction:**
   - Today `semsparql` must predict `vocabulary/export`'s internal subject IRI formula, and `vocabulary/export` fails to
     observe that a subject/object is already a W3C blank node (`_:label`) or that a predicate is already an HTTP(S) IRI.
