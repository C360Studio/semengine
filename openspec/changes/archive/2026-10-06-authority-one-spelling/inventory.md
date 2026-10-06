# Inventory: #72, every spelling of the deployment authority pair

base: SemStreams pin `8b99efe9` (`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`); SemEngine `main` `2ec3bcf`; PR #48 head
`c64ac338` (branch `claude/setup-04a-01-floor`, worktree read with Read/Grep only; 171 files per
`gh api --paginate repos/C360Studio/semengine/pulls/48/files`). The #48 worktree has since dropped
`message/federation.go` (`message/meta_wire_test.go:19` cites the removal) but nothing is pushed, so the pushed head
stays the base.
revision: round 2 — the independent review (BLOCKING: B1–B4, F5) is folded in; every added citation was re-read at
the pin, not copied from the review. `INVENTORY PASS` in round 2 (issue #72); the owner ruled on it in #72 comment
5969505488. This file is the copy the change carries; the owner's rulings A and C are what `design.md` implements.
status: inventory only — architect contract category 2, scoped to one fact. No target state, options, rule text or
enforcer is proposed here.

## 0. Question, scope, method

- **Question** (`docs/inventory-scope.md` rule 1): across the tier-0 port set at `8b99efe9`, every place the
  originating deployment's authority (`platform.org` + `platform.id`) is computed, declared, interpreted, persisted or
  carried, and for each: ported already by #48, ported by a later 04A change, consolidated, or left behind.
- **Repositories read:** SemStreams at the pin only — the whole snapshot was fetched once with
  `gh api repos/C360Studio/semstreams/tarball/8b99efe9` into the scratchpad (`pintree/`), and every `path:line` below
  was read from that copy; SemEngine `main`; PR #48's worktree. No sister checkout, no git command against one, no
  consumer repository (the brief's "no other repository"), so semsource's `cmd/semsource/run.go:327` and
  `entityid.MaxOrgLen` from the pin inventory are not re-read (§5 Q3).
- **Port set:** the 65 packages of `openspec/changes/archive/2026-10-01-setup-04a-foundation/design.md:155-178` (D2),
  plus `graph/llm`, `model/wire` dormant. Outside it, and therefore "left behind" wherever cited below: `cmd/*`,
  `internal/boot`, `internal/bootstrapobservability`, `internal/e2eboot`, `internal/entityidaudit`, `agentic/*`,
  `processor/agentic-*`, `processor/research-graph-*`, `examples/*`, `test/*`, `gateway/*`.
- **Baseline drift:** the pin inventory (`docs/proposals/gh1168-federation-identity-inventory.md`, same text at the
  pin as the scratchpad copy; `:307-312` verified) was measured at `d635e68`, before ADR-104 (entropy suffix minted
  onto `platform.id` at first boot) and #1188 (per-deployment config bucket) landed. Both are at the pin, and they
  move or retire a third of Fact B's citations (§3).
- **Why round 1 missed B1–B3.** Round 1 swept by *name of the fact* (`deps.Platform`, `platform.Config`,
  `GetOrg/GetPlatform`, `json:"org"`, `vocabulary`, `provenance`, subject grammars) and never swept by *shape*: a
  struct field or local named `Org`/`Platform` read off a parsed ID, or a function taking `(org, platform, …)`. The
  two searches that find all of B1–B3 in one pass, over port-set production Go:
  `grep -n -E '\b(parsed|eid|ep|e\.platform|r\.platform|rp\.platform|pendingRunMint)\.(Org|Platform)\b|\bOrgPlatform\b'`
  (hits: `graph/clustering/summarizer.go` 4, `processor/rule/actions.go` 3, `graph/llm/prompts.go` 1,
  `graph/llm/prompt_data.go` 1, plus the sites already listed) and
  `grep -rn -E 'func .*\(.*\borg, platform\b|IdentityFamily\(\)\.EntityID\(' --include='*.go' | grep -v _test.go`
  (port set: `pkg/types/framework_identity_families.go:77`, `pkg/types/entity_id_authority.go:35`,
  `processor/rule/graph_event_identity.go:22, :33`, `graph/events.go:196`, `config/config.go:865`; the other 15 hits
  are `agentic/*` and `processor/agentic-*`, outside). `graph/llm` was in the port-set list and in no sweep.

## 1. Category 2 — every spelling at the pin, with its SemEngine placement

Placement key: **#48** = in PR #48 at `c64ac338`; **ch.n** = 04A change n per D2; **outside** = not in the port set.
"Readers (port set)" counts production (non-`_test.go`) lines inside the 65 packages only; a caller outside the port
set is not a caller here.

### 1.1 The source and its validators (declared pair)

| # | Spelling | Pin citation — line text | Placement | Readers (port set) |
|---|---|---|---|---|
| 1 | the struct | `pkg/platform/platform.go:30` — `` Org          string   `json:"org"` `` ; `:31` — `` ID           string   `json:"id"` ``; doc `:23-28` — "positions 1-2 of every identity this deployment mints … round-trips cleanly through config files and embedded message metadata" | **#48** (`pkg/platform` row, carry; row's `consumer_purpose` cites "seven `message` symbols and `vocabulary.EntityIRI`") | `config` (ch.3), `message` (#33), `vocabulary` (#34) |
| 2 | alias | `config/config.go:29` — `type PlatformConfig = platform.Config` | ch.3 | `config/config.go:49`, `minimal_config.go:12` |
| 3 | the config field | `config/config.go:49` — `` Platform      PlatformConfig       `json:"platform"` `` | ch.3 | `config` |
| 4 | second config type, third validator | `config/minimal_config.go:12` — `` Platform PlatformConfig     `json:"platform"` ``; `:25-27` — `if c.Platform.ID == "" { return errors.New("platform.id is required") }` (no org check, no bound, no ADR-104 reserve) | ch.3; production callers at the pin **0** (`grep -rn MinimalConfig --include='*.go' \| grep -v _test` → only `service/doc.go:249`, a comment) — a surface-audit item for ch.3, not consolidated anywhere | 0 |
| 5 | env override, id only | `config/config.go:388` — `envPrefix:  "STREAMKIT",`; `:747-748` — `if val := os.Getenv(l.envPrefix + "_PLATFORM_ID"); val != "" { cfg.Platform.ID = val }`; `:745-770` has no `_PLATFORM_ORG` | ch.3 | `Load :423`, `LoadFromBytes :459` |
| 6 | full validator | `config/config.go:226-227` — `if c.Platform.Org == "" { return errors.New("platform.org is required")`; `:231` — `c.Platform.Org = strings.ToLower(c.Platform.Org)`; `:234` — `if !isValidNATSSubjectPart(c.Platform.Org)` (admits `.`, `:295`); `:241-242` id required; `:244` — `if err := validateAuthorityPair(c.Platform.Org, c.Platform.ID); err != nil` | ch.3 | `Load`, `SafeConfig.Update/Mutate` |
| 7 | declared-pair bound (163) | `config/config.go:829` — `func validateDeclaredAuthorityPair(org, id string) error`; `:806` — `const mintedSuffixBytes = len("-") + 6`; `:817-819` — `maxDeclarableAuthorityPairBytes() … MaxAuthorityPairBytes() - mintedSuffixBytes`; called `:428`, `:462` on every load | ch.3 | 2 |
| 8 | effective-pair bound (170 + compose) | `config/config.go:857` — `func validateAuthorityPair(org, id string) error`; `:859` — `if pair := len(org) + len(id); pair > semtypes.MaxAuthorityPairBytes()`; `:865` — `binding.EntityID(org, id, strings.Repeat("0", binding.InstanceBytes))` | ch.3 | `:244`, `manager.go:1107` |
| 9 | the budget | `pkg/types/framework_identity_families.go:64-66` — `func MaxAuthorityPairBytes() int { return MaxEntityIDBytes - LongestFrameworkIdentityFamily().FixedBytes() }` | **#48** | `config/config.go:818, 859, 862` (ch.3) |
| 10 | removed-field probe | `config/config.go:876-877` — `removedPlatformFields … "instance_id": "removed (ADR-102, BREAKING): platform.id is the single deployment authority field …"`; `:882` — `func rejectRemovedPlatformFields(raw map[string]any) error` | ch.3 | `:494`, `:550` |

### 1.2 The mint and its durable record (effective pair), none in #48

| # | Spelling | Pin citation — line text | Placement | Readers (port set) |
|---|---|---|---|---|
| 11 | the record | `config/manager.go:88` — `const platformIdentityKVKey = "platform_identity"`; `:94-101` — `` type platformIdentityRecord struct { Org string `json:"org"`; Stem string `json:"stem"`; ID string `json:"id"` } `` ("a cross-repo contract (ADR-104)", `:91`) | ch.3 | `manager.go` only |
| 12 | establish / mint / adopt | `config/manager.go:346` — `hasConfig, err := cm.establishPlatformIdentity(ctx, kvStore)` (before arbitration, `:342-345`); `:1021` establish (three branches `:1041-1059`); `:1080` — `func (cm *Manager) mintPlatformIdentity(…)`; `:1099` — `record := platformIdentityRecord{Org: declared.Org, Stem: declared.ID, ID: declared.ID + "-" + suffix}`; `:1115` — `kvStore.Create(ctx, platformIdentityKVKey, data)`; `:1200-1205` — `mintIdentitySuffix` … `rand.Read(raw)`; `:1211` adopt; `:1285-1288` — `applyEffectivePlatformID … current.Platform.ID = id` | ch.3 | `Start` |
| 13 | bucket name from the DECLARED pair | `config/bucket_name.go:31` — `func BucketName(org, stem string) (string, error)`; `:44` — `name := graph.BucketSemStreamsConfig + "_" + org + "_" + stem`; `config/manager.go:123` — `bucketName, err := BucketName(cfg.Platform.Org, cfg.Platform.ID)` | ch.3 | 1 |
| 14 | catalog descriptor for that family | `graph/constants.go:74` — `BucketSemStreamsConfig = "semstreams_config"` (doc `:60-66`: "named by its declared authority pair … each member holds the create-once platform identity record"); `graph/kvcatalog.go:104-130` (descriptor; `:190` — `var nameFamilies = map[string]bool{BucketSemStreamsConfig: true}`) | ch.2 (`graph`) | `config/manager.go:192` (ch.3) |
| 15 | KV `platform` mirror | `config/manager.go:930-931` — `if data, err := json.Marshal(cfg.Platform); … kvStore.Put(ctx, "platform", data)` (effective pair, after `applyEffectivePlatformID`); `:720-724` — "The KV \`platform\` key is a PUBLISHED MIRROR, never a source … deliberately has no case here"; watched `:158` | ch.3 | 0 readers in the port set (UI mirror) |
| 16 | mint log | `config/manager.go:1122-1123` — `cm.logger.Info("Minted platform identity", "org", record.Org, "stem", record.Stem, "platform", record.ID)`; adopt `:1272` | ch.3 | — |

### 1.3 Extraction and the in-process carrier (`deps.Platform`)

| # | Spelling | Pin citation — line text | Placement | Readers (port set) |
|---|---|---|---|---|
| 17 | accessors | `config/config.go:790-791` — `func (c *Config) GetOrg() string { return c.Platform.Org }`; `:796-797` — `func (c *Config) GetPlatform() string { return c.Platform.ID }` | ch.3 | **0** inside the port set — the only callers are `internal/boot/run.go:475-476` (outside) |
| 18 | the extractor, after the mint | `internal/boot/run.go:157` — `configManager, effectiveConfig, err := bootstrapobservability.StartValidatedConfigManager(`; `:198-199` — `cfg = effectiveConfig` / `platform := extractPlatformMeta(cfg)`; `:200` — `logger.Info("Platform identity configured",`; `:471-477` — `func extractPlatformMeta(cfg *config.Config) types.PlatformMeta { … Org: cfg.GetOrg(), Platform: cfg.GetPlatform() }` | **outside** (03B design `:562`: `internal/boot` "is not ported"; `internal/bootstrapobservability` not in D2) — the ordering "extract after `Manager.Start`" lives in no ported package | n/a |
| 19 | the carrier type | `types/component.go:134-137` — `type PlatformMeta struct { Org string; Platform string }` | ch.2 (`types`) | `component`, `service`, `processor/rule`, `processor/graph-ingest` |
| 20 | alias + component deps | `component/dependencies.go:21` — `type PlatformMeta = types.PlatformMeta`; `:73` — `Platform        PlatformMeta              // Platform identity (organization and platform)` | ch.2 | 27 lines in 6 packages (§3, `deps.Platform` row) |
| 21 | service deps and the copy | `service/dependencies.go:31` — `Platform          types.PlatformMeta           // Platform identity`; `service/component_manager.go:51` — `platform         types.PlatformMeta`; `:201` — `platform = deps.Platform`; `:1210-1213` — `Platform: component.PlatformMeta{ Org: cm.platform.Org, Platform: cm.platform.Platform, }` | ch.3 | 1 |

### 1.4 Component-held copies of `deps.Platform` (in-process, never serialized)

| # | Spelling | Pin citation — line text | Placement | Readers (port set) |
|---|---|---|---|---|
| 22 | graph-ingest | `processor/graph-ingest/component.go:596-597` — `org      string` / `platform string`; `:721-723` — `if deps.Platform.Org == "" \|\| deps.Platform.Platform == "" { … "deps.Platform must carry the deployment authority (platform.org and platform.id)"`; `:772-773` — `org: deps.Platform.Org, platform: deps.Platform.Platform,` | ch.2 | gate `authority_gate.go:52`; hierarchy wiring `:1438-1439` |
| 23 | hierarchy inference | `graph/inference/hierarchy.go:100-101` — `` Org      string `json:"-"` `` / `` Platform string `json:"-"` `` (doc `:88-93`: "json:\"-\" on purpose … graph-ingest sets them from its own deps.Platform read"); `:202-204` refuses an empty pair; set at `processor/graph-ingest/component.go:1438-1439` — `Org: c.org, Platform: c.platform,` | ch.2 | `:217` |
| 24 | rule engine | `processor/rule/processor.go:49` — `platform            types.PlatformMeta // deployment authority for every minted identity (ADR-102)`; `:356` — `func (rp *Processor) SetPlatform(platform types.PlatformMeta)`; `factory.go:124-126` refuses an empty pair; `processor.go:620-624` — `if rp.natsClient != nil && (rp.platform.Org == "" \|\| rp.platform.Platform == "") { return errs.WrapInvalid( fmt.Errorf("rule processor has no deployment authority (platform.org and platform.id); CreateRuleProcessor installs it from deps.Platform, a direct NewProcessor caller must call SetPlatform before Start")` (third refusal, at `Start`); `:144` — `processor.SetPlatform(deps.Platform)`; `actions.go:556` — `platform types.PlatformMeta`; `expression_factory.go:21, :86` — `func NewExpressionRule(platform types.PlatformMeta, packID string, def Definition)`; `test_rule_factory.go:18, :29` | ch.6 | 15 lines (`grep`, §4) |
| 25 | alert constructor params | `graph/events.go:175-176` — `func NewAlertEvent( org, platform, alertType, sourceEntityID string,` (doc `:172-174`: "carried as deps.Platform — never a payload value, a constant, or the source entity's own authority"); composes at `:196` — `alertID, err := semtypes.RuleAlertIdentityFamily().EntityID(org, platform, alertInstance(sourceEntityID, alertType, metadata))` | ch.2 (`graph`); production callers at the pin **0** (`grep -rn 'NewAlertEvent(' --include='*.go' \| grep -v _test` → the definition only) — a surface-audit item for ch.2 | 0 |
| 42 | rule's agentic run mint (B3) | `processor/rule/actions.go:1855-1859` — `pendingRunMint = &struct { org string; platform string; firingLoopID string }{org: e.platform.Org, platform: e.platform.Platform, firingLoopID: firingLoopID}` (doc `:1851-1852`: "deps.Platform is the only honest source (ADR-102 d2)"); `:1989-1990` — `if _, mintErr := agentrun.Mint(ctx, e.lifecycle, pendingRunMint.org, pendingRunMint.platform, pendingRunMint.firingLoopID, entityID); mintErr != nil {`; `:2009` — `e.stampRunAnchors(ctx, ec, entityID, pendingRunMint.org, pendingRunMint.platform, pendingRunMint.firingLoopID)`; import `actions.go:16` — `"github.com/c360studio/semstreams/agentic/agentrun"` | **left behind**: `processor/rule` is ch.6, but the `publish_agent` path is one of the four agentic edges E1–E4 cut at port time (04A design `:429` — "the rule core's four agentic edges (`actions.go`, …)"; `:449`); `agentic/agentrun` is outside the port set | 0 after the cut |

### 1.5 Interpreters of positions 1-2 (read the pair back out of an ID)

| # | Spelling | Pin citation — line text | Placement | Readers (port set) |
|---|---|---|---|---|
| 26 | the ID's fields | `pkg/types/entity_id.go:92-93` — `Org      string // Organization namespace (e.g., "acme")` / `Platform string // Minting deployment authority (e.g., "dep1"), from platform.id`; doc table `:77-78` — `platform.org (config)` / `platform.id (config, via deps.Platform)` | **#48** | every parser user |
| 27 | prefix accessors | `pkg/types/entity_id.go:268-270` — `func (eid EntityID) DeploymentPrefix() string { return eid.Org + "." + eid.Platform }`; `:274-276` `SourcePrefix` | **#48**; production callers outside `pkg/types` **0** (`grep -rn 'DeploymentPrefix(' --include='*.go'` → def, `:275`, 3 test lines) | 0 |
| 28 | the authority gate primitive | `pkg/types/entity_id_authority.go:35` — `func ValidateEntityIDAuthority(candidate, org, platform string, importLane bool) error`; `:12` `foreign_authority`, `:15` `local_authority_claimed` | **#48** | `processor/graph-ingest/authority_gate.go:52` (ch.2); `graph/inference/hierarchy.go:217` (ch.2); `processor/rule/actions.go:633` (ch.6) |
| 29 | the import lane (which pair is admitted) | `component/port_codec.go:63` — `"import": {Type: "bool", Editable: true, Directions: []Direction{DirectionInput}},`; `component/port_resolver.go:89` — `return ok && stream.Import()`; `processor/graph-ingest/keyed_ingest.go:38` — `importLane bool`; `authority_gate.go:51-52` — `func (c *Component) authorizeSubject(subject string, importLane bool) error { return semtypes.ValidateEntityIDAuthority(subject, c.org, c.platform, importLane) }` | ch.2 | 1 gate |
| 30 | rule substitution tokens | `processor/rule/entity_substitution.go:51-52` — `var entityPartNames = [6]string{ "org", "platform", "system", "domain", "type", "instance", }`; `:58-59` — `parsed.Org, // $<prefix>.org` / `parsed.Platform, // $<prefix>.platform` | ch.6 | rule templates |
| 31 | export IRI grammar (from the ID) | `vocabulary/export/export.go:127-130` — `eid, err := message.ParseEntityID(subject) … fmt.Sprintf("%s/entities/%s/%s/%s/%s/%s/%s", base, eid.Org, eid.Platform, eid.System, eid.Domain, eid.Type, eid.Instance)` | ch.7 (`vocabulary/export`) | `resolveSubjectIRI :139` |
| 43 | LLM prompt parts (B1) | `graph/llm/prompt_types.go:14-15` — `Org      string // Part 0: Organization` / `Platform string // Part 1: Platform` (`EntityParts`, `:12`); `graph/llm/prompt_data.go:8` — `OrgPlatform    string        // Common org.platform if uniform`; rendered `graph/llm/prompts.go:36` — `{{if .OrgPlatform}}Organization/Platform: {{.OrgPlatform}}` and `:47` — `org={{.Org}} platform={{.Platform}} domain={{.Domain}} …`; **taught meaning** `prompts.go:22-23` — `- org: Organization identifier (multi-tenancy)` / `- platform: Platform/product within organization`, which `entity-id-contract/spec.md:401-402` forbids ("MUST NOT be taken from … a product name") | **left behind at the seam**: `graph/llm` enters ch.2 as a dormant copy (D8, 04A design `:439-440`) and is deleted in ch.7 (`:449-451`); no tier-0 reader after the seam | `graph/clustering/summarizer.go` (#44) only |
| 44 | clustering summarizer (B1) | `graph/clustering/summarizer.go:690-691` — `func parseEntityID(entityID string) llm.EntityParts { ep := llm.EntityParts{Full: entityID}`; `:696-697` — `ep.Org = parsed.Org` / `ep.Platform = parsed.Platform` (from `semtypes.ParseEntityID`, `:692`); `:728-729` — `op := parsed.Org + "." + parsed.Platform` / `orgPlatforms[op]++` — the string `EntityID.DeploymentPrefix()` returns (#27), built by hand on the `llm.EntityParts` copy; `:803` — `OrgPlatform:    orgPlatform,` | ch.4 (`graph/clustering`, imports `graph/llm` at `summarizer.go:12`); the ch.7 seam has to cut or re-home it with `graph/llm` | 1 |

### 1.5b Composers: where the pair becomes positions 1-2 of an identity (B2)

| # | Spelling | Pin citation — line text | Placement | Readers (port set) |
|---|---|---|---|---|
| 45 | the one family composer | `pkg/types/framework_identity_families.go:77` — `func (f FrameworkIdentityFamily) EntityID(org, platform, instance string) (string, error) {` (doc `:74-76`: "composes and validates a member identity under the given authority … fails closed on … an empty or dotted authority segment") | **#48** | `graph/events.go:196` (#25, ch.2); `processor/rule/graph_event_identity.go:33` (#46, ch.6); `config/config.go:865` (#8, ch.3, probe only); `agentic/web_observation_entity.go:240` (outside) |
| 46 | rule trigger identity | `processor/rule/graph_event_identity.go:22` — `func ruleTriggerEntityID(org, platform, packID, ruleID string) (string, error) {`; `:33` — `entityID, err := semtypes.RuleTriggerIdentityFamily().EntityID(org, platform, hex.EncodeToString(digest.Sum(nil)))` (doc `:20-21`: "two deployments running the same pack do not [converge], because the authority differs") | ch.6 | `expression_factory.go:349` — `ruleTriggerEntityID(r.platform.Org, r.platform.Platform, r.packID, r.id)`; `test_rule_factory.go:132` (same text) |

### 1.6 Second carriers of `platform.Config` itself (the fact modeled twice)

| # | Spelling | Pin citation — line text | Placement | Readers (port set) |
|---|---|---|---|---|
| 32 | envelope carrier (S8) | `message/federation.go:22` — `type FederationMeta interface {`; `:36` `DefaultFederationMeta`; `:50` — `func NewFederationMeta(source string, pcfg platform.Config) *DefaultFederationMeta {` (`:53` `uid: uuid.New(),`); `:60` `NewFederationMetaWithTime`; `:73` `UID()`; `:78` — `func (m *DefaultFederationMeta) Platform() platform.Config {`; `:95` `GetPlatform`; `:105` `GetUID`; `:90` — `//     globalID := BuildGlobalID(entityID, platform)` (no such symbol anywhere at the pin); `message/base_message.go:81` — `func WithFederation(pcfg platform.Config) Option {`; `:91` `WithFederationAndTime`; never serialized: `:236-238` write only `created_at`/`received_at`/`source`, `:290` — `m.meta = NewDefaultMetaWithReceivedAt(createdAt, receivedAt, source)` | **#48 at `c64ac338` still carries all of it** (`message/federation.go` in the file list; worktree `message/base_message.go:79-97` unchanged). Ruled removed in #48's surface audit (#72 comment 5969293525); the removal has not landed at that head, and #48's tasks/ledger do not yet name it (`grep -n federation tasks.md design.md` → design P22 `:647` only) | **0** callers at the pin, prod and test (`grep -rn 'FederationMeta\|WithFederation\|GetUID(\|message.GetPlatform(' --include='*.go'` → definitions and doc comments only) |
| 33 | the advertising | `message/doc.go:332` — `//     WithFederation(platformConfig))`; `:463` — `//   - Add options only when needed: WithTime(), WithFederation()`; `message/base_message.go:36, :40, :116, :120` (doc examples); `graph/README.md:140` — `- **message/** owns: Transport primitives (EntityID, Triple, FederationMeta)`; `message/triple.go:36` — `//   - Federation: Works with federated entity IDs from multiple sources` (advertises federation; no second spelling); `pkg/platform/platform.go:27-28` — "round-trips cleanly through config files and embedded message metadata" | `message/*`, `pkg/platform` **#48**; `graph/README.md` ch.2 | — |
| 34 | IRI grammar from `platform.Config` (no org) | `vocabulary/iris.go:85` — `func EntityIRI(dottedType string, pcfg platform.Config, localID string) string {`; `:86` — `if pcfg.ID == "" \|\| localID == "" {`; `:104-106` — `if pcfg.Region != "" { return fmt.Sprintf("%s/entities/%s/%s/%s/%s/%s", SemStreamsBase, pcfg.ID, pcfg.Region, domain, entityType, localID)`; `:109-110` — `SemStreamsBase, pcfg.ID, domain, entityType, localID)`; doc `:70` — `{platform_id}[/{region}]/{domain}/{type}/{local_id}` | **#48** (`vocabulary` row, adapt); it is the one non-`message` reason the `pkg/platform` row gives for the package being public | production callers at the pin **0** (`grep -rn 'EntityIRI(' --include='*.go'` → def, its doc example, `vocabulary/iris_test.go:138, :298`). The pin inventory never lists it |

### 1.7 Wire fields carrying the pair (the #1154 class) — all outside the port set

| # | Spelling | Pin citation — line text | Placement |
|---|---|---|---|
| 35 | agentic typed messages | `agentic/loop_execution_entity.go:80-81` — `` Org      string       `json:"org"` `` / `` Platform string       `json:"platform"` ``; `agentic/ops_diagnosis_entity.go:70-71`; `agentic/web_observation_entity.go:66-67`; `agentic/model_endpoint_entity.go:39-40`; `agentic/agent_lesson_entity.go:198-199` | **outside** (`agentic` is behind the 04A seam) — left behind; at the pin these are the only production `json:"org"`/`json:"platform"` fields besides `platform.Config` and the `platform_identity` record (`grep -rn 'json:"\(org\|platform\)' --include='*.go' \| grep -v _test`) |
| 36 | e2e mission command | `cmd/e2e-semstreams/mission/command.go:52-53` — `` OrgID     string `json:"org_id"` `` / `` Platform  string `json:"platform"` `` | **outside** |

### 1.7b Same name, different fact: for an enforcer to exclude (F5)

| # | Spelling | Pin citation — line text | Placement | Classification |
|---|---|---|---|---|
| 47 | caller's org claim | `processor/rule/caller_substitution.go:51` — `Org string` on `CallerContext` (`:37`); doc `:47-50` — "Org is the caller's organization claim. For most deployments this matches Platform.Org (same-org request), but is kept as a separate field … for future cross-org scenarios at the proxy tier"; `:68` — `result = strings.ReplaceAll(result, "$caller.org", cc.Org)`; carried at `processor/rule/execution_context.go:150` — `Caller *CallerContext`, doc `:227` — `$caller.org: Caller organization claim (caller-aware rules only)` | ch.6 | **not a spelling of the deployment authority**: a request-scoped claim supplied by the caller, never read as positions 1-2 and never minted from `platform.Config`. Not counted in §2. Its doc's "matches Platform.Org" is the naming coincidence the architect contract's "parallel declaration" tell describes |

### 1.8 Decision records and specs that state the rule (ported as documents)

| # | Spelling | Pin citation — line text | Placement |
|---|---|---|---|
| 37 | ADR-102 | `docs/adr/102-entity-id-segment-semantics.md:57` — "`platform` is the composition root's own identity field — `platform.id`; `instance_id` leaves identity (O-2)"; `:116` — "A sister conforms when: `platform` = its composition root's `platform.id`" | ruled into **#48** (#72 comment 5969293525); not in the worktree at `c64ac338` (`ls docs/adr` → no such directory) |
| 38 | ADR-104 | `docs/adr/104-unique-platform-authority.md:28-30` — "`platform.id` is unique by default … mints a six-hex-byte entropy suffix from `crypto/rand`, records it once with an atomic `Create`, and adopts it on every later boot"; `:143-148` — cross-repo contract: "its composition root passes `deps.Platform` unchanged; its configuration files declare the STEM … read the effective pair from `semstreams_config/platform_identity`"; `:94-100` — decision 7, the `(org, id, environment)` guard, which `:127-128` and the spec below say #1188 retired | ruled into **#48**; not in the worktree |
| 39 | entity-id-contract spec | `openspec/specs/entity-id-contract/spec.md:400-402` — "`platform` is the minting deployment authority: the composition root's `platform.id`, carried to components as `deps.Platform`, and MUST NOT be taken from a payload, a constant, a product name, or a firing entity"; `:406-407` — "Every framework-derived family … MUST carry the deployment's own `org.platform`; a fixed framework literal in positions 1–2 is not a valid authority"; `:513-521` (declared pair ≤ 163); `:624-630` (cloned template never shares a pair) | ruled into **#48**; not in the worktree (`ls openspec/specs` → five harness specs only) |
| 40 | component-runtime-config spec | `openspec/specs/component-runtime-config/spec.md:349-367` (establish/adopt/mint/refuse, three branches); `:369-371` — "`platform.environment` SHALL NOT separate deployments: no bucket name, key, guard, or comparison reads it"; `:373-377` — "The record SHALL carry exactly the fields `org`, `stem`, and `id` … Configuration synchronization SHALL NOT apply the KV `platform` key"; `:547` — bucket `semstreams_config_<org>_<stem>` | ch.3 (with `config`); no ruling names it |
| 41 | concept doc | `docs/concepts/16-federation.md:34-35` — "`org.platform` is the **minting deployment authority** — the composition root's `platform.org` / `platform.id`"; `:56-58` — "nothing coordinates that pair automatically: two deployments that choose the same `platform.org` / `platform.id` mint colliding identities"; `:60` — "What the boundary enforces today is the lexical contract only."; `:64` — "Until it lands, an entity claiming a foreign or colliding authority is accepted as local truth." | ruled into **#48**; not in the worktree. `:56-58` contradicts ADR-104 d1 (#38) at the pin itself; `:60-64` is false at the pin: the gate (#28, #29) is wired at `processor/graph-ingest/canonical_mutations.go:244, :307, :383, :474` and `component.go:1743, :2065, :2224, :2306, :2449, :2591` — ten `c.authorizeSubject(…)` calls → `authority_gate.go:51-52` → `ValidateEntityIDAuthority` |

## 2. Placement totals

Code spellings (#1-#36, #42-#46): **41** (#47 is listed and classified out). Of these: already in **#48** at
`c64ac338`: 9 (#1, #9, #26, #27, #28, #32, #33's `message` and `pkg/platform` lines, #34, #45); later 04A changes:
27 — ch.2: 7 (#14, #19, #20, #22, #23, #25, #29), ch.3: 15 (#2-#8, #10-#13, #15-#17, #21), ch.4: 1 (#44), ch.6: 3
(#24, #30, #46), ch.7: 1 (#31); **left behind**: 5 — outside the port set with zero port-set readers (#18, #35,
and #36), cut from a ported package at port time (#42, by E1–E4 in ch.6), or carried dormant and deleted at the
seam (#43, `graph/llm`, ch.2 → ch.7). Consolidated by any change so far: **0** — every carried copy (#19-#25, #42) is a
copy of one `deps.Platform` read, which is the pin's own consolidation; the second carriers of `platform.Config`
(#32, #34) are not consolidated anywhere at `c64ac338`: #32 is ruled for removal and not yet removed (pending in the
worktree); #34 is ported with no caller and no ruling; the hand-built prefix (#44) duplicates #27 and both survive.
Documents (#37-#41): four ruled into #48 and absent from its worktree; one (#40) rides with `config` in ch.3.

## 3. Re-pin of the pin inventory's Fact B, S1–S8 and table 3.2

| Pin-inventory claim (at `d635e68`) | At `8b99efe9` |
|---|---|
| "no entropy source exists under `config/`" (Fact B; §5.1) | **false at the pin**: `config/manager.go:1200-1205` `mintIdentitySuffix` reads `crypto/rand` (import `rand`) — ADR-104 |
| Extractors `cmd/semstreams/main.go:523-530`, `cmd/e2e-semstreams/main.go:677-684` | moved to `internal/boot/run.go:471-478`, one extractor for both binaries, outside the port set (#18) |
| `config.go` anchors `:224-247`, `:733-758`, `:777-786`, `:795-807`, `:388`, `:423→427` | `:224-246`, `:745-770`, `:790-798`, `:857-869`, `:388`, `:423→428→434`; plus the new `:806`, `:817-819`, `:829-840` (declared-pair reserve) |
| S7: KV `semstreams_config` is "a fixed global bucket shared by every sem* app", Start refuses on an `(org, id, environment)` mismatch read from the KV `platform` key (`manager.go:172-215, 866-894`) | **retired (#1188)**: bucket is `semstreams_config_<org>_<stem>` (`config/bucket_name.go:44`); identity is established from the `platform_identity` record (`manager.go:1021-1060`), and `component-runtime-config/spec.md:369-371` + `pkg/platform/platform.go:36-42` say environment separates nothing. ADR-104 `:94-100` still prints decision 7 (the environment guard) and `:127-128` says #1188 retired it — the ADR text is internally stale at the pin |
| `NewConfigManager` creates `context.Background()` at `manager.go:73` (Q9) | fixed: `manager.go:116-120` "No I/O here, and no context" |
| KV `platform` key update is applied to in-memory config (`manager.go:567-570`) | reversed: `manager.go:720-726` "PUBLISHED MIRROR, never a source … deliberately has no case here" |
| `MinimalConfig.Validate` third validator home (`minimal_config.go:24-33`) | unchanged (`:24-27`), still 0 production callers |
| S8 `FederationMeta` family, 0 callers, never serialized, `BuildGlobalID` phantom | unchanged line-for-line (`federation.go:22-110`, `base_message.go:79-97, 233-238, 271-290`) |
| `DeploymentPrefix` 0 callers outside `pkg/types` (`entity_id.go:267-269`) | unchanged (`:268-270`) |
| `deps.Platform` 55 lines / 29 files | `grep -rn -E '\bdeps\.Platform\b\|Dependencies\.Platform\b' --include='*.go' \| grep -v _test.go`: whole tree 62 lines (comments included); **port set only**: 27 lines in 6 packages — `processor/rule` 15, `processor/graph-ingest` 6, `graph/inference` 3, `pkg/types` 1 (doc), `graph` 1 (doc), `service` 1. `config` reads its own `.Platform.Org/.ID` (9 lines), never `deps.Platform` |
| `ValidateEntityIDAuthority` 3 callers (`authority_gate.go:52`, `actions.go:597`, `hierarchy.go:217`) | 3 callers, `actions.go` now `:633` |
| `graph.NewAlertEvent` 0 production callers | unchanged |
| `docs/concepts/16-federation.md:55-58` "nothing checks uniqueness" cited as fact (S1(c)) | the doc still says it (`:56-58`) but ADR-104 d1 makes it false at the pin — a stale sentence in a document ruled for port |
| `graph/llm`, composers, rule run mint silent (B1–B3) | **missed at both heads**: `graph/llm/prompts.go:22-23` teaches `platform` = "Platform/product within organization" (#43); `graph/clustering/summarizer.go:728` hand-builds `org.platform` (#44); `FrameworkIdentityFamily.EntityID` (#45) and `ruleTriggerEntityID` (#46) are the composers; `processor/rule/actions.go:1855-1859, :1989-1990, :2009` carry the pair into `agentrun.Mint` (#42) |
| vocabulary silent | **missed at both heads**: `vocabulary.EntityIRI(…, pcfg platform.Config, …)` (#34) carries `platform.ID` + `Region` and drops `Org`; `vocabulary/export/export.go:130` (#31) is a second IRI grammar for the same entity, from the ID |

## 4. Searches run anew, over the pin snapshot

- **NATS subject / stream / bucket grammars embedding the pair**, port-set production Go:
  `grep -n -E '(Sprintf|Join)\([^)]*(\.Org|\.Platform\b|org,|platform,)'` filtered to subject/stream/bucket → only
  `config/bucket_name.go:47` (#13); `grep -n -E
  'semstreams_config|BucketName\(|(Subject|Stream|Bucket)[A-Za-z]*\([^)]*\b(org|platform|Org|Platform)\b'`
  → `config/bucket_name.go`, `config/manager.go:123, :1131`, `graph/constants.go:62, :74`, `graph/kvcatalog.go:111`,
  `config/config.go:234` (org validated as a subject part). **No NATS subject or stream name at the pin embeds the
  pair**; the only derived name is the configuration bucket (#13, #14). Entity IDs themselves are subjects
  (`graph.mutation.*`, `*.*.chain.agent.execution.*`), which is #26, not a second grammar.
- **`provenance`**: `grep -n -i provenance` over port-set production Go, filtered to lines also naming
  org/platform/authority/deployment → **0**. Files named provenance at the pin:
  `docs/adr/057-cryptographic-provenance.md`,
  `test/e2e/scenarios/authority_provenance_test.go` (outside). The pin's own split stands: product provenance is
  `Triple.Source` (`message/triple.go:58` — `` Source string `json:"source"` ``) and the envelope `source`
  (`base_message.go:238`), never positions 1-2 (`entity-id-contract/spec.md:404-405`).
- **`vocabulary`** (`vocabulary`, `/bfo`, `/cco`, `/export`, `/agentic`): `grep -n -E
  '\b(Org|Platform|platform\.Config|pcfg|authority|Authority)\b'`
  → `vocabulary/iris.go:85-110` (#34), `vocabulary/export/export.go:130` (#31); the rest are
  `PredicateAuthority` (namespace delegation, `namespace_authority.go:61-73`) and "delegated authority" predicate
  descriptions (`vocabulary/agentic/register.go:246-258`) — a different fact.
- **Wire fields**: `grep -rn 'json:"\(org\|platform\|platform_id\|org_id\)[",]' --include='*.go' | grep -v _test` → #1,
  #3, #4, #11 (the three in-set declarations) and #35-#36 (outside); nothing else in the port set.
- **SemEngine `main` (`2ec3bcf`)**: `git grep -n -E 'platform\.(org|id|Config)|PlatformMeta|[Ff]ederation|deployment
  authority' -- '*.go' 'openspec/specs' 'docs/adr' 'docs/*.md'`
  → `docs/inventory-scope.md:16, :23` only; no ADR directory; `openspec/specs` holds the five harness specs. The
  fact has **no spelling on `main`**.
- **Functions taking the pair / composing under it** (added in round 2): `grep -rn -E 'func .*\(.*\borg,
  platform\b|IdentityFamily\(\)\.EntityID\(' --include='*.go' | grep -v _test.go`
  → in the port set only #45, #28, #46 (`:22`, `:33`), `graph/events.go:196`, `config/config.go:865`; the remaining
  15 definitions are `agentic/entity_ids.go:19, :29, :71, :88, :128, :144`, `agentic/agent_lesson_entity.go:444, :454,
  :482`,
  `agentic/ops_diagnosis_entity.go:204, :214`, `agentic/web_observation_entity.go:218`,
  `agentic/agentrun/agentrun.go:395`,
  `processor/agentic-loop/graph_writer.go:526`, `processor/agentic-dispatch/loop_wire.go:75` — all outside.
- **Gate wiring** (for #41): `grep -n 'authorizeSubject(' processor/graph-ingest/*.go | grep -v _test` → the
  definition and ten call sites (`canonical_mutations.go:244, :307, :383, :474`; `component.go:1743, :2065, :2224,
  :2306, :2449, :2591`); only `:1743` and `:2065` pass `importLane`, the rest pass `false`.
- **Open pull requests overlapping** (`gh pr list --state open`): #48 only (draft). No other claim touches `message`,
  `pkg/platform`, `pkg/types`, `vocabulary` or `config`.

## 5. Open evidence questions (not findings; for the reviewer and the owner)

- **Q1 — the extractor is a consumer obligation.** At the pin the effective pair exists only after
  `config.Manager.Start` (#12) and reaches components because `internal/boot/run.go:198-199` extracts from
  `effectiveConfig` (#18). Neither `internal/boot` nor `internal/bootstrapobservability` is in the port set, so no
  SemEngine package will hold the "extract after Start" ordering; `processor/graph-ingest/component.go:721` and
  `processor/rule/factory.go:124` refuse an empty pair but cannot tell a stem from an effective id. Whether SemSource's
  composition root (`cmd/semsource/run.go:327` at the pin inventory's head) orders it the same way was not read (brief:
  no consumer repository).
- **Q2 — #48 at `c64ac338` versus the ruling.** The `FederationMeta` family (#32), its doc lines (#33), and the
  `pkg/platform` row's `consumer_purpose` ("seven `message` symbols and `vocabulary.EntityIRI`") are as at the pin;
  ADR-102, ADR-104, `16-federation.md` and `entity-id-contract` are absent from the worktree. After the ruled removal,
  the only public signature in change 1 naming `platform.Config` is `vocabulary.EntityIRI` (#34), which has zero
  production callers at the pin and is not in any ruling.
- **Q3 — not re-read:** semsource's `entityid.MaxOrgLen = 64` (pin inventory Fact B′) and `PlatformMeta{` at
  `cmd/semsource/run.go:327`; semconnect's ADR-102/104 use (scope table names it, brief excludes it).
- **Q4 — two zero-caller surfaces in the port set that take the pair as parameters:** `graph.NewAlertEvent` (#25,
  ch.2) and `config.MinimalConfig` (#4, ch.3); plus `EntityID.DeploymentPrefix` (#27, in #48). Each is a surface-audit
  question for its change under the #9 ruling (ported whole; dead surface removed), not decided here.
- **Q5 — five passages ruled or placed for port carry text the pin itself supersedes:** ADR-104 d7 (#38, `:94-100`)
  against `component-runtime-config/spec.md:369-371`; `16-federation.md:56-58` (#41) against ADR-104 d1;
  `16-federation.md:60-64` (#41, "lexical contract only … accepted as local truth") against the wired gate (§4);
  `pkg/platform/platform.go:27-28` ("embedded message metadata") against the ruled removal of #32; and
  `graph/llm/prompts.go:22-23` (#43, `platform` = "Platform/product") against `entity-id-contract/spec.md:401-402` —
  the last one is code carried dormant into ch.2, where a model is told the superseded meaning until ch.7 deletes it.
- **Q6 — the ch.7 seam and #44.** `graph/clustering/summarizer.go:690-803` is the only port-set reader of
  `llm.EntityParts`/`OrgPlatform`; cutting `graph/llm` at the seam either removes this summary input or re-homes the
  three `graph/llm` declarations (#43) — a placement the ch.7 design has to state, not decided here.

Inventory ends here; stop for independent inventory review.
