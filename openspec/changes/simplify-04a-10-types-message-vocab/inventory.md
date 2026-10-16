# Inventory: simplify-04a-10-types-message-vocab

base: `e8c90fb9912f8691a146dab74fb4219042efad1f` (origin/main `2422d8fb975212a05532dd1940f4d9de24596994` plus the
empty claim commit; every line number below is the same on both)

Question: what in `pkg/types`, `message`, `payloadregistry` and `vocabulary` do #78, #219 and #90 ask to change,
who reads each exported symbol involved, and which kind of export (#190) each one is. Consumers read for symbol use
(`docs/inventory-scope.md`): semsource, semconnect, semteams, semboids, and for `vocabulary` semvocab and semsparql.
Their local trees were read with `grep -rn --exclude-dir=.worktrees` (no `git` command). semsource, semconnect,
semteams and semboids import `github.com/c360studio/semstreams/...` (pin or beta.160), so their uses of `message.X`
are uses of the pin's `message` package, which these packages port. semvocab imports `semengine` v0.1.0-alpha.2.
semsparql has no Go code in its checkout (only `LICENSE`).

Pin facts come from a snapshot of `8b99efe9` fetched with `gh api repos/C360Studio/semstreams/tarball/8b99efe9`
and unpacked outside this repository (local only: `/tmp/simplify-04a-10-evidence/pin`). Pin decision records come
from the same snapshot's `docs/adr`.

This change carries no package from the pin: all four are already ported (ledger rows `pkg/types` `:491`,
`message` `:750`, `payloadregistry` `:732`, `vocabulary` `:456`, all `adapt`). No pin probe is owed. Non-test Go
lines today: `pkg/types` 757, `message` 2,003, `payloadregistry` 434, `vocabulary` (root package) 2,470.

## 1. The claimed gaps (#219 items 1–5, #78, #90; cites re-measured)

- **#219 item 1, two validators for one type.** `pkg/types/type.go:51` — `func (mt Type) IsValid() bool {` checks
  non-empty only (`:52`); `:63` — `func (mt Type) Validate() error {` refuses empty, non-UTF-8 and `"."`
  (`:67-75`). **The issue's premise that `Validate` enforces `[a-z0-9_-]+` is false**: it checks those three things
  and nothing else. `message/base_message.go:152` — `if !m.msgType.IsValid() {`. CONFIRMED (the split exists).
- **#219 item 1, `EntityType`.** `pkg/types/entity_type.go:35` — `func (et EntityType) IsValid() bool {`. No
  `Validate`. Readers of `EntityType.IsValid`: none in this tree, none in pin production code, none in the six
  consumers (`grep -rnE 'EntityType\(\)\.IsValid|et\.IsValid|entityType\.IsValid'`: nothing). Readers of
  `EntityID.EntityType()` in production: none here (`git grep -n '\.EntityType()' -- ':!*_test.go'`: doc comments only),
  none in the consumers.
- **#219 item 2, generic JSON check.** `message/generic_json.go:99` — `func (g *GenericJSONPayload) Validate() error {`
  checks only `g.Data == nil`; `:120` —
  `if err := (shapeCheck{onPath: map[shapeKey]bool{}}).check(g.Data, "data"); err != nil {` runs inside
  `MarshalJSON`. The sort the issue calls double: `:184` — `for _, k := range slices.Sorted(maps.Keys(m)) {`, whose
  comment (`:183`) gives its reason: "so the refusal names the same key every time". CONFIRMED. **Constraint the
  issue does not name:** `openspec/specs/message-codec/spec.md:41` and `:63` make `GenericJSONPayload.MarshalJSON`
  itself refuse non-UTF-8 strings and non-JSON-shaped values. `encoding/json` would otherwise write each invalid
  byte as U+FFFD (spec `:37-39`, carried text). A payload marshalled without `Validate` first exists in this tree:
  `message/base_message.go:138` — `if data, err := m.payload.MarshalJSON(); err == nil {` (`Hash`).
- **#219 item 3, aliases.** `message/types.go:11` — `type Keyable = types.Keyable`, `:16` — `type Type = types.Type`,
  `:20` — `type EntityType = types.EntityType`, `:24` — `type EntityID = types.EntityID`, `:27` —
  `func ParseEntityID(value string) (EntityID, error) {`. **The issue's list is wrong**: `message.Payload`,
  `message.Message` and `message.Meta` are interfaces declared in `message` (`payload.go:50`, `message.go:20`,
  `meta.go:13`), and `BehavioralPayload` and `RawJSON` do not exist (`git grep -nE 'BehavioralPayload|RawJSON'`:
  nothing). **The claim body's premise "no reader outside `message`" is false for two of the five** (§ 4).
- **#219 item 3, `Triple.IsExpired`.** `message/triple.go:133` — `func (t Triple) IsExpired() bool {`. Readers: only
  `message/triple_helpers_test.go:308`. None at the pin outside its own test (`grep -rn 'IsExpired()'` hits other
  types only), none in the consumers. `message/triple_identity.go:32` — `//     FORWARD HAZARD: triple TTL is
  currently unenforced (Triple.IsExpired has` names it. The field `ExpiresAt` (`:99`) has production readers
  (`graph/helpers.go:113`, `processor/graph-ingest/canonical_mutations.go:660`,
  `pkg/projection/mutation_client.go:389`). CONFIRMED.
- **#219 item 4, string keys.** `payloadregistry/registry.go:84` — `registrations map[string]*Registration // Registry
  by message type string`; the key is spelled with `Sprintf` at `:77` — `return fmt.Sprintf("%s.%s.%s", pr.Domain,
  pr.Category, pr.Version)`, `:294` and `:313` — `typeStr := fmt.Sprintf("%s.%s.%s", domain, category, version)`, beside
  `pkg/types/type.go:41` — `return fmt.Sprintf("%s.%s.%s", mt.Domain, mt.Category, mt.Version)`. Every production lookup
  caller holds a `types.Type` and turns it into a string first: `processor/graph-ingest/canonical_mutations.go:217-218`
  — `key := entity.MessageType.Key()` / `if _, ok := c.payloadRegistry.GetRegistration(key); !ok {`,
  `processor/graph-ingest/component.go:677-678` — `containerType := inference.HierarchyContainerMessageType().Key()`,
  `:2063` — `floor, registered := c.payloadRegistry.IndexingProfileFor(mt.Key())`, and `message/base_message.go:315` —
  `payload := m.registry.Create(m.msgType.Domain, m.msgType.Category, m.msgType.Version)`. CONFIRMED.
- **#219 item 5, predicate memo.** `vocabulary/predicate_contract.go:84-87` — `validParsedPredicates sync.Map` and
  `validParsedPredicateCount atomic.Int64`; `:93` — `const maxMemoizedPredicates = 65536`; `:113` —
  `segments := strings.Split(predicate, ".")`. Readers of the memo outside the function: only
  `vocabulary/predicate_memo_test.go` (`:49`, `:88`, `:131`, `:146-148`, `:158`). CONFIRMED.
- **#219 item 5, `parseDomainCategory`.** `vocabulary/registry.go:332` —
  `func parseDomainCategory(name string) (domain, category string) {`. Callers: none, tests included
  (`git grep -n parseDomainCategory`: the declaration and its comment only). At the pin: also none
  (`vocabulary/registry.go:350`, no caller). CONFIRMED.
- **#78, `Hash`.** `message/base_message.go:127` — `func (m *BaseMessage) Hash() string {`; `:138` drops the marshal
  error. It is on the interface: `message/message.go:39` — `Hash() string`. Readers: only
  `message/base_message_test.go:168`, `:171`, `:174`. At the pin: no production caller (`grep -rnE '\.Hash\(\)'`
  over non-test files: nothing). Consumers: no `BaseMessage.Hash` caller, no `message.Message` implementer
  (`grep -rnP 'message\.Message\b'` in production: 0; the only `Hash() string` is semteams'
  `sandboxmanager/requirements.go:239`, an unrelated type). Two pin ADRs describe it (§ 6); none decides anything on
  it. CONFIRMED.
- **#90, `DcIdentifier`.** **The ruling's premise is false.** `vocabulary/standards.go:47` — `DcIdentifier =
  "http://purl.org/dc/terms/identifier"` is a Go constant on `main` (unchanged since `deaafd4`), and semconnect reads it
  in production: `parser/sensorml/predicates.go:129` — `vocabulary.Register(PredUniqueID,
  vocabulary.WithIRI(vocabulary.DcIdentifier))`. `vocabulary/README.md:300` — `DcIdentifier =
  "http://purl.org/dc/terms/identifier"` (indented, inside a Go block) sits in the README's Dublin Core block, which
  lists `standards.go` constants, and is accurate. So are `:232`, `:250` and `:266`. Nothing in the README is stale
  about it.
- **#90 item 3, capability interfaces.** One is left: `message/behaviors.go:27` —
  `type IndexingProfiler interface {`. It has a production reader (`processor/graph-ingest/component.go:1913` —
  `if profiler, ok := payload.(message.IndexingProfiler); ok {`), a consumer implementer (semsource
  `graph/event_payload.go:65`), and the ledger names it (`docs/admission-ledger.yaml`, `message` contract).

## 2. Every spelling of each fact the change touches

| Fact | Spellings |
| --- | --- |
| message-type grammar | `pkg/types/type.go:63-78` (`Validate`); `:51-53` (`IsValid`, non-empty only); `message/base_message.go:212-220` (a UTF-8 loop in `MarshalJSON`, there because `Validate` at `:152` uses `IsValid`, comment `:210-211`); `payloadregistry/registry.go:117-127` (three non-empty checks before `Validate` at `:134`) |
| dotted message-type key | `pkg/types/type.go:41`; `payloadregistry/registry.go:77`, `:294`, `:313` |
| generic JSON shape | `message/generic_json.go:120-225` only; `Validate` (`:99-104`) does not run it |
| predicate split | `vocabulary/predicate_contract.go:99-130` (`ParsePredicate`, the sole parser by pin ADR-074); `vocabulary/registry.go:332-359` (`parseDomainCategory`, dead); `vocabulary/entity_type.go:87` (`ParseEntityTypeKey`, two segments, same segment rule) |
| entity-type grammar | `pkg/types/entity_domain_authority.go:57` (`validateEntityIDSegment`: alphanumeric start, then alphanumeric, `_`, `-`); `vocabulary/entity_type.go:80-98` (`ParseEntityTypeKey`: lower-kebab); `pkg/types/entity_type.go:35-37` (`IsValid`: non-empty). The first two disagree: `Robotics_1` passes the first and fails the second |
| message content hash | `message/base_message.go:127-147` only. `message/triple_identity.go` keys triples, a different fact |

## 3. Adjacent claims

- Specs: `openspec/specs/message-codec/spec.md` (`:36-59` UTF-8, `:61-84` JSON-shaped, `:117-126` round trip).
  `vocabulary-packs` has no requirement on predicate grammar (`grep -n ParsePredicate openspec/specs`: nothing).
  `component-registration` `:18` names `RegisterPayloads(*payloadregistry.Registry) error` only.
- Ledger: the four rows above. `message` `known_risks` holds "Hash() ignores a payload marshal error, as at the pin …
  (tracked by #78)". `payloadregistry` `contract`: "New, Register, lookups and registration validation … as at the
  pin". `vocabulary` `contract`: "dotted-name and namespace validation … as at the pin". All `adapt`, so
  `task ledger:check` (carry rows only) does not compare them with the pin.
- Coverage floors: `scripts/cover-check.sh:27-28` — `"message unit"`, `"payloadregistry unit"` (80%).
- Docs naming touched symbols: `pkg/types/README.md:71-98` (the aliases), `pkg/types/doc.go:6`,
  `message/doc.go:78`, `:155-157` (`Keyable`), `message/message.go:11` ("Content-addressable: Hash method …"),
  `vocabulary/README.md:480` — `` - `message/types.go` - EntityID, EntityType, Type patterns ``.
- Open PRs (`gh pr list --state open --json number,title,changedFiles,files`, 2026-10-16): #275 (this claim, no
  files); #273 (draft, PR 7, `component`; OpenSpec files only so far; its design edits
  `processor/graph-ingest/component.go`, the `Register` and `ConfigSchema` sites, and `docs/admission-ledger.yaml`);
  #272 (draft, PR 9b, `pkg/lifecycle`; OpenSpec files only; its design edits `pkg/lifecycle` and the ledger); #242–#248
  (Dependabot: `go.mod`/`go.sum`, `package*.json`, one workflow). None touches the four packages' files. If owner
  question 4 is accepted, this change edits `processor/graph-ingest/component.go:678` and `:2063` and
  `canonical_mutations.go:218`, a file #273 also edits (other lines).
- Unclaimed wave PR touching a neighbour: PR 10 (`processor/graph-ingest`, #139).

### Open issues naming the packages

Queries: `gh issue list --state open --search "<q>" --limit 200` for `types`, `message`, `payloadregistry`,
`vocabulary`. Each hit's body and comments were searched for the package paths, `path:line` cites and symbols.

| Issue | Names it by | What it asks |
| --- | --- | --- |
| #219 | title, body | items 1–5 (§ 1) |
| #78 | title, `message/base_message.go:127` | `Hash` drops the marshal error |
| #90 | `vocabulary/README.md:300` (ruling), `message/behaviors.go` | ruled: PR 3 closes it; the README line it names is accurate (§ 1) |
| #33 | `message/generic_json.go:26` (pin cite) | doc text naming `payloadbuiltins`; the `message` half is done (`git grep payloadbuiltins -- message/`: nothing); the rest is graph-ingest, `internal/harness/payloadfixture/testing.go:15` and the unported `storage/objectstore` |
| #134 | item 5, the global `vocabulary` registry | graph/inference skips an inverse it cannot find; the work is in `graph/inference` |
| #139 | `message.Triple` | graph-ingest's append functions; the work is in graph-ingest |
| #9, #223 | rulings and the wave list | epics |

The other hits (#7, #20, #25–#27, #31, #36, #83, #84, #94, #103, #105, #110, #111, #131, #132, #135, #153, #154,
issue #161, #183, #189, #195–#198, #222) name the word, not the package.

## 4. Readers of each exported symbol involved

Production readers; "tests" means only tests read it. Consumer counts exclude `_test.go` and `.worktrees`.

| Symbol | This repo | Pin (non-test) | Consumers |
| --- | --- | --- | --- |
| `types.Type.IsValid` | `message/base_message.go:152`; `pkg/projection/mutation_client.go:130`, `:328`, `:331`; graph-ingest `canonical_mutations.go:234`, `component.go:1743`, `:2076` | `processor/rule/payload_projection.go:63` and the above | semconnect `gateway/cs-api/graph_mutations.go:140` — `if !mt.IsValid() {` |
| `types.EntityType.IsValid` | none | none | none |
| `message.Type` | 18 lines in 8 files, e.g. `graph/types.go:40` — `MessageType message.Type` | many | semsource 17 lines (`graph/event_payload.go:32`), semconnect 21 (`message/oms/payload.go:15`), semteams 13, semboids 4 |
| `message.Keyable`, `message.EntityType`, `message.EntityID` | none (tests: none outside `message`) | none (`vocabulary/doc.go:230`, `iris.go:31`, `:83`: doc comments) | none |
| `message.ParseEntityID` | `vocabulary/export/export.go:135` | `graph/clustering/summarizer.go:19`, `processor/research-graph-classify/adapters.go:179`, `vocabulary/export/export.go:127` | none |
| `message.IndexingProfiler` | `processor/graph-ingest/component.go:1913` | graph-ingest | semsource implements it (`graph/event_payload.go:65`) |
| `Triple.IsExpired` | tests | none | none |
| `Message.Hash`, `BaseMessage.Hash` | tests | none | none |
| `BaseMessage.Validate` | `message/base_message.go:193` | many | via `message.Message` |
| `GenericJSONPayload.Validate` | `message/generic_json.go:23` (`buildGenericJSONPayload`) | yes | not measured (consumers use the pin's type) |
| `Registry.GetRegistration` | graph-ingest `canonical_mutations.go:218`, `component.go:678` | graph-ingest | none |
| `Registry.IndexingProfileFor` | graph-ingest `component.go:2063` | graph-ingest | none |
| `Registry.Create` | `message/base_message.go:315` | `message` | semteams `scripts/measure-agent-contract-family.go:572` |
| `Registry.List` | none | none | semteams `scripts/measure-agent-contract-family.go:571` |
| `Registry.Build` | none | none | none |
| `Registration.MessageType` | `payloadregistry/registry.go:153` | same | none |
| `vocabulary.ParsePredicate` | `graph/entity_predicate_contract.go:98`; `vocabulary/namespace_authority.go:31`, `pack.go:299`, `predicates.go:366`, `registry.go:302`, `:393` | many | none directly (they call `vocabulary.Register`) |
| `vocabulary.DcIdentifier` | none | examples only | semconnect `parser/sensorml/predicates.go:129` |

## 5. Problem shape

"One validator owns a grammar; every other entry point delegates to it." The closest existing instances:
`pkg/types/type.go:55-62` ("It is the one owner of message-type component grammar") and pin ADR-074
("`vocabulary.ParsePredicate` is the sole parser"). Both are adopted, not added to. For the scan, the closest
existing `strings.Cut` split is `pkg/lifecycle/tags.go:330` — `name, _, _ := strings.Cut(tag, ",")`. No dotted-key
scan exists in production (`git grep -nE "strings\.IndexByte\([a-zA-Z]+, '\.'\)"`: nothing).

No collision table: the change adds no durable, communication or coordination primitive. No intent check: it moves
no boundary. No adoption sweep is owed: nothing new is a reusable primitive. One optional reader of the scan is noted
for later: `vocabulary/entity_type.go:87` (`ParseEntityTypeKey`) still uses `strings.Split`.

## 6. Pin decision records

None is carried; this change ports nothing. Searched in the pin snapshot's `docs/adr` for `ParsePredicate`,
`payloadregistry`, `GenericJSON`, `IsExpired|ExpiresAt`, `IsValid()`, `message/types.go|type alias|backwards compat`,
`IndexingProfiler`, `Type.Validate|types.Type`, `Hash()`. Records that govern what this change touches:

- **ADR-074** (Canonical Predicate Contract): `:30-36` — the three-segment lower-kebab grammar, 64-byte segments,
  194-byte total, and "`vocabulary.ParsePredicate` is the sole parser". Kept: the scan keeps the parser and its
  verdicts.
- **ADR-103** (The Payload Registry Is the Single Type Authority): Decision 1 (`:33-34`), a type is a type of the
  deployment if and only if it is registered. Kept: typed keys change the lookup's spelling, not the authority.
- **ADR-054** (Indexing Eligibility): `:169-171`, `IndexingProfiler`. Kept.
- ADR-056 `:1898` and ADR-057 `:21` (withdrawn) describe `Hash()` as an unauthenticated content digest; neither
  decides anything about it. ADR-059 `:45` lists `IsExpired` as context; it decides nothing about it.
- `GenericJSON`, `IsValid()`, `Type.Validate`: no record. The `type alias` hits (ADR-030, ADR-041) are other packages.

## 7. Findings outside the issues (for the design)

- **F-1, LOW, second spelling.** `payloadregistry/registry.go:117-127` checks each component non-empty, then `:134`
  runs `Validate`, which checks the same. Inherited; optional under Bounded port scope (the errors differ:
  `ErrInvalidConfig` here, the `Validate` text wrapped there).
- **F-2, LOW, doc.** `pkg/types/README.md:71-98` says `message` re-exports four types "for backwards compatibility";
  `vocabulary/README.md:480` points at `message/types.go` for patterns that live in `pkg/types`.
- **F-3, information.** Two entity-type grammars disagree (§ 2). Not this change's to settle; it matters only if an
  `EntityType.Validate` is added (design owner question 3).

## Adopter seams (existing surface)

- `message.Type`: semsource, semconnect, semteams and semboids write it 55 times in production. Removing it would
  be found by a compile error at cutover, in every file that names a message type.
- `Type.IsValid` tightened: semconnect's check (`graph_mutations.go:140`) would refuse a dotted or non-UTF-8 type
  itself instead of at the registry lookup. Such a type can never be registered (`registry.go:134`), so the outcome
  (refused, invalid class) is the same and arrives earlier.
- `GenericJSONPayload.Validate` tightened: a caller that validates first learns of a non-JSON-shaped value then, not
  at encode. Same class, same path in the message.
- Removed symbols (`Hash`, three aliases, `IsExpired`, `EntityType.IsValid`): no consumer names them; a later reader
  finds out through a compile error.
- Registry lookups typed: no consumer calls `GetRegistration` or `IndexingProfileFor`; a caller passing a string
  gets a compile error naming the parameter type.
