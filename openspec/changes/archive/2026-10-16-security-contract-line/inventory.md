# Inventory: security-contract-line

base: `28454d4b5ecbcc0de68d5aa5740862cfdda1c8c0` (origin/main `e593a28e56bf49eb847ae74f9cc20c62c0d11775` plus the
empty claim commit of draft PR #279; every line number below is the same on both)

Question: what does `pkg/security` export, who reads each export (this tree, the SemStreams pin `8b99efe9`, and the
four consumers `docs/inventory-scope.md` names), which of #190's three kinds each export is, and what a product that
reaches it through `component.Dependencies.Security` must know (#274). Repositories read: this tree; the pin, from a
snapshot fetched with `gh api repos/C360Studio/semstreams/tarball/8b99efe9c66a4faa4fa509f9f62cc6bad8392128` and
unpacked outside this repository (local only: `/tmp/security-contract-line-pin/src`); semsource, semconnect, semteams
and semboids, read with `grep -rn` over their local checkouts (no `git` command). No other repository is read.

This change carries no package from the pin: `pkg/security` is already ported (ledger row
`docs/admission-ledger.yaml:390`, disposition `adapt`). No pin probe is owed. Non-test Go lines today: 201
(`pkg/security/config.go` 70, `pkg/security/doc.go` 131).

## 1. The claimed gap

Issue #274: "no spec requirement and no `pkg/security` README section says what it offers a product".

- Spec requirement: none. `git grep -n -i 'security' -- openspec/specs` returns nothing. CONFIRMED.
- README: none. `ls pkg/security` lists `config.go`, `config_test.go`, `doc.go`. CONFIRMED.
- **Ledger `contract` field: present, and the issue does not name it.** `docs/admission-ledger.yaml:396-401` —
  `contract: >- the seven configuration types (Config, TLSConfig, ServerTLSConfig, ClientTLSConfig, ServerMTLSConfig,
  ClientMTLSConfig, ACMEConfig) and their JSON keys as at the pin, except ServerMTLSConfig.ClientCertOptional ...`.
  #190's body and the architect contract (`.agents/contracts/semengine-architect.md:89`, `:271`) count the ledger
  `contract` field as a contract line. The owner's 2026-10-16 words (#90 comment 6214503074; #223 comment
  6214503370) and #161's allowlist reason (`.agents/rules-index.md:76`: `contract: <spec requirement or README
  section>`) name only a spec requirement or a README section. So the gap is real for the 2026-10-16 bar, and false
  for the older one. What the ledger line does not say, and nothing says: that products reach the types through
  `component.Dependencies.Security`, and which fields any SemEngine code applies.
- Product-shaped test: none. `git grep -nw 'Security' -- '*.go' ':!pkg/security'` finds only
  `component/dependencies.go:51` — `Security        security.Config           // Platform-wide security configuration`
  and two `// Security:` comments (`component/registry.go:214`, `:226`). No production or test code reads or sets
  `Dependencies.Security`. Known positive for the search: `git grep -nw 'NATSClient' -- component/dependencies.go`
  finds `:47`. CONFIRMED.

## 2. Every spelling of the fact (the configuration, and what applies it)

- Declared: `pkg/security/config.go:5-70`, seven struct types, 31 fields, each with a `json` tag and a `schema` tag.
- Described: `pkg/security/doc.go:1-131`. `:65-78` — an "ACME-managed certificates" usage example; `:130` —
  `//   - [github.com/c360studio/semengine/pkg/acme]: ACME client implementation`. `pkg/acme` does not exist here
  (`ls pkg/acme`: no such directory). The owner ruled on #9 (comment 5994720412, item 5) that README sections and
  links for packages SemEngine does not have are removed; its exception, a planned import path named in a milestone,
  does not cover `pkg/acme` (no milestone title or description names it:
  `gh api 'repos/C360Studio/semengine/milestones?state=all'`). Further ACME text in `doc.go`: `:83` (`Mode:
  "manual" (provide certs) or "acme" (automatic)`), `:101-106` (the `ACMEConfig` section), `:120` (`Manual mode (no
  ACME by default)`); and `:129` points products at `internal/tlsutil`, which they cannot import.
- Carried to products: `component/dependencies.go:51` (above).
- Applied: `metric/handler.go:37` — `func NewServer(port int, path string, registry *MetricsRegistry, securityCfg
  security.Config) *Server {`; `:149-150` — `if s.security.TLS.Server.Enabled {` /
  `tlsConfig, err := tlsutil.LoadServerTLSConfig(s.security.TLS.Server)`; `:103` refuses server mTLS without server
  TLS. `internal/tlsutil/tlsutil.go:18` — `func LoadServerTLSConfig(cfg security.ServerTLSConfig) (*tls.Config,
  error) {`; `:54` — `func LoadClientTLSConfig(cfg security.ClientTLSConfig) (*tls.Config, error) {`; `:122-126` —
  the fail-closed client-certificate rule; `:158-166` — `parseTLSVersion` refuses a MinVersion other than `1.2`,
  `1.3` or empty.
- `LoadClientTLSConfig` has no production caller: `git grep -nw LoadClientTLSConfig -- '*.go' ':!*_test.go'` finds
  its declaration, its own error strings and `internal/tlsutil/doc.go` only.
- Second interpreter of the `schema` tags: `component/schema_tags.go:243-249` refuses a `type` outside
  `isValidType` (`:316-319`: `"string", "int", "bool", "float", ...`), and `:254-258` refuses a `category` other than
  `basic` or `advanced`; `GenerateConfigSchema` skips a field whose tag it refuses (`:404-409`, "Log error and skip
  field - graceful degradation"). Every one of the 31 `pkg/security` tags carries `category:security`, and the 7 bool
  fields carry `type:boolean`. Probe (a scratch module outside this repository, `replace` onto this worktree, Go
  1.26.9; local only: `/tmp/security-contract-line-probe/main.go`): `GenerateConfigSchema` returns 0 properties for
  `security.Config` (1 field), `ServerTLSConfig` (7), `ClientTLSConfig` (6), `ACMEConfig` (8) and
  `ServerMTLSConfig` (4); `ParseSchemaTag("type:boolean,...,category:security")` →
  `invalid type: boolean`; with `type:bool` → `invalid category: security (must be 'basic' or 'advanced')`. The pin
  has the same parser (`component/schema_tags.go:260`, `:320` at the pin) and no pin code generates a schema over a
  `security` type (`grep -rn 'GenerateConfigSchema(reflect.TypeOf(security'`: nothing). The tags are read by nothing
  that accepts them, here or at the pin. They also disagree with the pin's behaviour: `ACMEConfig.RenewBefore` says
  `default:24h` (`pkg/security/config.go:22`); the pin's ACME loader defaults to 8 hours and turns an unparsable
  value into 8 hours silently (pin `pkg/tlsutil/tlsutil.go:350-354`, `pkg/acme/client.go:96`).
- Stale text: `metric/tls_test.go:92` — `// plain HTTP to every client. security.Config.Validate does not catch it:`.
  `security.Config` has no methods (`config.go:5-7`); the floor port removed `Validate` (ledger `:404-405`).

## 3. Adjacent claims

Open issues, by `gh issue list --state open --search "<term>" --limit 200` for `security`, `tls`, `tlsutil`,
`acme`, `mtls` and `Dependencies` (hits read, not counted):

| Issue | Names `pkg/security`? | Disposition |
| --- | --- | --- |
| #274 | yes (title, body) | retired by this PR (`Closes #274`) |
| #84 epic, `status:needs-decision` | yes (body) | stays: the security capability (authn, authz, secret hygiene) is the owner's decision; this PR decides none of it |
| #9 epic | yes (rulings 5953295358, 5985900154, 5994720412) | stays: epic; its rulings are inputs |
| #223 epic | yes (comment 6214503370, the worked case) | stays: #274 is outside the wave; no `Addresses` |
| #161 | yes (body, as history) | stays: its check is not built; when built, its allowlist takes this PR's spec requirement as the `contract:` reason |

Names the word, not the package: #74 (`config`'s NATS TLS settings), #29, #38, #17, #153, #24, #114, #16, #15, #7.

Open pull requests (`gh pr list --state open --json number,title,changedFiles,files`; none has more than 100 files):

- #273 (draft, PR 7 of the wave, 54 files at `bbb0c447571362b16b6e33649687a521f5ce78c5`): changes
  `component/dependencies.go` (`Platform` field type, the line above `Security`), `component/registry.go`
  (`RegisterFactory` and `RegisterWithConfig` become `Register(Registration)`) and `component/discovery.go` (deletes
  `type Metadata`, and removes `Meta()` and `ConfigSchema()` from `Discoverable`); edits
  `docs/admission-ledger.yaml` (hunks at `:1872` and later); adds a `component-registration` spec delta. No
  capability overlap with a new `security-config` capability. Two real overlaps: a product type in this change's test
  must implement `Discoverable`, whose method set #273 changes, so whichever PR lands second edits
  `pkg/security/product_test.go`; and both edit the ledger, far apart (`:390`, `:675` here), so a textual conflict
  is unlikely.
- #278 (draft claim; 5 files, all under `openspec/changes/simplify-04a-12-dispatch/`, as of round 2), #279 (this
  claim, 0 files), Dependabot #242-#248 (`go.mod`, `go.sum`, `package*.json`, a workflow): no overlap.

Specs: no current spec names `pkg/security`, `security.Config`, `Dependencies.Security`, TLS or mTLS
(`git grep -n -i -e security -e tls -e mtls -- openspec/specs`: nothing). Active changes: none names them
(`git grep -n -i 'pkg/security' -- openspec/changes ':!openspec/changes/archive'`: nothing).

Ledger rows: `pkg/security` (`:390-410`); `internal/tlsutil` (`:675-698`; ACME loaders "deferred to change 5",
`:686-687`); `metric`.

Rulings: the ACME loaders and `pkg/acme` are cut from the floor and travel to change 5 (owner, #9 comment
5950752741; `openspec/changes/archive/2026-10-05-setup-04a-01-floor/design.md:71-74`, `:752-755`). `pkg/security` is
public because `metric.NewServer` names `security.Config` (#9 comment 5953295358; floor design `:559`), and is framework
API through `Dependencies.Security` (#90 comment 6214503074). `pkg/tlsutil` became `internal/tlsutil` because "no
consumer import; no public signature names its types" (floor design `:562`), a ruling made before #190 (2026-10-12).

## 4. Who it is for: every export (#190 kinds)

Readers here: `git grep -nw "security\.<Type>" -- '*.go' ':!pkg/security/'` per type, then production-only by hand.
Readers at the pin: `grep -rln 'semstreams/pkg/security"' --include='*.go'` (9 production files) and field reads
(`grep -nE 'security\.|\.Security|\.TLS\.|\.ACME|\.MTLS'` in each). Consumers: `grep -rnw --include='*.go' -e Security
-e ServerTLSConfig -e ClientTLSConfig -e ACMEConfig` and `grep -rn 'pkg/security"'` over each checkout: **0 hits in
all four**; known positive `grep -rn 'semstreams/component"'`: semsource 60, semconnect 6, semteams 140, semboids 22.

| Export | Kind | Production reader here | Production reader at the pin |
| --- | --- | --- | --- |
| `Config` (`config.go:5`), field `TLS` | framework API | `component/dependencies.go:51`; `metric/handler.go:30`, `:37` | `component/dependencies.go:74`, `config/config.go:50`, `service/component_manager.go:70`, `:1214`, `service/metrics.go:28`, `metric/handler.go:40`, `output/websocket/websocket.go:68`, `output/httppost/httppost.go:103`, `input/websocket/websocket_input.go:37` |
| `TLSConfig` (`:10`), field `Server` | framework API | `metric/handler.go:149-150` | `metric/handler.go:153`, `service/metrics.go:189`, `output/websocket/websocket.go:753` |
| `TLSConfig.Client` | framework API | none | `output/httppost/httppost.go:218-232`, `:305`; `input/websocket/websocket_input.go:1076-1105`; `config/config.go:333-349` |
| `ServerTLSConfig` (`:36`): `Enabled`, `CertFile`, `KeyFile`, `MinVersion`, `MTLS` | framework API | `internal/tlsutil/tlsutil.go:18-46` via `metric/handler.go:150` | pin `pkg/tlsutil/tlsutil.go:18`, `:78`; `config/config.go:305-325` |
| `ServerTLSConfig.Mode`, `.ACME` | framework API | none | `output/websocket/websocket.go:755-760`; `input/websocket/websocket_input.go:820`; pin `pkg/tlsutil/tlsutil.go:191-197` |
| `ServerMTLSConfig` (`:28`): `Enabled`, `ClientCAFiles`, `ClientCertOptional`, `AllowedClientCNs` | framework API | `internal/tlsutil/tlsutil.go:112-133` | pin `pkg/tlsutil/tlsutil.go:98`; `output/websocket/websocket.go:780` |
| `ClientTLSConfig` (`:59`): `CAFiles`, `InsecureSkipVerify`, `MinVersion`, `MTLS` | framework API | `internal/tlsutil/tlsutil.go:54-90`, which has no production caller | `output/httppost/httppost.go:218-232`; pin `pkg/tlsutil/tlsutil.go:38`, `:150` |
| `ClientTLSConfig.Mode`, `.ACME` | framework API | none | `output/httppost/httppost.go:222`, `:306`; pin `pkg/tlsutil/tlsutil.go:274` |
| `ClientMTLSConfig` (`:51`): `Enabled`, `CertFile`, `KeyFile` | framework API | `internal/tlsutil/tlsutil.go:84-85` (no production caller) | pin `pkg/tlsutil/tlsutil.go:150`; `output/httppost/httppost.go:232` |
| `ACMEConfig` (`:16`): `Enabled`, `DirectoryURL`, `Email`, `Domains`, `ChallengeType`, `RenewBefore`, `StoragePath`, `CABundle` | framework API | none | pin `pkg/tlsutil/tlsutil.go:350-364`; `input/websocket/websocket_input.go:1529-1543`; `output/httppost/httppost.go:722-736` |

Counts: 38 exported identifiers (7 types, 31 fields). Framework API 38; plumbing 0; back door 0; dead surface 0 (each
has a production reader at the pin in an admitted package, `config`, `service`, `output/websocket` or `pkg/tlsutil`,
or in a consumer adapter, `input/websocket` or `output/httppost`). Of the 31 fields, 11 are applied today through an
exported SemEngine call (`metric.NewServer`): `Config.TLS`, `TLSConfig.Server`, the five server fields above and the
four server mTLS fields. 8 are read only by `internal/tlsutil.LoadClientTLSConfig`, which no production code calls:
`TLSConfig.Client`, four client fields, three client mTLS fields. 12 are read by nothing here: both `Mode` fields,
both `ACME` fields, and the eight `ACMEConfig` fields.

No new export is proposed.

## 5. The problem shape

Shape: framework API offered to products through a `component.Dependencies` field, with a capability spec as its
contract line. Closest instances: `component.Dependencies.PayloadRegistry` (`component/dependencies.go:53`), judged
framework API in the archived
`openspec/changes/archive/2026-10-16-simplify-04a-10-types-message-vocab/design.md` ("`*payloadregistry.Registry`
reaches every product component through `component.Dependencies.PayloadRegistry`"); and `pkg/errs`, framework API
whose line is `openspec/specs/error-classification/spec.md`. This design adopts both: one capability spec, scenarios
the proving test names. A second shape, refuse-until-wired for an accepted-but-ignored setting, has its closest
instance in `internal/tlsutil/tlsutil.go:158-166` (unknown MinVersion refused) and #74's "refuse it at load, with an
error that names the key, until it is wired".

## 6. The pin's decision records

None. The change carries no package from the pin. Search of all 85 pin records for `pkg/security`, `security.Config`,
`ServerTLSConfig`, `ACMEConfig` and `tlsutil`: nothing. No title names TLS, certificates or security (the only
`auth` title hits are "authoritative"/"authority" records about graph state). No local record names them
(`git grep -n -i -e pkg/security -e tlsutil -- docs/adr`: nothing).

## Triggered checks

- Collision table: not triggered (no durable, communication or coordination primitive).
- Intent check: not triggered. The change moves no boundary; `pkg/security` stays public and #84 is undecided.
- Adoption sweep: not triggered (no new reusable primitive).
- Guidance at the pin: none names `pkg/security` (`grep -rln 'pkg/security' --include='*.md'` at the pin:
  `metric/README.md` and two archived inventories only).

## Adopter seam inventory

The adopter: a developer writing a product component whose factory receives `component.Dependencies` and wants the
platform's TLS settings.

1. **What must they know?**
   1. Who fills `deps.Security`. Nothing in SemEngine does today: no `config` or `service` package is ported, so the
      field holds whatever the host that built `Dependencies` put there. At the pin the boot config's `security` key
      fills it (pin `config/config.go:50`, `service/component_manager.go:171`, `:1214`).
   2. How to apply it. The only exported call that applies it is `metric.NewServer`. The loaders that turn it into a
      `*tls.Config` are under `internal/`, which a product cannot import, so a product rebuilds the fail-closed
      client-certificate rule (`internal/tlsutil/tlsutil.go:120-126`) and the MinVersion refusal itself.
   3. Which settings do anything. `mode`, the `acme` blocks and the whole `client` block are applied by no SemEngine
      call. With `metric.NewServer`, `mode: "acme"` plus cert files serves the files; `mode: "acme"` without them fails
      at `Start` with "load certificate".
   Three items, more than two: a design finding.
2. **What happens if they do nothing?** Their component ignores `deps.Security`; whatever it serves is outside
   SemEngine's reach. Through `metric.NewServer`, the zero value serves plain HTTP (TLS is opt-in, `doc.go:117`).
3. **Where do they find out?** Item 1: nowhere today. Item 2: a compile error (`use of internal package`) when they
   try. Item 3: nowhere; `doc.go:65-78` advertises the opposite.
4. **What should they have to know?** One fact: pass `deps.Security` (or one of its blocks) to SemEngine's loader. The
   gap is a public loader (item 2), a declared or refused inert setting (item 3), and boot filling the field (item 1,
   the `config`/`service` port's work, not this change's).
