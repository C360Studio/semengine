<!-- markdownlint-disable-next-line MD013 -->
# Surface audit: `vocabulary`, `message` capability interfaces, `pkg/security` (PR #48, `setup-04a-01-floor`)

Architect inventory, read-only. Requested by the #48 session under the owner's rulings of 2026-10-04 (#9 comment
5985697767), triggered by the retrospective read (PR #48 comment 5985648705).

- **base (worktree):** `/Users/coby/Code/c360/semengine-wt/claude/setup-04a-01-floor` at `931d90f`
- **pin:** SemStreams `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, read from `gh api
  repos/C360Studio/semstreams/tarball/<sha>`
- **semsource:** `e4febc0d2e23bf33811edc20b9a4159cf5275dc4` (HEAD, go.mod already at the pin pseudo-version)
- **semconnect:** PR #74 head `dff1265708910ef411ab6f85b16fd9e94663e997` (go.mod at the pin). semconnect `main`
  (`d0d06e00`, beta.160) was also read; it differs only in reading four fewer vocabulary names
  (`DataTypeBool`, `DataTypeEntityID`, `DataTypeFloat`, `IndexingProfileContent`).
- No git command was run against any sister checkout. Tarballs live in the scratchpad (`src/`), local only.

## Scope under `docs/inventory-scope.md`

Question: who reads each exported identifier of `vocabulary`, the ten `message` capability interfaces and
`pkg/security`.

| Repository | Admitted for this question? | Read |
| --- | --- | --- |
| semstreams | yes: code facts at the pin | whole tree, split into the 65 admitted packages (`docs/tier1-cross-check.md` "In both" + "In tier 0 only") and the rest |
| semsource | yes: "dependency closure and imports" (symbol level) | whole tree |
| semconnect | yes: "vocabulary/export seams" named explicitly | whole tree at PR #74 head |
| semteams | **no**: admitted only for `processor/rule`, `pkg/lifecycle` and agentic seams | not read. **Evidence gap** |
| semboids | **no**: admitted only for workload and `pkg/lifecycle`/`processor/rule` | not read. **Evidence gap** |
| semembed, seminstruct, others | no | not read |

If the owner wants semteams/semboids symbol use of these three packages counted, that is a ruling on #22.

## Method (reproducible)

All counts come from two small Go AST tools in the scratchpad (`exp/main.go`, `rd/main.go`). Neither is a grep.

1. `exp <pkgdir>` lists exported top-level identifiers and exported methods on exported types (non-test files).
   - `vocabulary`: 252 top-level + 6 methods = **258** (pin: 253 + 6; the difference is `EntityIRI`, already removed
     under #72).
   - `message`: 49 + 19 (pin 57 + 21).
   - `pkg/security`: 11 + 7 = **18** (identical at the pin).
2. `rd <root> <pkgdir> /<importpath>` records every reference:
   - a selector `<localname>.<Ident>` in any file that imports `github.com/c360studio/sem{streams,engine}/<pkg>`.
     It resolves import aliases, for example semsource's `semvocab` and `semvocabulary`.
   - bare-identifier uses inside the package's own non-test files ("intra").
   - test files flagged separately.
3. `classify.py` splits pin readers into admitted and not-admitted packages.
4. String-literal second spellings were checked separately for every constant classed dead, and for the registered
   graph terms. Each quoted value was searched in all non-test `.go` files of the pin, semsource and semconnect.
5. **Pin probes.** Every pin statement here is a static read of the pin snapshot, cited `path:line`. No runtime probe
   was run, because no statement here is about runtime behavior at the pin. The one exception is the NATS
   auth/TLS defect in §3, quoted from the change's design (`design.md:971-978`, issue #74) and not re-probed.
6. Coverage, measured at `931d90f`: `go test -count=1 -cover ./pkg/security/ ./internal/tlsutil/ ./vocabulary/
   ./message/`
   gives security 0.0%, tlsutil 90.8%, vocabulary 89.6%, message 91.9%.

Classes used below:

- **P**: pattern. How vocabulary is defined, registered, validated and resolved, including closed enums the
  resolver interprets.
- **B**: base term the engine itself uses.
- **C**: term only a consumer reads.
- **T**: test-support pattern, read by tests of admitted packages or of consumers.
- **Q**: owner question.
- **D**: dead. Nothing reads it in SemEngine production, in an admitted pin package, or in semsource or semconnect
  production code.

Test-only readers never keep an identifier alive, except T.

## Inventory category 3: open pull requests overlapping these paths

`gh pr list --state open --json number,title,isDraft,changedFiles,files`, filtered to `vocabulary/`, `message/`,
`pkg/security/`, `internal/tlsutil/`:

- **#48** (this change; 286 files, 33 in these paths within the 100-file listing).
- #73 (10 files, 0 hits).
- #82 (63 files, 0 hits).

No other claim overlaps. #73 (deployment authority, one spelling) is adjacent to vocabulary only through
`TestNoDeploymentAuthorityNames`, which already holds `EntityIRI` out.

---

## 1. `vocabulary`

### 1.1 Counts

| Class | Count | What |
| --- | --- | --- |
| P pattern | 57 | `Register`, `Option` and 12 `With*` options; `PredicateMetadata`; `AliasType` + 5 members + 2 methods; 7 `DataType*`; lookup (`GetPredicateMetadata`, `ListRegisteredPredicates`, `DiscoverAliasPredicates`, `DiscoverLabelPredicates`, `GetInversePredicate`, `IsRuleOpaque`, `IsValidPredicate`, `IsValidIndexingProfile`); grammar (`ParsePredicate`, `PredicateParts`, `PredicateValidationError`, `PredicateValidationReason` + 8 reasons, `MaxPredicateBytes`, `MaxPredicateSegmentBytes`, 3 methods); namespace (`PredicateNamespace`, `ParsePredicateNamespace`, `RequireDeclaredPredicate`); `SemStreamsBase` |
| B base term | 29 | 5 `LifecycleTransition*` (pkg/lifecycle); `GeoLocationLatitude/Longitude/Altitude`; `GraphRelContains`; 7 `Hierarchy*` (graph/inference); `EntityIndexingProfile` + 4 `IndexingProfile*`; `ContentClassificationTag` (graph-query); `DCTermsTitle` (graph-query) and its IRI `DcTitle`; IRIs used by kept registrations: `SkosBroader`, `SkosNarrower`, `SkosRelated`, `ProvHadMember`, `ProvNamespace` |
| C consumer-only | 4 | `SkosNotation`, `DcIdentifier` (semconnect `parser/sensorml/predicates.go:117`, `:129`); `ProvWasAttributedTo`, `ProvGeneratedAtTime` (semsource `source/vocabulary`) |
| T test support | 2 | `SnapshotRegistry` (tests of pkg/projection, pkg/fusion/fusionvocab, processor/rule, vocabulary/export at the pin; semconnect tests), `ClearRegistry` (vocabulary/export tests) |
| Q owner | 4 | `PredicateAuthority`, `NamespaceDelegation`, `NewPredicateAuthority`, `PredicateAuthority.Authorize`. The only reader is `agentic/tools.go:445-459`, a `defer-exclude` package. See 1.4 |
| D dead | 162 | below |
| **total** | **258** | |

Dead by family (measured): Sensor 17, Geo 10 (all but lat/lon/alt), Time 10, Network 10, Quality 9, GraphRel 11
(all but `contains`), Role 9 (`PredicateRole`, 7 `Role*`, `WithRole`), OWL 7, SKOS 3, RDFS 3, DC 11, Schema.org 7,
FOAF 3, PROV 43. Nine non-constant identifiers are also dead:

- `EntityTypeIRI`, `RelationshipIRI`, `SubjectIRI`, `GraphNamespace`, `SystemNamespace`.
- `RegisterPredicate`, `IsSymmetricPredicate`, `HasInverse`, `DiscoverInversePredicates`.

That is 153 dead constants, 8 dead functions and 1 dead type.

### 1.2 Findings behind the classes

- **Self-declared examples.** The domain constants call themselves examples.
  - `vocabulary/predicates.go:25-26`: "Domain applications ... define their own domain-specific predicates. These
    examples demonstrate the framework's predicate pattern."
  - `:435-439`: "The predicates in this file are EXAMPLES for demonstration purposes ... applications should define
    their own domain-specific vocabulary in their own packages."
  - The Sensor/Geo/Time/Network/Quality constants are bare strings. None is registered: zero intra readers.
- **Geo lat/lon/alt are base terms, spelled twice.** semconnect reads the constants.
  - semconnect: `gateway/cs-api/projection_contracts.go:83-85, :95, :98, :140-142`; `systems_post.go:266-271`;
    `systems_patch.go:189-191`.
  - Two admitted pin packages hard-code the same strings as literals:
    `processor/graph-index-spatial/component.go:884-892`
    and `processor/rule/expression/evaluator.go:911-915`.
  - So the spatial index (tier 0) depends on these three terms. Keep them. Later ports of those two packages should
    read the constant (adoption sweep, 1.6).
- **`GraphRelContains` is read only as a literal** in the default config at `graph/inference/config.go:209` (admitted).
- **The other 11 `GraphRel*` are dead.** `relationships.go:7-60` registers them in `init()`, so they become declared
  predicates.
  - Nothing reads the constants or their string values anywhere: a literal search of all 12 values found only
    `graph.rel.contains`.
  - Their IRIs `DcReferences`, `DcRequires`, `SchemaAbout`, `DcReplaces` and `DcRelation` are read only by those
    registrations, so they are dead with them.
- **Dropping a registration is observable.** It removes the term from `ListRegisteredPredicates`, which semsource
  reads (`graph`, `processor/source-manifest`).
  - It also makes `RequireDeclaredPredicate("graph.rel.near")` fail.
  - No consumer or admitted package uses those strings, so no reader changes behavior.
- **`Hierarchy*` (7) stay.** graph/inference reads the `*Member` terms and `TypeSibling`, both as constants and as
  literals (`graph/inference/config.go`).
  - The `*Contains` terms are their registered inverses, reached through `GetInversePredicate`.
  - `IsSymmetric` is consumed through `GetInversePredicate` (`registry.go:545-558`), which makes
    `IsSymmetricPredicate`, `HasInverse` and `DiscoverInversePredicates` redundant query spellings with no reader.
- **The Role family is dead.** The code says so itself at `registry.go:253-260`: no repository or sister reads
  `PredicateMetadata.Role`. The only pin reader is `processor/agentic-tools/executors/graph_query.go:584`, which is
  not admitted. The field `PredicateMetadata.Role` goes with it.
- **`RegisterPredicate` is a second registration spelling** ("backward compatibility and testing",
  `registry.go:382-386`).
  - It has no production caller anywhere.
  - At the pin it is used by `processor/rule/config_validation_test.go:122,127`, an admitted package's test that
    can switch to `Register`, and by `vocabulary/predicate_datatype_test.go`, which tests the second path's own
    validation.
  - `Register`'s doc comment (`registry.go:309-310`) points to it for replace semantics. That sentence changes if it
    goes.
- **IRI helpers.** `EntityTypeIRI`, `RelationshipIRI` and `SubjectIRI` are read only by `vocabulary/iris_test.go`,
  here and at the pin.
  - `vocabulary/export` (admitted, change 7) builds its own IRIs from `SemStreamsBase` (`export/export.go:55, :118`;
    `prefix.go:36, :95`) and has its own `subjectToIRI`: a second spelling of `SubjectIRI`.
  - This matches the #72 removal of `EntityIRI`: the export IRI is the one that stays. Dead.
- **Standard IRIs (OWL, SKOS, RDFS, DC, Schema.org, FOAF, PROV).** These are a partial copy of W3C catalogs.
  - Engine reads: only the IRIs attached to kept registrations.
  - The not-admitted `vocabulary/examples` reads `OwlSameAs`, `SkosPrefLabel`, `SkosAltLabel`, `SchemaIdentifier`
    and `FoafAccountName`; `vocabulary/agentic` reads `ProvWasDerivedFrom`.
- **Fields written but read by no behavior** (surface audit (b) shape, not identifiers):
  - `PredicateMetadata.Units` and `.Range` are set by `WithUnits`/`WithRange`, which semconnect calls at
    `gateway/cs-api/projection_contracts.go:151,154`.
  - A search for `.Units`/`.Range` in semconnect, semsource, vocabulary/export, pkg/, graph/ and processor/graph-*
    found no reader outside `registry.go`.
  - Keep the options, since a consumer calls them, but record that the metadata is descriptive only.
- **`WithRuleOpaque` has a reader but no admitted writer.** `IsRuleOpaque` is read by processor/rule (admitted).
  At the pin only `vocabulary/agentic` and `vocabulary/governance` write the flag; neither is admitted. Kept as the
  only writer of a flag the rule engine reads. Without a writer, rule behavior is always "not opaque".
- **README advertises constants that do not exist** (surface audit (c)): `vocabulary/README.md:363-371` lists
  `SsnHasDeployment`, `SosaObserves` and `SosaHasSimpleResult`. `grep -ci 'sosa\|ssn' vocabulary/standards.go` gives
  0. `README.md:384-456` also describes the `bfo/`, `cco/`, `agentic/` and `export/` sub-packages, none of which is
  in this tree.

### 1.3 Proposed keep set

Under the rulings in force, an admitted package is ported whole, and only dead surface is left behind. The owner
may still cut admitted surface by name.

- **Keep 92:** P 57 + B 29 + C 4 + T 2. Recommended.
- **Drop 162 (D).**
- **Q 4:** per the owner's answer below.

The pattern a downstream sees after the cut:

- Grammar: `ParsePredicate` and the reason codes.
- One registration call: `Register` with options.
- One metadata struct.
- Closed enums: data types and alias types.
- Lookups.
- The namespace check: `RequireDeclaredPredicate`.
- The indexing-profile set.
- About 20 framework-registered base terms: lifecycle, hierarchy, `graph.rel.contains` and `dc.terms.title`.
- Three geo terms the spatial index reads.

The base terms are also the worked examples of the pattern.

**What a consumer does for a cut term:** declare a local constant and register it through the pattern. semconnect
already does this for its own terms: `projection_contracts.go:144-155` registers the geo predicates with
`Register(..., WithDescription, WithUnits, WithRange)`. Geo lat/lon/alt are not cut, so semconnect changes nothing.
Under the tighter option (V2 below), each cut standard IRI becomes one local `const` holding the W3C IRI string. The
consumer learns it from a compile error at upgrade, the best rank on the adopter scale.

Options for the owner on the 4 C terms:

- **V1 (recommended): keep them.** They have consumer readers, so the rule keeps them. Cost: 4 W3C constants stay in
  the framework.
- **V2: cut them by name.** Cost: semconnect (2 lines) and semsource (2 lines) each add local constants. It also sets
  the precedent that consumer-read surface can be cut by owner name for tightness.

### 1.4 Owner question in vocabulary: `PredicateAuthority`

`namespace_authority.go:28-124` is a producer-to-namespace write authorization: `Authorize(producer, predicate)`
admits a predicate that is not registered only when that producer holds a delegation for its namespace. It is the
only authz-shaped primitive in the floor.

- Its only reader is `agentic/tools.go:445-459`, which is `defer-exclude`. By the rule it is dead.
- It trusts a producer string the caller asserts. Nothing authenticates that producer, so on its own it is
  authorization without authentication.
- **Recommendation: drop it from #48 under the dead-surface rule, and name it as prior art in the security design
  (§3).** Reason: an authz check keyed on an identity nobody authenticated gives a false sense of protection.
- **The other answer's cost:** keeping it carries 4 exported identifiers with no caller, plus a primitive whose
  trust model nobody has designed.

### 1.5 Ledger, test, docs and README effects

- **Ledger row `vocabulary`** (`docs/admission-ledger.yaml:467-504`). The disposition stays `adapt`. Add one adapt
  item, "dead surface removed (owner ruling `<comment>`, under #9 comment 5968830525)", listing pin line ranges per
  removed block:
  - `predicates.go`: Sensor/Geo/Time/Network/Quality blocks `:27-187` (keeping lat/lon/alt, `:76-81`); GraphRel
    `:197-239`
    (keeping `:193-195`); Role field `:545-550` and type plus constants `:561-583`.
  - `standards.go`: per-constant ranges.
  - `relationships.go`: 11 registrations.
  - `iris.go`: `:13-14`, the three functions `:17-118`, and the private `toKebabCase` `:119-149`, whose only caller is
    `RelationshipIRI` (`iris.go:79`).
  - `registry.go`: `WithRole` `:253-272`, `RegisterPredicate` from `:382`, `IsSymmetricPredicate`/`HasInverse`/
    `DiscoverInversePredicates` `:579-638`.
  - `proving_tests` loses the tests of the removed names: `relationships_test.go` (16 dead ids), `registry_test.go`
    (7), `iris_test.go` (5), `hierarchy_test.go` (4; its symmetric/inverse assertions should move to
    `GetInversePredicate`), `predicate_datatype_test.go` (`RegisterPredicate`, 1).
  - Exact line ranges are for the developer to measure with `task ledger:diff -- vocabulary`. Where only a start line is
    given, the developer measures the end.
- **Note for later rows** (not #48): `processor/rule/config_validation_test.go:122,127` switches to `Register` when
  rule is ported.
- **`vocabulary/README.md`.** Lines naming dead identifiers (measured with `grep -nwF -f dead-list`): 200, 232-266
  (alias examples using dead standard IRIs), 276, 290, 305-359 (standards mappings), 377-378, 466, 598-656 (Graph
  Domain Predicates section).
  - The SSN/SOSA block `:363-371` is advertised-absent.
  - The sub-package section `:384-456` describes code that is not here.
  - The README's import paths were already adapted under #9 comment 5968665464, so this is a further adapt item.
  - The README was ported with lint-only edits (owner ruling, #9 comment 5957221949). Editing its content needs the
    owner's word on the row, or a README rewrite by the technical writer.
- **`vocabulary/doc.go:225, :234`** show `EntityTypeIRI` in a migration example.
- **Doc comments** in `predicates.go` and `standards.go` go with their constants.
- No spec under `openspec/specs/` or the change's spec deltas names a dead vocabulary identifier. The search over
  `*.md`/`*.yaml` outside the archive found only `vocabulary/README.md`.

### 1.6 Adoption sweep (one home per term)

The three geo terms, `graph.rel.contains` and the `hierarchy.*` member terms are spelled as literals in admitted pin
packages that are not yet ported. Each later port should read the `vocabulary` constant:

- `processor/graph-index-spatial/component.go:884-892`
- `processor/rule/expression/evaluator.go:911-915`
- `graph/inference/config.go:209` (and its `hierarchy.*` defaults)

This is an enumeration for the tracking issue. #48 fixes none of them.

### 1.7 Adopter seam

- **Who:** a consumer author defining predicates.
- **What they must know after the cut:** two facts.
  - `Register` panics on a bad name at `init` (a root-process panic, so allowed).
  - A framework-registered name is amended, not replaced, by a second `Register` (`registry.go:292-310`, `:329`).
- **If they do nothing:** a cut constant is a compile error. No silent path exists, because the dead constants were
  never registered (Sensor and the rest) or have no reader (the 11 `GraphRel*`).
- **What they should have to know:** the same two facts. Cutting the examples removes the false impression that the
  framework owns sensor, network or quality semantics.

### 1.8 Per-identifier inventory (258 rows)

Reader columns are package directories (`p/` = `processor/`). "intra" lists the package's own non-test files.

| Identifier | Kind | Decl (`vocabulary/`) | Class | SemEngine prod | Pin admitted prod | Pin not admitted prod | semsource prod | semconnect prod | Test-only readers |
|---|---|---|---|---|---|---|---|---|---|
| `SemStreamsBase` | const | iris.go:12 | P | intra: iris.go | vocabulary/export | - | - | - | vocabulary |
| `GraphNamespace` | const | iris.go:13 | D | - | - | - | - | - | vocabulary |
| `SystemNamespace` | const | iris.go:14 | D | - | - | - | - | - | vocabulary |
| `EntityTypeIRI` | func | iris.go:31 | D | - | - | - | - | - | vocabulary |
| `RelationshipIRI` | func | iris.go:73 | D | - | - | - | - | - | vocabulary |
| `SubjectIRI` | func | iris.go:95 | D | - | - | - | - | - | vocabulary |
| `LifecycleTransitionFrom` | const | lifecycle.go:5 | B | intra: lifecycle.go | pkg/lifecycle | - | - | - | - |
| `LifecycleTransitionTo` | const | lifecycle.go:7 | B | intra: lifecycle.go | pkg/lifecycle | - | - | - | - |
| `LifecycleTransitionAt` | const | lifecycle.go:9 | B | intra: lifecycle.go | pkg/lifecycle | - | - | - | - |
| `LifecycleTransitionSource` | const | lifecycle.go:11 | B | intra: lifecycle.go | pkg/lifecycle | - | - | - | - |
| `LifecycleTransitionNote` | const | lifecycle.go:13 | B | intra: lifecycle.go | pkg/lifecycle | - | - | - | - |
| `PredicateNamespace` | type | namespace_authority.go:11 | P | intra: namespace_authority.go | - | - | - | - | vocabulary |
| `PredicateNamespace.String` | method | namespace_authority.go:17 | P | follows its type | | | | | |
| `NamespaceDelegation` | type | namespace_authority.go:28 | Q | intra: namespace_authority.go | - | agentic | - | - | vocabulary |
| `PredicateAuthority` | type | namespace_authority.go:37 | Q | intra: namespace_authority.go | - | agentic | - | - | - |
| `RequireDeclaredPredicate` | func | namespace_authority.go:47 | P | pkg/projection/contract/contract.go | pkg/lifecycle, pkg/projection/contract, p/rule | p/gated-dag | - | - | test/e2e/scenarios/lessons, vocabulary, vocabulary/builtins |
| `NewPredicateAuthority` | func | namespace_authority.go:60 | Q | - | - | agentic | - | - | vocabulary |
| `ParsePredicateNamespace` | func | namespace_authority.go:79 | P | intra: namespace_authority.go | p/graph-index | - | - | - | vocabulary |
| `PredicateAuthority.Authorize` | method | namespace_authority.go:101 | Q | follows its type | | | | | |
| `MaxPredicateSegmentBytes` | const | predicate_contract.go:12 | P | intra: predicate_contract.go | - | internal/predicateaudit | - | - | agentic, internal/predicateaudit, processor/graph-index, vocabulary |
| `MaxPredicateBytes` | const | predicate_contract.go:14 | P | intra: predicate_contract.go | - | internal/predicateaudit | - | - | processor/graph-index, vocabulary |
| `PredicateValidationReason` | type | predicate_contract.go:18 | P | intra: predicate_contract.go | graph, p/graph-ingest | internal/predicateaudit | - | - | internal/harness/semantictest, internal/semantictest, vocabulary |
| `PredicateReasonEmpty` | const | predicate_contract.go:22 | P | intra: predicate_contract.go | p/graph-ingest | - | - | - | vocabulary |
| `PredicateReasonLength` | const | predicate_contract.go:24 | P | intra: predicate_contract.go | p/graph-ingest | - | - | - | vocabulary |
| `PredicateReasonArity` | const | predicate_contract.go:26 | P | intra: predicate_contract.go | p/graph-ingest | - | - | - | graph, processor/graph-ingest, vocabulary |
| `PredicateReasonSegmentEmpty` | const | predicate_contract.go:28 | P | intra: predicate_contract.go | p/graph-ingest | - | - | - | vocabulary |
| `PredicateReasonSegmentLength` | const | predicate_contract.go:30 | P | intra: predicate_contract.go | p/graph-ingest | - | - | - | vocabulary |
| `PredicateReasonSegmentStart` | const | predicate_contract.go:32 | P | intra: predicate_contract.go | p/graph-ingest | - | - | - | internal/harness/semantictest, internal/semantictest, processor/graph-ingest, vocabulary |
| `PredicateReasonSegmentCharacter` | const | predicate_contract.go:34 | P | intra: predicate_contract.go | p/graph-ingest | - | - | - | graph, internal/harness/semantictest, internal/semantictest, vocabulary |
| `PredicateReasonSegmentHyphen` | const | predicate_contract.go:36 | P | intra: predicate_contract.go | p/graph-ingest | - | - | - | internal/harness/semantictest, internal/semantictest, vocabulary |
| `PredicateParts` | type | predicate_contract.go:40 | P | intra: predicate_contract.go | - | - | - | - | vocabulary |
| `PredicateParts.String` | method | predicate_contract.go:47 | P | follows its type | | | | | |
| `PredicateValidationError` | type | predicate_contract.go:53 | P | intra: predicate_contract.go | graph | internal/predicateaudit | - | - | graph, internal/harness/semantictest, internal/semantictest, vocabulary |
| `PredicateValidationError.Error` | method | predicate_contract.go:60 | P | follows its type | | | | | |
| `ParsePredicate` | func | predicate_contract.go:99 | P | internal/harness/semantictest/fixtures.go, intra: namespace_authority.go, predicates.go, registry.go | graph, p/graph-clustering, p/graph-index, p/rule | agentic, internal/predicateaudit, internal/semantictest, p/agentic-tools/executors | - | - | processor/graph-index, semconnect:vocabulary/csapi, semsource:source/vocabulary, vocabulary |
| `SensorTemperatureCelsius` | const | predicates.go:32 | D | - | - | - | - | - | vocabulary |
| `SensorTemperatureFahrenheit` | const | predicates.go:34 | D | - | - | - | - | - | - |
| `SensorTemperatureKelvin` | const | predicates.go:36 | D | - | - | - | - | - | - |
| `SensorPressurePascals` | const | predicates.go:39 | D | - | - | - | - | - | - |
| `SensorPressureBar` | const | predicates.go:41 | D | - | - | - | - | - | - |
| `SensorPressurePsi` | const | predicates.go:43 | D | - | - | - | - | - | - |
| `SensorHumidityPercent` | const | predicates.go:46 | D | - | - | - | - | - | - |
| `SensorHumidityAbsolute` | const | predicates.go:48 | D | - | - | - | - | - | - |
| `SensorAccelX` | const | predicates.go:51 | D | - | - | - | - | - | - |
| `SensorAccelY` | const | predicates.go:53 | D | - | - | - | - | - | - |
| `SensorAccelZ` | const | predicates.go:55 | D | - | - | - | - | - | - |
| `SensorGyroX` | const | predicates.go:58 | D | - | - | - | - | - | - |
| `SensorGyroY` | const | predicates.go:60 | D | - | - | - | - | - | - |
| `SensorGyroZ` | const | predicates.go:62 | D | - | - | - | - | - | - |
| `SensorMagX` | const | predicates.go:65 | D | - | - | - | - | - | - |
| `SensorMagY` | const | predicates.go:67 | D | - | - | - | - | - | - |
| `SensorMagZ` | const | predicates.go:69 | D | - | - | - | - | - | - |
| `GeoLocationLatitude` | const | predicates.go:77 | B | - | - | - | - | gateway/cs-api | semconnect:gateway/cs-api |
| `GeoLocationLongitude` | const | predicates.go:79 | B | - | - | - | - | gateway/cs-api | semconnect:gateway/cs-api |
| `GeoLocationAltitude` | const | predicates.go:81 | B | - | - | - | - | gateway/cs-api | semconnect:gateway/cs-api |
| `GeoLocationElevation` | const | predicates.go:83 | D | - | - | - | - | - | - |
| `GeoVelocityGround` | const | predicates.go:86 | D | - | - | - | - | - | - |
| `GeoVelocityVertical` | const | predicates.go:88 | D | - | - | - | - | - | - |
| `GeoVelocityHeading` | const | predicates.go:90 | D | - | - | - | - | - | - |
| `GeoAccuracyHorizontal` | const | predicates.go:93 | D | - | - | - | - | - | - |
| `GeoAccuracyVertical` | const | predicates.go:95 | D | - | - | - | - | - | - |
| `GeoAccuracyDilution` | const | predicates.go:97 | D | - | - | - | - | - | - |
| `GeoZoneUtm` | const | predicates.go:100 | D | - | - | - | - | - | - |
| `GeoZoneMgrs` | const | predicates.go:102 | D | - | - | - | - | - | - |
| `GeoZoneRegion` | const | predicates.go:104 | D | - | - | - | - | - | - |
| `TimeLifecycleCreated` | const | predicates.go:112 | D | - | - | - | - | - | - |
| `TimeLifecycleUpdated` | const | predicates.go:114 | D | - | - | - | - | - | - |
| `TimeLifecycleSeen` | const | predicates.go:116 | D | - | - | - | - | - | - |
| `TimeLifecycleExpired` | const | predicates.go:118 | D | - | - | - | - | - | - |
| `TimeDurationActive` | const | predicates.go:121 | D | - | - | - | - | - | - |
| `TimeDurationIdle` | const | predicates.go:123 | D | - | - | - | - | - | - |
| `TimeDurationTotal` | const | predicates.go:125 | D | - | - | - | - | - | - |
| `TimeScheduleStart` | const | predicates.go:128 | D | - | - | - | - | - | - |
| `TimeScheduleEnd` | const | predicates.go:130 | D | - | - | - | - | - | - |
| `TimeScheduleNext` | const | predicates.go:132 | D | - | - | - | - | - | - |
| `NetworkConnectionStatus` | const | predicates.go:140 | D | - | - | - | - | - | - |
| `NetworkConnectionStrength` | const | predicates.go:142 | D | - | - | - | - | - | - |
| `NetworkConnectionLatency` | const | predicates.go:144 | D | - | - | - | - | - | - |
| `NetworkProtocolType` | const | predicates.go:147 | D | - | - | - | - | - | - |
| `NetworkProtocolVersion` | const | predicates.go:149 | D | - | - | - | - | - | - |
| `NetworkProtocolPort` | const | predicates.go:151 | D | - | - | - | - | - | - |
| `NetworkTrafficBytesIn` | const | predicates.go:154 | D | - | - | - | - | - | - |
| `NetworkTrafficBytesOut` | const | predicates.go:156 | D | - | - | - | - | - | - |
| `NetworkTrafficPacketsIn` | const | predicates.go:158 | D | - | - | - | - | - | - |
| `NetworkTrafficPacketsOut` | const | predicates.go:160 | D | - | - | - | - | - | - |
| `QualityConfidenceScore` | const | predicates.go:168 | D | - | - | - | - | - | - |
| `QualityConfidenceSource` | const | predicates.go:170 | D | - | - | - | - | - | - |
| `QualityConfidenceMethod` | const | predicates.go:172 | D | - | - | - | - | - | - |
| `QualityValidationStatus` | const | predicates.go:175 | D | - | - | - | - | - | - |
| `QualityValidationErrors` | const | predicates.go:177 | D | - | - | - | - | - | - |
| `QualityValidationWarnings` | const | predicates.go:179 | D | - | - | - | - | - | - |
| `QualityAccuracyAbsolute` | const | predicates.go:182 | D | - | - | - | - | - | - |
| `QualityAccuracyRelative` | const | predicates.go:184 | D | - | - | - | - | - | - |
| `QualityAccuracyPrecision` | const | predicates.go:186 | D | - | - | - | - | - | - |
| `GraphRelContains` | const | predicates.go:195 | B | intra: relationships.go | - | - | - | - | vocabulary |
| `GraphRelReferences` | const | predicates.go:199 | D | intra: relationships.go | - | - | - | - | vocabulary |
| `GraphRelInfluences` | const | predicates.go:203 | D | intra: relationships.go | - | - | - | - | vocabulary |
| `GraphRelCommunicates` | const | predicates.go:207 | D | intra: relationships.go | - | - | - | - | vocabulary |
| `GraphRelNear` | const | predicates.go:211 | D | intra: relationships.go | - | - | - | - | vocabulary |
| `GraphRelTriggeredBy` | const | predicates.go:215 | D | intra: relationships.go | - | - | - | - | vocabulary |
| `GraphRelDependsOn` | const | predicates.go:219 | D | intra: relationships.go | - | - | - | - | vocabulary |
| `GraphRelImplements` | const | predicates.go:223 | D | intra: relationships.go | - | - | - | - | vocabulary |
| `GraphRelDiscusses` | const | predicates.go:227 | D | intra: relationships.go | - | - | - | - | vocabulary |
| `GraphRelSupersedes` | const | predicates.go:231 | D | intra: relationships.go | - | - | - | - | vocabulary |
| `GraphRelBlockedBy` | const | predicates.go:235 | D | intra: relationships.go | - | - | - | - | vocabulary |
| `GraphRelRelatedTo` | const | predicates.go:239 | D | intra: relationships.go | - | - | - | - | vocabulary |
| `HierarchyDomainMember` | const | predicates.go:253 | B | intra: hierarchy.go | graph/inference | - | - | - | graph/inference, vocabulary |
| `HierarchyDomainContains` | const | predicates.go:260 | B | intra: hierarchy.go | - | - | - | - | graph/inference, vocabulary |
| `HierarchySystemMember` | const | predicates.go:267 | B | intra: hierarchy.go | graph/inference | - | - | - | graph/inference, vocabulary |
| `HierarchySystemContains` | const | predicates.go:274 | B | intra: hierarchy.go | - | - | - | - | graph/inference, vocabulary |
| `HierarchyTypeSibling` | const | predicates.go:281 | B | intra: hierarchy.go | graph/inference | - | - | - | processor/graph-ingest, vocabulary |
| `HierarchyTypeMember` | const | predicates.go:288 | B | intra: hierarchy.go | graph/inference | - | - | - | graph/inference, vocabulary |
| `HierarchyTypeContains` | const | predicates.go:295 | B | intra: hierarchy.go | - | - | - | - | graph/inference, processor/graph-ingest, vocabulary |
| `EntityIndexingProfile` | const | predicates.go:307 | B | - | p/graph-clustering, p/graph-embedding, p/graph-ingest | test/e2e/scenarios/lessons | - | gateway/cs-api | processor/graph-embedding, processor/graph-ingest, semsource:internal/governance, semsource:processor/supersession, t... |
| `IndexingProfileContent` | const | predicates.go:312 | B | intra: predicates.go | - | agentic, agentic/research, test/e2e/scenarios/lessons | graph, handler/doc, handler/git, handler/url, source/ast | - | payloadbuiltins, payloadregistry, pkg/projection/contract, processor/graph-clustering, processor/graph-embedding, pro... |
| `IndexingProfileControl` | const | predicates.go:316 | B | intra: predicates.go | graph/inference, pkg/lifecycle, p/graph-ingest | agentic, agentic/research, cmd/e2e-semstreams/fixtures, test/e2e/scenarios/lessons | graph, handler/audio, handler/cfgfile, handler/git, handler/image, handler/video, source/ast | - | cmd/e2e-semstreams/fixtures, payloadbuiltins, payloadregistry, pkg/projection/contract, processor/graph-clustering, p... |
| `IndexingProfileSignal` | const | predicates.go:320 | B | intra: predicates.go | p/graph-embedding | agentic | graph | - | pkg/projection/contract, processor/graph-clustering, processor/graph-embedding, processor/graph-ingest, vocabulary |
| `IndexingProfileTrace` | const | predicates.go:323 | B | intra: predicates.go | p/graph-clustering, p/graph-embedding | agentic, agentic/research | graph, handler/video | - | payloadregistry, pkg/projection/contract, processor/graph-clustering, processor/graph-embedding, processor/graph-inge... |
| `IsValidIndexingProfile` | func | predicates.go:330 | P | payloadregistry/registry.go, pkg/projection/contract/contract.go | payloadregistry, pkg/projection/contract, p/graph-ingest | - | graph | - | payloadbuiltins, processor/graph-ingest, vocabulary |
| `DataTypeString` | const | predicates.go:352 | P | intra: hierarchy.go, labels.go, predicates.go | - | examples/processors/document, examples/processors/iot_sensor, examples/processors/weather_station, vocabulary/agentic, vocabulary/examples, vocabulary/governance | - | - | examples/processors/iot_sensor, test/contract, vocabulary, vocabulary/agentic, vocabulary/export |
| `DataTypeEntityID` | const | predicates.go:355 | P | intra: predicates.go | vocabulary/export | examples/processors/iot_sensor | - | parser/sensorml, vocabulary/csapi | semconnect:gateway/cs-api, semconnect:parser/sensorml, semconnect:vocabulary/csapi, test/contract, vocabulary, vocabu... |
| `DataTypeInt` | const | predicates.go:358 | P | intra: predicates.go | vocabulary/export | vocabulary/agentic, vocabulary/governance | - | - | test/contract, vocabulary/agentic, vocabulary/export |
| `DataTypeFloat` | const | predicates.go:360 | P | intra: predicates.go | vocabulary/export | examples/processors/document, examples/processors/iot_sensor, examples/processors/weather_station, vocabulary/agentic, vocabulary/governance | - | gateway/cs-api | test/contract, vocabulary/agentic |
| `DataTypeBool` | const | predicates.go:362 | P | intra: predicates.go | vocabulary/export | examples/processors/iot_sensor, vocabulary/agentic | - | vocabulary/csapi | test/contract, vocabulary/agentic |
| `DataTypeDateTime` | const | predicates.go:365 | P | intra: predicates.go | vocabulary/export | examples/processors/document, examples/processors/iot_sensor, examples/processors/weather_station, vocabulary/agentic | - | - | test/contract, vocabulary/agentic, vocabulary/export |
| `DataTypeJSON` | const | predicates.go:367 | P | intra: predicates.go | vocabulary/export | - | - | - | test/contract, vocabulary/export |
| `ContentClassificationTag` | const | predicates.go:432 | B | - | p/graph-query | - | - | - | - |
| `PredicateMetadata` | type | predicates.go:443 | P | intra: registry.go | - | - | - | - | processor/rule, vocabulary |
| `PredicateRole` | type | predicates.go:565 | D | intra: predicates.go, registry.go | - | - | - | - | - |
| `RoleUnspecified` | const | predicates.go:570 | D | - | - | - | - | - | vocabulary |
| `RoleIdentity` | const | predicates.go:572 | D | - | - | - | - | - | processor/agentic-tools/executors, vocabulary |
| `RoleLabel` | const | predicates.go:574 | D | - | - | - | - | - | - |
| `RoleRelationship` | const | predicates.go:576 | D | - | - | - | - | - | - |
| `RoleMetric` | const | predicates.go:578 | D | - | - | - | - | - | - |
| `RoleDescriptive` | const | predicates.go:580 | D | - | - | - | - | - | - |
| `RoleMetadata` | const | predicates.go:582 | D | - | - | - | - | - | - |
| `IsValidPredicate` | func | predicates.go:587 | P | pkg/projection/contract/contract.go | pkg/projection/contract, p/graph-ingest | - | - | - | test, vocabulary, vocabulary/agentic |
| `AliasType` | type | registry.go:12 | P | intra: predicates.go, registry.go | - | - | - | - | vocabulary |
| `AliasTypeIdentity` | const | registry.go:24 | P | intra: registry.go | - | vocabulary/examples | - | - | vocabulary |
| `AliasTypeLabel` | const | registry.go:39 | P | intra: labels.go, registry.go | - | vocabulary/examples | source/ast | - | vocabulary |
| `AliasTypeAlternate` | const | registry.go:52 | P | intra: registry.go | - | - | - | - | - |
| `AliasTypeExternal` | const | registry.go:64 | P | intra: registry.go | - | examples/processors/iot_sensor, vocabulary/examples | - | - | - |
| `AliasTypeCommunication` | const | registry.go:74 | P | intra: registry.go | - | vocabulary/examples | - | - | - |
| `AliasType.CanResolveToEntityID` | method | registry.go:78 | P | follows its type | | | | | |
| `AliasType.String` | method | registry.go:90 | P | follows its type | | | | | |
| `Option` | type | registry.go:101 | P | intra: registry.go | - | - | - | gateway/cs-api | - |
| `WithDescription` | func | registry.go:104 | P | intra: hierarchy.go, labels.go, lifecycle.go, relationships.go | - | examples/processors/document, examples/processors/iot_sensor, examples/processors/weather_station, vocabulary/agentic, vocabulary/examples, vocabulary/governance | source/ast, source/ontology, source/vocabulary | gateway/cs-api, vocabulary/csapi | processor/agentic-tools/executors, vocabulary, vocabulary/export |
| `WithDataType` | func | registry.go:131 | P | intra: hierarchy.go, labels.go | - | examples/processors/document, examples/processors/iot_sensor, examples/processors/weather_station, vocabulary/agentic, vocabulary/examples, vocabulary/governance | source/ast, source/vocabulary | gateway/cs-api, parser/sensorml, vocabulary/csapi | test/contract, vocabulary, vocabulary/export |
| `WithUnits` | func | registry.go:144 | P | - | - | examples/processors/iot_sensor, examples/processors/weather_station | - | gateway/cs-api | - |
| `WithRange` | func | registry.go:156 | P | - | - | examples/processors/iot_sensor, examples/processors/weather_station, vocabulary/agentic, vocabulary/governance | - | gateway/cs-api | - |
| `WithIRI` | func | registry.go:170 | P | intra: hierarchy.go, labels.go, relationships.go | - | examples/processors/document, examples/processors/iot_sensor, vocabulary/agentic, vocabulary/examples | source/ast, source/ontology, source/vocabulary | message/oms, parser/sensorml, vocabulary/csapi | vocabulary, vocabulary/export |
| `WithAlias` | func | registry.go:187 | P | intra: labels.go | - | examples/processors/iot_sensor, vocabulary/examples | source/ast | - | vocabulary |
| `WithInverseOf` | func | registry.go:212 | P | intra: hierarchy.go | - | vocabulary/agentic | - | parser/sensorml | processor/agentic-tools/executors, vocabulary |
| `WithRuleOpaque` | func | registry.go:229 | P | - | - | vocabulary/agentic, vocabulary/governance | - | - | vocabulary |
| `WithSymmetric` | func | registry.go:247 | P | intra: hierarchy.go | - | - | - | - | vocabulary |
| `WithRole` | func | registry.go:267 | D | - | - | - | - | - | processor/agentic-tools/executors, vocabulary |
| `WithWeight` | func | registry.go:286 | P | - | - | - | source/ast, source/vocabulary | - | pkg/fusion/fusionvocab, vocabulary |
| `Register` | func | registry.go:320 | P | intra: hierarchy.go, labels.go, lifecycle.go, relationships.go | - | agentic/agentrun, agentic/research, cmd/e2e-semstreams/mission, examples/processors/document, examples/processors/iot_sensor, examples/processors/weather_station, p/gated-dag, vocabulary/agentic, vocabulary/examples, vocabulary/governance, vocabulary/rulepacks | source/ast, source/ontology, source/vocabulary | gateway/cs-api, message/oms, parser/sensorml, vocabulary/csapi | pkg/fusion/fusionvocab, pkg/lifecycle, pkg/projection, pkg/projection/contract, processor/agentic-tools/executors, pr... |
| `RegisterPredicate` | func | registry.go:386 | D | - | - | - | - | - | processor/rule, vocabulary |
| `GetPredicateMetadata` | func | registry.go:466 | P | intra: namespace_authority.go | pkg/fusion/fusionvocab, vocabulary/export | p/agentic-tools/executors | p/source-manifest | gateway/cs-api | examples/processors/iot_sensor, semconnect:gateway/cs-api, semconnect:parser/sensorml, semconnect:vocabulary/csapi, s... |
| `ListRegisteredPredicates` | func | registry.go:481 | P | - | - | p/agentic-tools/executors | graph, p/source-manifest | - | test/contract, vocabulary, vocabulary/agentic |
| `DiscoverAliasPredicates` | func | registry.go:498 | P | - | p/graph-index | - | - | - | vocabulary |
| `DiscoverLabelPredicates` | func | registry.go:523 | P | - | p/graph-index | - | - | - | vocabulary |
| `GetInversePredicate` | func | registry.go:549 | P | - | graph/inference | - | - | - | semconnect:gateway/cs-api, semconnect:parser/sensorml, vocabulary |
| `IsRuleOpaque` | func | registry.go:568 | P | - | p/rule | p/agentic-tools/executors | - | - | vocabulary, vocabulary/governance |
| `IsSymmetricPredicate` | func | registry.go:584 | D | - | - | - | - | - | vocabulary |
| `HasInverse` | func | registry.go:597 | D | - | - | - | - | - | vocabulary |
| `DiscoverInversePredicates` | func | registry.go:624 | D | - | - | - | - | - | vocabulary |
| `ClearRegistry` | func | registry.go:641 | T | - | - | - | - | - | test/e2e/scenarios/lessons, vocabulary, vocabulary/agentic, vocabulary/builtins, vocabulary/export |
| `SnapshotRegistry` | func | registry.go:656 | T | - | - | - | - | - | pkg/fusion/fusionvocab, pkg/projection, pkg/projection/contract, processor/agentic-tools/executors, processor/rule, s... |
| `OwlSameAs` | const | standards.go:25 | D | - | - | vocabulary/examples | - | - | - |
| `OwlEquivalentClass` | const | standards.go:28 | D | - | - | - | - | - | - |
| `OwlEquivalentProperty` | const | standards.go:31 | D | - | - | - | - | - | - |
| `OwlInverseOf` | const | standards.go:36 | D | - | - | - | - | - | - |
| `OwlSymmetricProperty` | const | standards.go:41 | D | - | - | - | - | - | - |
| `OwlTransitiveProperty` | const | standards.go:45 | D | - | - | - | - | - | - |
| `OwlReflexiveProperty` | const | standards.go:49 | D | - | - | - | - | - | - |
| `SkosPrefLabel` | const | standards.go:57 | D | - | - | vocabulary/examples | - | - | - |
| `SkosAltLabel` | const | standards.go:62 | D | - | - | vocabulary/examples | - | - | - |
| `SkosHiddenLabel` | const | standards.go:66 | D | - | - | - | - | - | - |
| `SkosNotation` | const | standards.go:70 | C | - | - | - | - | parser/sensorml | - |
| `SkosBroader` | const | standards.go:75 | B | intra: hierarchy.go | - | - | - | - | vocabulary |
| `SkosNarrower` | const | standards.go:80 | B | intra: hierarchy.go | - | - | - | - | vocabulary |
| `SkosRelated` | const | standards.go:85 | B | intra: hierarchy.go | - | - | - | - | vocabulary |
| `RdfsLabel` | const | standards.go:92 | D | - | - | - | - | - | - |
| `RdfsComment` | const | standards.go:95 | D | - | - | - | - | - | - |
| `RdfsSeeAlso` | const | standards.go:98 | D | - | - | - | - | - | - |
| `DcIdentifier` | const | standards.go:106 | C | - | - | examples/processors/iot_sensor, vocabulary/examples | - | parser/sensorml | - |
| `DcTitle` | const | standards.go:110 | B | intra: labels.go | - | - | - | parser/sensorml | - |
| `DcAlternative` | const | standards.go:114 | D | - | - | - | - | - | - |
| `DcSource` | const | standards.go:117 | D | - | - | - | - | - | - |
| `DCTermsTitle` | const | standards.go:126 | B | intra: labels.go | p/graph-query | test/e2e/scenarios, test/e2e/scenarios/lessons | - | - | processor/graph-index, test/e2e/scenarios, test/e2e/scenarios/lessons, vocabulary |
| `DCTermsCreator` | const | standards.go:130 | D | - | - | - | - | - | - |
| `DCTermsIdentifier` | const | standards.go:134 | D | - | - | - | - | - | - |
| `SchemaName` | const | standards.go:141 | D | - | - | - | - | - | - |
| `SchemaAlternateName` | const | standards.go:145 | D | - | - | - | - | - | - |
| `SchemaIdentifier` | const | standards.go:149 | D | - | - | vocabulary/examples | - | - | - |
| `SchemaSameAs` | const | standards.go:153 | D | - | - | - | - | - | - |
| `ProvNamespace` | const | standards.go:162 | B | intra: standards.go | - | - | - | - | - |
| `ProvEntity` | const | standards.go:171 | D | - | - | - | - | - | - |
| `ProvActivity` | const | standards.go:177 | D | - | - | - | - | - | - |
| `ProvAgent` | const | standards.go:183 | D | - | - | - | - | - | - |
| `ProvPlan` | const | standards.go:192 | D | - | - | - | - | - | - |
| `ProvCollection` | const | standards.go:196 | D | - | - | - | - | - | - |
| `ProvBundle` | const | standards.go:201 | D | - | - | - | - | - | - |
| `ProvEmptyCollection` | const | standards.go:204 | D | - | - | - | - | - | - |
| `ProvLocation` | const | standards.go:208 | D | - | - | - | - | - | - |
| `ProvSoftwareAgent` | const | standards.go:212 | D | - | - | - | - | - | - |
| `ProvPerson` | const | standards.go:215 | D | - | - | - | - | - | - |
| `ProvOrganization` | const | standards.go:218 | D | - | - | - | - | - | - |
| `ProvWasAttributedTo` | const | standards.go:226 | C | - | - | - | source/vocabulary | - | semsource:source/vocabulary |
| `ProvWasDerivedFrom` | const | standards.go:231 | D | - | - | vocabulary/agentic | - | - | vocabulary/agentic |
| `ProvHadPrimarySource` | const | standards.go:236 | D | - | - | - | - | - | - |
| `ProvWasQuotedFrom` | const | standards.go:240 | D | - | - | - | - | - | - |
| `ProvWasRevisionOf` | const | standards.go:244 | D | - | - | - | - | - | - |
| `ProvWasGeneratedBy` | const | standards.go:253 | D | - | - | - | - | - | - |
| `ProvGenerated` | const | standards.go:257 | D | - | - | - | - | - | - |
| `ProvUsed` | const | standards.go:262 | D | - | - | - | - | - | - |
| `ProvWasInvalidatedBy` | const | standards.go:266 | D | - | - | - | - | - | - |
| `ProvInvalidated` | const | standards.go:270 | D | - | - | - | - | - | - |
| `ProvWasAssociatedWith` | const | standards.go:279 | D | - | - | - | - | - | - |
| `ProvActedOnBehalfOf` | const | standards.go:284 | D | - | - | - | - | - | - |
| `ProvHadMember` | const | standards.go:289 | B | intra: relationships.go | - | - | - | - | vocabulary |
| `ProvWasInfluencedBy` | const | standards.go:298 | D | - | - | - | - | - | - |
| `ProvInfluenced` | const | standards.go:302 | D | - | - | - | - | - | - |
| `ProvWasInformedBy` | const | standards.go:307 | D | - | - | - | - | - | - |
| `ProvWasStartedBy` | const | standards.go:311 | D | - | - | - | - | - | - |
| `ProvWasEndedBy` | const | standards.go:315 | D | - | - | - | - | - | - |
| `ProvHadActivity` | const | standards.go:319 | D | - | - | - | - | - | - |
| `ProvQualifiedGeneration` | const | standards.go:327 | D | - | - | - | - | - | - |
| `ProvQualifiedUsage` | const | standards.go:331 | D | - | - | - | - | - | - |
| `ProvQualifiedAssociation` | const | standards.go:335 | D | - | - | - | - | - | - |
| `ProvQualifiedDerivation` | const | standards.go:339 | D | - | - | - | - | - | - |
| `ProvQualifiedAttribution` | const | standards.go:343 | D | - | - | - | - | - | - |
| `ProvQualifiedDelegation` | const | standards.go:347 | D | - | - | - | - | - | - |
| `ProvQualifiedInfluence` | const | standards.go:350 | D | - | - | - | - | - | - |
| `ProvHadPlan` | const | standards.go:354 | D | - | - | - | - | - | - |
| `ProvHadRole` | const | standards.go:358 | D | - | - | - | - | - | - |
| `ProvStartedAtTime` | const | standards.go:366 | D | - | - | - | - | - | - |
| `ProvEndedAtTime` | const | standards.go:370 | D | - | - | - | - | - | - |
| `ProvGeneratedAtTime` | const | standards.go:374 | C | - | - | - | source/vocabulary | - | semsource:source/vocabulary |
| `ProvInvalidatedAtTime` | const | standards.go:378 | D | - | - | - | - | - | - |
| `ProvAtTime` | const | standards.go:382 | D | - | - | - | - | - | - |
| `ProvAtLocation` | const | standards.go:390 | D | - | - | - | - | - | - |
| `ProvValue` | const | standards.go:398 | D | - | - | - | - | - | - |
| `DcReferences` | const | standards.go:405 | D | intra: relationships.go | - | - | - | - | vocabulary |
| `DcIsReferencedBy` | const | standards.go:409 | D | - | - | - | - | - | - |
| `DcRequires` | const | standards.go:413 | D | intra: relationships.go | - | - | - | - | vocabulary |
| `DcIsRequiredBy` | const | standards.go:417 | D | - | - | - | - | - | - |
| `DcReplaces` | const | standards.go:421 | D | intra: relationships.go | - | - | - | - | vocabulary |
| `DcIsReplacedBy` | const | standards.go:425 | D | - | - | - | - | - | - |
| `DcRelation` | const | standards.go:429 | D | intra: relationships.go | - | - | - | - | vocabulary |
| `SchemaAbout` | const | standards.go:436 | D | intra: relationships.go | - | - | - | - | vocabulary |
| `SchemaIsPartOf` | const | standards.go:439 | D | - | - | - | - | - | - |
| `SchemaHasPart` | const | standards.go:443 | D | - | - | - | - | - | - |
| `FoafName` | const | standards.go:450 | D | - | - | - | - | - | - |
| `FoafNick` | const | standards.go:454 | D | - | - | - | - | - | - |
| `FoafAccountName` | const | standards.go:458 | D | - | - | vocabulary/examples | - | - | - |

---

## 2. `message` capability interfaces

The ten audited here:

- nine in `message/behaviors.go:22-137`: `Locatable`, `Timeable`, `Observable`, `Correlatable`, `Measurable`,
  `Deployable`, `Processable`, `Traceable`, `Expirable`.
- `TripleGenerator` at `message/triple.go:102-135`.

Pin line numbers are identical: `diff` of `behaviors.go:1-140` against the pin is empty, and the pin has
`TripleGenerator` at `triple.go:130`. The other `message` interfaces were checked as controls.

| Interface | SemEngine refs | Pin refs (any package) | semsource | semconnect | Type assertions or switches (named or structural) | Implementers by method shape (pin/semsource/semconnect prod files) | Class |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `Locatable` | 0 | 0 | 0 | 0 | 0 | `Location() (` 0/0/0 | D |
| `Timeable` | 0 | 0 | 0 | 0 | 0 | `Timestamp() time.Time` 0/0/0 | D |
| `Observable` | 0 | 0 | 0 | 0 | 0 | `ObservedEntity()` 0/0/0 | D |
| `Correlatable` | 0 | 0 | 0 | 0 | 0 | `CorrelationID() string` 0/0/0 | D |
| `Measurable` | 0 | 0 | 0 | 0 | 0 | `Measurements() map` 0/0/0 | D |
| `Deployable` | 0 | 0 | 0 | 0 | 0 | `DeploymentID() string` 0/0/0 | D |
| `Processable` | 0 | 0 | 0 | 0 | 0 | `Priority() int`, `Deadline() time` 0/0/0 | D |
| `Traceable` | 0 | 0 | 0 | 0 | 0 | `TraceID() string` 0/0/0 | D |
| `Expirable` | 0 | 0 | 0 | 0 | 0 | `ExpiresAt() time`, `TTL() time.Duration` 0/0/0 | D |
| `TripleGenerator` | 0 | 0 | 0 | 0 | 0 | `Triples() []message.Triple` 16/8/3, but these satisfy `graph.Graphable` (pin `graph/graphable.go:54-59`), which is what the pin asserts: `processor/graph-ingest/component.go:1799`, `storage/objectstore/component.go:734, :776` | D (a second spelling of `Graphable.Triples`) |
| control: `IndexingProfiler` (`behaviors.go:154`) | 0 | admitted `processor/graph-ingest` | test only (`processor/source-manifest`) | 0 | yes (graph-ingest) | n/a | keep |
| control: `RuleReadable`, `Storable`, `ContentStorable`, `BinaryStorable` | intra | admitted `processor/rule`, `processor/graph-ingest`, `storage/objectstore` | 0 | 0 | yes | n/a | keep |

How each column was measured:

- **Refs:** the `rd` tool, which counts every `message.<Name>` selector in files importing the package, plus intra
  uses.
- **Structural assertions:** `grep -rnE 'interface\s*\{\s*(Location|Timestamp|ObservedEntity|...|Triples)\('` over
  all four trees found 0.
- **Implementers:** `grep -rlE '^func \([^)]*\) <Method>'` over non-test files.

**Disposition: all ten are dead.** Nothing type-asserts any of them anywhere. No producer implements the nine
behaviors.

**The docs advertise behavior that does not exist** (surface audit (c)):

- `behaviors.go:5-16` says "Services discover these capabilities at runtime through type assertions".
- `:22-24` says a `Locatable` payload "can be indexed by location". The spatial index instead reads
  `geo.location.*` triples (pin `processor/graph-index-spatial/component.go:884-892`). That makes two spellings of
  the location fact; the triple is the one that is read.
- `message/README.md:14` ("Behavior-based processing: type-assert to capabilities").

**Effects:**

- **Ledger row `message`** (`docs/admission-ledger.yaml:872`): add an adapt item, "dead surface removed", with pin
  lines `behaviors.go:22-137` (keeping `:138-157` `IndexingProfiler`), the header example `:5-16` reworded, and
  `triple.go:102-135`.
- **`message/README.md`:** lines 14, 44-45, 66-69, 128, 134, 140, 298, 305-308, 329, 387, 545, 570, 652-655.
- **`message/doc.go`:** lines 38, 45, 50, 58, 66, 74, 79, 88, 94, 120, 133, 429, 446-447, 489.
- `rule_readable.go`'s note ("RuleReadable is a member of this family", `behaviors.go:18-19`) needs rewording.
- No spec or design names them: a search of `docs/contract.md`, `docs/repository-map.md`, the change's `design.md`
  and its `message-codec` delta gave 0.

**Adopter seam:** no consumer references or implements any of the ten (0/0 above). Removal costs a compile error
only for code that names them, and none does.

---

## 3. `pkg/security`

### 3.1 What it contains and what it provides

`pkg/security/config.go` (244 lines) and `doc.go` (140 lines), stdlib only:

- 7 configuration types: `Config`, `TLSConfig`, `ServerTLSConfig`, `ClientTLSConfig`, `ServerMTLSConfig`,
  `ClientMTLSConfig`, `ACMEConfig`.
- 4 `Default*` constructors.
- 7 `Validate` methods.

| Capability | Provided? | Where |
| --- | --- | --- |
| TLS for servers and clients (cert/key files, minimum version 1.2/1.3, extra CA files) | **config types only**; the behavior is in `internal/tlsutil` | `internal/tlsutil/tlsutil.go:15-70` |
| mTLS (client CA pool, require-or-optional client certificate, client-cert provision) | config types; behavior in tlsutil | `tlsutil.go:75-160` |
| Authorization | only a client-certificate Common Name allowlist (`AllowedClientCNs`), checked against `Subject.CommonName` with SANs ignored | `tlsutil.go:120-143` |
| ACME (automatic certificates) | types only. The ACME loaders were removed from tlsutil (owner ruling, #9 comment 5950752741; ledger `docs/admission-ledger.yaml:666-678`), and `pkg/acme` is not ported | — |
| Caller authentication (tokens, API keys, JWT, NATS nkeys/creds) | **no** | — |
| Authorization policy (who may read or write which subject, bucket, predicate or entity) | **no** | — |
| Secrets handling (loading, redaction, rotation) | **no**: key material is referenced by file path; NATS password is a plain string in `natsclient` | — |
| Message integrity or signing, audit | **no** | — |

### 3.2 Readers

| Identifier | SemEngine prod | Pin admitted prod | Pin not admitted | semsource | semconnect | Class |
| --- | --- | --- | --- | --- | --- | --- |
| `Config` | `metric/handler.go:35, :42` (`NewServer` signature) | `component/dependencies.go:74`, `config/config.go:50`, `metric/handler.go:33,40`, `output/websocket/websocket.go:68,82,135`, `service/metrics.go:28,92`, `service/component_manager.go:70,162` | `input/websocket`, `output/httppost` | 0 | 0 | keep |
| `TLSConfig` | field type only | field type only | — | 0 | 0 | keep (structural) |
| `ServerTLSConfig` | `internal/tlsutil/tlsutil.go:15,75` | `pkg/tlsutil` | — | 0 | 0 | keep |
| `ServerMTLSConfig` | `tlsutil.go:75,95` | `pkg/tlsutil` | — | 0 | 0 | keep |
| `ClientTLSConfig` | `tlsutil.go:35` | `pkg/tlsutil` | `output/httppost` | 0 | 0 | keep. The client loaders' only production callers at the pin are `input/websocket` and `output/httppost`, both not admitted; in SemEngine nothing calls `LoadClientTLSConfig*` (tlsutil's ledger row carries them "as at the pin") |
| `ClientMTLSConfig` | `tlsutil.go:147` | `pkg/tlsutil` | — | 0 | 0 | keep (as above) |
| `ACMEConfig` | none | `pkg/tlsutil` ACME loaders; `output/websocket/websocket.go:760-764` reads `.ACME.Enabled` | `input/websocket:1529`, `output/httppost:722` | 0 | 0 | keep by rule (admitted `output/websocket` reads it), but **advertised-absent until change 5** |
| `DefaultConfig`, `DefaultTLSConfig`, `DefaultServerTLSConfig`, `DefaultClientTLSConfig` | none | none | none | 0 | 0 | **D** |
| `Config.Validate`, `TLSConfig.Validate`, `ServerTLSConfig.Validate`, `ClientTLSConfig.Validate`, `ServerMTLSConfig.Validate`, `ClientMTLSConfig.Validate`, `ACMEConfig.Validate` | none | none | none | 0 | 0 | **D**: no call anywhere outside the package |

How readers were measured:

- The `rd` tool, plus `grep -rn
  'security\.Default\|Security\.Validate\|\.TLS\.Validate\|ACME\.Validate\|MTLS\.Validate\|Server\.Validate\|Client\.Validate'`
  over the pin and the worktree, which found 0.
- semsource and semconnect import `pkg/security` 0 times (import census).

**Counts: 18 identifiers; 7 kept by the rule, 11 dead (4 `Default*`, 7 `Validate`).**

### 3.3 Quality

1. **No tests, here or at the pin.** The pin's `pkg/security/` holds only `config.go` and `doc.go`. Coverage at
   `931d90f` is 0.0%. Its behavior is tested only indirectly, through `internal/tlsutil` (90.8%, including
   `mtls_integration_test.go`).
2. **Validation exists twice, and the copy that runs differs from the one that is shipped.**
   - The pin's `config/config.go:303-357` (`validateSecurity`) is what boot runs.
   - It requires `cert_file` and `key_file` whenever server TLS is enabled, even in ACME mode.
   - It ignores `mode`, mTLS and client mTLS.
   - It prints an `insecure_skip_verify` warning to stderr.
   - The seven `Validate` methods here check a different set of rules, and nothing calls them. So `doc.go:114-120`
     ("All config types provide Validate() methods that check ...") is advertised behavior that no path executes.
3. **The ledger row's "no I/O" is wrong.** The `Validate` methods call `os.Stat` (`config.go:104, :158, :161, :189,
   :192, :231`), against ledger `docs/admission-ledger.yaml:420` ("stdlib only; no goroutine, no I/O"). The I/O is
   unreachable today, because nothing calls `Validate`.
4. **Documented default versus code default for `RequireClientCert` (fail-open against the doc).**
   - The schema tag says `default:true` (`config.go:91`).
   - The Go zero value is `false`, and no `Default*` sets it.
   - `tlsutil.go:113-117` maps `false` to `tls.VerifyClientCertIfGiven`, which makes client certificates optional.
   - So an operator who enables mTLS and relies on the documented default gets optional client certificates.
   - Whether the future `config` loader applies schema-tag defaults is not probed; `config` is not ported. tlsutil's
     tests pass the field explicitly (`tlsutil_test.go:423, :448`).
5. **Unknown TLS versions degrade silently.** `parseTLSVersion` (`tlsutil.go:172`) maps any unrecognized value to
   1.2, so a typo such as `"1.3 "` downgrades without a signal. The `Validate` that would refuse it is never called.
   This is undeclared under the fail-closed rule.
6. **ACME mode has no implementation here.** `Mode: "acme"` passes the types. The metric server ignores `Mode`
   and loads files, so it fails loudly at `Start` (`LoadX509KeyPair("", "")`), which is fail-closed. It is still
   advertised in `doc.go:71-90` and in the schema tags.
7. **Crypto use is sound but minimal.** It uses stdlib `crypto/tls` and `crypto/x509` with a TLS 1.2 floor and
   default cipher suites; there is no hand-rolled crypto.
   - A `SystemCertPool` error falls back to an empty pool (`tlsutil.go:41`). That fails closed: verification fails.
   - The CN allowlist relies on the legacy CN field rather than SANs.
   - Whether `VerifyPeerCertificate` runs with zero chains under `VerifyClientCertIfGiven` was not probed.
8. **Defaults.** "TLS disabled by default" (`config.go:16`) is the posture for every surface. Nothing warns when a
   non-loopback listener serves plaintext.

<!-- markdownlint-disable-next-line MD013 -->
### 3.4 Security-relevant facts outside `pkg/security` (same question, nearest instances; inventory category 5)

- **NATS connection (the engine's main trust boundary).** `natsclient` keeps `WithCredentials(user, password)` as
  its only auth option (`natsclient/options.go:77`; `client.go:645-647`).
  - The 3.7a audit dropped the pin's `WithToken` and `WithTLS` (pin `natsclient/options.go:157-172`;
    `client.go:423-430`) because nothing called them (`design.md:971-973`).
  - The pin defect behind that, quoted from `design.md:973-978` and issue #74 (open, `bug`, not re-probed here):
    `config` parses NATS username, password, token and TLS, but boot builds the client from URLs only. An operator
    who configures them gets a plaintext, unauthenticated connection, and nothing says so.
  - There is no nkey or creds-file (JWT) option at the pin either. That is the NATS-native decentralized auth that
    works offline.
- **HTTP surfaces.** The pin's `output/websocket/doc.go:225, :234` says "No authentication/authorization (add
  reverse proxy)". The metric server has TLS and mTLS (`metric/handler.go:116-118, :162-168`; the worktree already
  fixed the pin's ignored-mTLS defect, pin `metric/handler.go:154`).
- **Write admission and provenance.**
  - `Triple.Source` and `meta.source` are asserted by the caller and never authenticated.
  - `vocabulary.PredicateAuthority` (1.4) authorizes by a producer string the caller supplies.
  - Nothing binds a write to an authenticated connection identity.

### 3.5 Intent check

`AGENTS.md` "What this is for" names ingest, index, query, vocabulary, provenance, fusion, the tier ladder,
durable workflows, replay, settlement, retries and rules. It does not name authentication, authorization or
security. I found no ruling admitting them:

- `gh search issues --repo C360Studio/semengine 'security OR authentication OR authorization'` returns only process
  issues.
- `docs/setup-plan.md` mentions security only for CI scanning (`:242-254`).

| Capability | Status | Ruling |
| --- | --- | --- |
| vocabulary | admitted | ledger row `vocabulary`; #8 |
| provenance | admitted (named in purpose) | no ruling on whether PROV-O IRI constants are part of it. All 43 PROV constants cut in 1.3 have no reader, and engine provenance is carried by `Triple.Source`/`meta.source`, not PROV-O IRIs |
| TLS for engine-served HTTP | admitted as a dependency of `metric` | ledger rows `pkg/security`, `pkg/tlsutil` |
| authn/authz/security primitives | **no ruling**: not named in the purpose | **owner question** (below) |

<!-- markdownlint-disable-next-line MD013 -->
### 3.6 What basic primitives SemEngine should own (analysis for the owner, not settled)

The yardstick is the purpose: engine-owned means it belongs to one of the two halves and runs at tier 0 with no
external provider, offline-first and at the edge. Measured against that, an engine on NATS has four trust boundaries:

| # | Boundary | What the engine should own at tier 0 | What stays with consumers | Where the floor is today |
| --- | --- | --- | --- | --- |
| S1 | process ↔ NATS (the main one) | Client authentication with NATS-native means that work offline: TLS/mTLS, nkeys, creds/JWT files, user/password. The config/boot path refuses at boot any option it cannot honor. A subject and bucket layout that NATS server permissions can express per producer and tenant. The NATS server enforces; the engine wires it and does not reinvent it | operator/account JWT issuance; the NATS server config | **regressed and broken**: only user/password in `natsclient`; the pin silently drops TLS/token (#74) |
| S2 | engine-served HTTP (metrics, websocket out, query) | TLS/mTLS (present). One small seam: an `Authenticator` that turns a request into a principal, and an `Authorizer(principal, action, resource)`. Tier-0 implementations: an mTLS principal from the certificate, and a static token file | OAuth/OIDC, IdPs, user management, RBAC UIs | TLS/mTLS config in `pkg/security`; no authn/authz seam; websocket out says "use a reverse proxy" |
| S3 | write admission and provenance (who may assert which triples) | Bind the producer identity to the authenticated connection (S1/S2 principal) instead of a caller-asserted `Source`. A namespace delegation check keyed on that principal (the shape of `PredicateAuthority`, with a trusted identity) | domain policy on which entity types a producer owns | nothing trusted: `PredicateAuthority` is caller-asserted and its only user is excluded |
| S4 | secrets | Load credentials from file or env; redact them from logs, status and config dumps; refuse world-readable key files | secret managers (Vault and the like) | none |

**Distance from `pkg/security`:** it covers part of S2 (TLS/mTLS configuration types) and nothing of S1, S3 or S4.
The biggest gap, S1, is not in `pkg/security` at all; it sits in `natsclient` and the not-yet-ported `config`/boot
path (#74).

### 3.7 Options for #48 (the owner decides)

**(a) Port as is, with tests added in #48.**

- Write tests for the 7 `Validate` methods and fix the `RequireClientCert` doc mismatch.
- Cost:
  - tests for validators no path calls, unless #48 also wires them, which is new behavior in `metric`;
  - keeps 11 dead identifiers against the standing rule;
  - keeps a package named `security` that implies coverage it lacks;
  - adds review rounds to a PR at hold 7.1.

**(b) Port a reduced core in #48, and open a follow-up epic for security primitives. Recommended.**

- #48:
  - drop the 11 dead identifiers (4 `Default*`, 7 `Validate`) under the existing dead-surface rule;
  - keep the 7 types, which `metric` and `tlsutil` read;
  - correct the `RequireClientCert` schema tag to the real default, or make the default `true`. Either needs a test
    that fails when the field is ignored (tlsutil has one at `tlsutil_test.go:410`);
  - correct `doc.go`'s Validation and ACME claims, and the ledger's "no I/O" line;
  - `parseTLSVersion`'s silent downgrade is fixed or declared (it is in `internal/tlsutil`, already ported).
- Epic: security primitives S1-S4 as their own OpenSpec change, admitted by owner mandate at a named tier with a
  named qualifying workload (the AGENTS.md admission rule). S1 is first and absorbs #74.
- Cost:
  - Config validation then has one home: the `config` port. That port must carry it, which #74 already requires.
  - A new epic, and the owner's time to name its tier and workload.

**(c) Leave `pkg/security` out of #48, and design the primitives as their own change.**

- Cost:
  - `metric.NewServer` takes `security.Config` (public by the signature rule, ledger `:416-417`), so `metric` and
    `internal/tlsutil` must change their TLS parameter in #48, late in review;
  - when `config` is ported, a second TLS config shape appears unless the new design lands first;
  - the metric server's TLS/mTLS, which works and is tested, is reworked for no present user gain.

**Recommendation: (b).**

1. It removes only what the standing rule already removes, so no new principle is needed.
2. It keeps the working, tested TLS/mTLS path.
3. It fixes the one fail-open item (the `RequireClientCert` default) and the false docs.
4. It moves the owner's real concern to a design sized for it. That design starts where the exposure actually is,
   the NATS connection (S1, #74), rather than in `pkg/security`.

**Costs the owner pays under (b):**

- one ruling now (accept the 11 removals plus the doc and default fixes);
- one admission ruling later, naming the tier and qualifying workload for the security epic;
- reading that epic's design.

---

## Inventory review

- **Reviewer:** claude (opus, `semengine-reviewer`), working under the reviewer contract § Inventory review.
- **What I reviewed:** the architect's surface-audit inventory, at the stated base: worktree `931d90f`, SemStreams pin
  `8b99efe9`, semsource `e4febc0d` and semconnect PR #74 head `dff12657`.
- **Method, independent of the architect's tools:**
  - SemEngine code came from my own `git archive 931d90f` export.
  - The pin came from my own earlier tarball.
  - semsource and semconnect came from tarballs I fetched myself with `gh api …/tarball/<sha>`. I confirmed that PR
    #74's head is `dff1265708910ef411ab6f85b16fd9e94663e997`, and that the PR is open.
  - I did not reuse the architect's `exp`, `rd` or `src/` files.
  - I ran no git command against a sister checkout.
- **Tools I wrote, all in my scratchpad:**
  - A `go/types` enumerator of exported objects.
  - A `go/types` dump of every string constant's value.
  - A program that prints the vocabulary registry at init.
  - An AST scanner over every interface type and method declaration.
  - A throwaway TLS handshake probe against the real `internal/tlsutil`. I deleted it after the run.
- **Searches:** word-bounded name searches and string-value searches across all four trees, in every file type, not only
  `.go` files.

### Counts: confirmed exactly

`go/types` gives the same totals as the inventory:

| Package | Exported identifiers | Breakdown |
|---|---|---|
| `vocabulary` | 258 | 209 constants, 33 functions, 10 types, 6 methods |
| `message` | 68 | 49 + 19 |
| `pkg/security` | 18 | 7 types, 4 functions, 7 methods |

The §1.8 table also matches: P 57, B 29, C 4, T 2, Q 4, D 162.

### The "dead" claims hold, checked more widely than the inventory did

**The 162 vocabulary names.** I searched every `.go` file in all four trees by name, whatever the import, outside
`vocabulary/`'s own files. That finds aliased and dot-imported uses too; there are no dot imports. The hits fall into
three groups:

- Tests: `processor/rule/config_validation_test.go` (`RegisterPredicate`) and `agentic-tools` tests (`Role*`,
  `WithRole`).
- Packages that are not admitted: `vocabulary/examples` and `vocabulary/agentic`.
- semsource's own constants, described below.

No admitted or consumer production code reads any of the 162.

**Their string values, in all file types.** The inventory searched values in `.go` files only. I searched all 69
predicate-name values in every file type, including configs, JSON, YAML and Markdown. The hits are tests, the
not-admitted `examples/`, docs, OpenSpec archives and old e2e result JSON. No config or rule file in any tree uses any
of them.

**Readers that look terms up by string or iterate the registry.** The registry at init holds 25 predicates, 11 of them
the dead `graph.rel.*` terms. The consumers that look up or iterate the registry are:

- semsource `processor/source-manifest/status.go:241-268` filters by `code.`, `dc.`, `agentic.` and `source.*`.
- semsource `graph/contract.go:38` filters by `source.` and `code.`.
- semconnect reads only its own names (`projection_contracts.go:132, :145`).

None of these filters matches `graph.rel.`. So removing the 11 registrations changes no consumer output. This adds
evidence to §1.2's "no reader changes behavior".

**The `message` interfaces.** I scanned every interface type, named or unnamed, in all four trees for the methods of the
ten. Only these carry a matching method:

| Interface | Method | Relation to the ten |
|---|---|---|
| `graph.Graphable` | `Triples()` | another spelling of `TripleGenerator.Triples` |
| `message.Storable` (`message/storable.go:71`) | `Triples()` | another spelling of `TripleGenerator.Triples` |
| `pkg/fusion` lens | `Location` | a different signature; not equivalent |

No production type in any tree implements the nine behaviors. The only implementer is a test,
`message/payload_test.go:78-90`, which calls the methods directly rather than through the interfaces. All ten are dead.

**The 11 `pkg/security` identifiers.** No caller exists, including through interface dispatch:

- The pin's generic `component.Validatable` dispatch (`component/validation.go:192`, inside `SafeUnmarshal`) receives
  component configs only.
- No struct in any tree embeds a `security` type, so no type inherits `Validate` by method promotion.
- The one other `cfg.Validate()` match, `pkg/acme/client.go:103`, belongs to `acme.Config`, its own type.
- `config.validateSecurity` checks fields directly (`config/config.go:303-357`).
- semsource and semconnect never import the package.

**`PredicateAuthority`.**

- Its only code readers are `agentic/tools.go:443-459`.
- `agentic`'s exported wrapper `AuthorizeLineageTriplePredicate` (`:458`) is called only inside `agentic` (`:479`), so
  no admitted package reaches it.
- The comment at `processor/agentic-tools/executors/graph_query.go:455` mentions it but does not call it.
- `agentic` is `defer-exclude`, which settles the dead-surface answer.

**Scope.** Scope matches `docs/inventory-scope.md`:

- semconnect is admitted for "vocabulary/export seams".
- semsource is admitted for its dependency closure, and rule 2 measures that at symbol level.
- semteams and semboids are not admitted for this question.

One nuance: semteams is admitted for "the agentic seams", which is where `PredicateAuthority`'s only caller lives. That
cannot change the answer, because `agentic` itself is excluded. The owner may still want to know whether semteams calls
`agentic.AuthorizeLineageTriplePredicate`, since its answer is a ruling on #22.

**Open pull requests.** I ran the listing myself. It matches the inventory: #48 has 33 hits within its first 100 files,
and #73 and #82 have none.

### Findings: corrections for the architect to fold in before the owner rules

None changes a keep set or a count.

**MEDIUM-I1 §3.3 items 4 and 7: the `RequireClientCert` fail-open is confirmed but narrower than stated, and the "not
probed" question is now answered.**

- Statically: nothing at the pin applies `schema:"…default:true"` tags. They are read only by
  `component.GenerateConfigSchema` (`component/schema_tags.go:367`), which renders a schema. The loader's `getDefaults`
  (`config/config.go:504-540`) sets no security field. So an omitted `require_client_cert` is `false` at the pin, and in
  SemEngine.
- At runtime, my probe against `internal/tlsutil` gave the same result on TLS 1.2 and TLS 1.3. It used
  `LoadServerTLSConfigWithMTLS` with mTLS enabled and `RequireClientCert` left at zero. A client with no certificate
  then completes the handshake when `AllowedClientCNs` is empty. When an allowlist is set, `VerifyPeerCertificate` runs
  with zero chains and refuses ("no verified certificate chains", `tlsutil.go:131-133`).
- So the fail-open is: mTLS enabled, no CN allowlist, client certificate omitted. An allowlist closes it.
- §3.3 item 7's open question should be replaced with this result, and option (b)'s fix should say which case it closes.

**MEDIUM-I2 §3.3 item 5: the silent TLS-version downgrade is a regression introduced by the port, not a defect at the
pin.**

- At the pin, boot refused a bad version: `config/config.go:324-326, :348-350` call `validateTLSVersion` (`:358-364`),
  which accepts only "1.2" and "1.3".
- SemEngine has no `config` loader yet. `metric.NewServer` takes a `security.Config` from its caller, so
  `parseTLSVersion`'s fallback to 1.2 (`tlsutil.go:172-180`) is now reachable without any check.
- The inventory implies the defect is longstanding. It should say the port opened it, and that the `config` port, or a
  refusal in `parseTLSVersion`, closes it.

**NIT-I3 §2: `TripleGenerator` is a third spelling of `Triples()`, not a second.** `message.Storable` also declares it
(`message/storable.go:71`).

**NIT-I4 §2 "Effects": two documentation sites are missing.**

- `message/message.go:30` and `message/payload.go:10` also name the behaviors.
- So do the test comments in `message/payload_test.go:78, :86, :245, :251`. Their methods stay valid once the interfaces
  go.

**NIT-I5 §1.2 and §1.6: semsource has its own role taxonomy with the same values.**

- `processor/source-manifest/status.go:48-53` declares `RoleIdentity`, `RoleContent`, `RoleLocation`,
  `RoleRelationship`, `RoleMetric` and `RoleMetadata` ("identity", …). semsource derives roles itself
  (`predicateRole(pred)`, `:281`) and never reads `PredicateMetadata.Role`.
- This supports dropping the Role family. It also means that if the engine ever needs roles again, semsource's spelling
  is the home to adopt. That belongs as a one-line note in the adoption sweep.

**Note for later ports, not for this audit:** the pin's `test/contract/predicate_datatype_contract_test.go:219-230` pins
the registered `graph.rel.*` set. If that contract test is ported, it must drop those rows.

### Verdict

**INVENTORY PASS.** All of these hold, by my own method:

- the counts;
- the keep and drop sets;
- every "dead" claim, including by string value in every file type, through registry lookup, through interface dispatch
  and promotion, and by reflection or dot import, of which there are none;
- the scope, the ledger "no I/O" finding and the `PredicateAuthority` finding.

No same-class owner or reader is missing. MEDIUM-I1 and MEDIUM-I2 correct two statements in §3 that the owner relies on
when choosing option (b). They should be folded in before the owner rules. The NITs are optional.
