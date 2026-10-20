# Inventory: port-rulepack

base: `e626b6f7e8bb8378852d7a2adbdc368059bc9902` (origin/main `69436f0` plus the claim). Pin paths at SemStreams
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, read from the pin tarball
(`gh api repos/C360Studio/semstreams/tarball/8b99efe9`).

## Admission

- `openspec/changes/archive/2026-10-01-setup-03b-contract-boundary/design.md:212`: `pkg/rulepack` (143), readers
  `processor/rule/config.go:14`, `service/rule_pack_bind.go:8`, `frameworkcapabilities/rulepacks/validate.go`,
  "Admit (carry; E5)". E5 (`:539`): "`pkg/rulepack` carries (no edit)". Matrix row `:716`: qualifying consumer
  semboids, "`pkg/rulepack` 0 imports".
- `openspec/changes/archive/2026-10-01-setup-04a-foundation/design.md:165` (change 3); its `inventory.md:756-759`:
  not a leaf. Order: #9 comment 6270875286 (fifth of seven). No ledger row (`git grep -n rulepack
  docs/admission-ledger.yaml` is empty). 143 + 40 test lines: under the #159 bound.
- Not admitted: `internal/boot` ("outside the 65", `.../setup-03b-contract-boundary/inventory-2-scope.md:125`; no
  consumer uses it, `.../setup-04a-foundation/inventory.md:527`); `internal/bootstrapobservability` and
  `frameworkcapabilities/*` are in no admitted list of `docs/tier1-cross-check.md` and no ledger row (`git grep -n
  frameworkcapabilities` hits only reader citations in the 03B and 04A archives). Neither is a consumer adapter:
  `frameworkcapabilities/rulepacks` validates rule-processor composition (the rules of the durable-execution half),
  and `internal/boot` is the pin binary's own composition root.

## Pin probe

go1.26.4 darwin/arm64, package only; the lifecycle suite does not apply (no owner).

- `go test -race -count=1 -p 1 -cpu 1 ./pkg/rulepack`: ok. `-count=5 -cpu 1 -p 1 -shuffle=on`: ok (seed
  1792497532881129000). Coverage 19.2%: `ValidateID` 100%, `ValidateConfig`, `ValidateRuntimeUpdate`, `decodePackID`
  0%. With `-coverpkg=./pkg/rulepack` over `frameworkcapabilities/rulepacks`, `processor/rule`, `service`: 90.4%
  (86.5% from `frameworkcapabilities/rulepacks` alone): the two functions are tested only from an unadmitted package.
- Test-text rules: `identity_test.go:9` — `t.Parallel()`; no `time.Sleep`, Skip or build tag. gofmt and the tree's
  pinned revive (`go tool revive -config revive.toml`): no findings (a two-line sensitivity file gave 2 warnings).
- Behaviour probe (throwaway test in the pin copy, removed): `pack:one`, `pack.one`, `pack*`, `pack>`, `pack one`,
  `päck` are refused by the KV-token check (message "one literal KV token"); only `pack/one` reaches the loop
  ("offending char \"/\""); `a=b` accepted; 247 and 513 bytes refused by the length check. `ValidateConfig`: duplicate
  enabled IDs refused, a disabled instance with `bad.id` refused, unknown config keys accepted, nil accepted.
  `ValidateRuntimeUpdate`: unchanged nil; newly enabled refused; pack change refused; other config change refused;
  disabling accepted.

## Shape sweep (pin `pkg/rulepack`)

1. Second spelling of the PackID alphabet: `identity.go:18` — `PackIDCharset = "[A-Za-z0-9_=-]"` (read for message
   text and, at `processor/rule/config.go:151-152` and `schema.go:18`, the operator-schema pattern) and
   `identity.go:45` — `for _, char := range packID {` (`:45-55`), which never reads the constant. The KV alphabet is a
   third, per byte: `natsclient/kv_key_contract.go:262` (pin; tree `:257`) — `case '-', '/', '_', '=':`. Since
   `ValidateKVLiteralToken` runs first (`:39`), the loop's only reachable refusal is `/`; no test ties the constant
   to the loop.
2. Second spelling of the duplicate-pack refusal: `identity.go:78` — `if first, duplicate := seen[packID]; duplicate
   {` and `service/rule_pack_bind.go:99` — `if _, duplicate := seen[packID]; duplicate {` (same message stem, `:100`).
   The `service` copy runs after the projection preflight (`:89-93`). `:78-81` names the two instances in map order.
3. Copied shell: none (no Start, Stop or watch). Layered interface: none.
4. Exported surface: `PackIDCharset`, `MaxPackIDBytes` and `ValidateID` have admitted pin readers (below).
   - `identity.go:62` — `func ValidateConfig(cfg *config.Config) error {`: only reader
     `frameworkcapabilities/rulepacks/validate.go:18`, reached from `internal/boot/run.go:90` and
     `internal/bootstrapobservability/bootstrap.go:209`, none admitted.
   - `identity.go:91` — `func ValidateRuntimeUpdate(instanceName string, previous *types.ComponentConfig, ...`: no
     production reader at the pin; its wrapper `validate.go:44-45` is called only by `validate_test.go:69,97,107,160`.
     It guards component hot reload, which ADR-094 retired (`094-...md:42`, "runtime reconcile, restart, and
     replacement paths are retired"); no pin production code subscribes to `components.*` (`OnChange(` has one
     production caller, `config/manager.go:258`, for `model_registry`).
   - Text no code implements: `identity.go:19-20` (`MaxPackIDBytes preserves the 256-byte owner-key budget after
     the "rule-pack." prefix`) and the messages at `:35`, `:41`, `:51` name an owner id `rule-pack.<pack_id>` that no
     pin code builds (the literal `rule-pack.` in pin Go code: comments and these messages only); ownership left with
     ADR-091, and `rule-projection-mutations/spec.md:60` forbids registering pack ownership. Package comment `:1-4`.
   - Test labelling: `identity_test.go:21` (`invalid alphabet`, `pack:one`) is refused by the KV check, not the loop.
5. Classification by error text: none in production (the test matches substrings).

## Who reads it (#190)

- `PackIDCharset`: `processor/rule/config.go:151`; `MaxPackIDBytes`: `:153` (both feed `schema.go:13,18`).
- `ValidateID`: `processor/rule/config.go:177` (via `validatePackID`, called at `config.go:162`,
  `graph_event_identity.go:23`, `rule_factory.go:199`, `expression_factory.go:87`, `test_rule_factory.go:30`) and
  `service/rule_pack_bind.go:96`. Both admitted (03B E5), neither ported yet.
- `ValidateConfig`, `ValidateRuntimeUpdate`: as in sweep item 4. In this tree nothing imports the package (`git grep
  -n -i 'rulepack' -- '*.go'`: one comment in `payloadregistry/registry.go:12`, the word only).
- Consumers: `gh search code` for `"semstreams/pkg/rulepack"` and `"frameworkcapabilities/rulepacks"` in semboids,
  semteams and semsource: 0 each (semconnect `pkg/rulepack` 0; its second search hit the rate limit).

## Pin ADRs and specs (#157)

All 85 records searched (`rulepack`, `pack_id`, `PackID`, `rule-pack.`, `ValidateRuntimeUpdate`): ADR-056, ADR-076,
ADR-094 hit; ADR-058 names `ConfigureRulePackMutations`; ADR-091 read by title.

- ADR-076 decision 4 (`076-...md:33-37`): PackID required, static, 1–246 ASCII bytes of `[A-Za-z0-9_=-]+`, one
  literal KV token, duplicates rejected before binding. Binds `ValidateID` and the constants.
- ADR-094 (`:35-43`): boot seals component composition; post-boot `components.*` changes act at the next boot.
- ADR-056 (`:65`, `:1742`, `:1748`): the `rule-pack.<pack-id>` owner id; superseded by ADR-091 (`056-...md:5`).
- ADR-058: the `service` boot phases (ADR-091 amendment, `:4-6`); governs `service`, not this package.
- Specs: `graph-events/spec.md:65-66` (grammar, bound, no normalization) and `:75-77` (duplicates fail composition
  before projection validation); `rule-projection-mutations/spec.md:12` (preflight validates IDs and duplicates).
  Neither capability is in this tree (`ls openspec/specs`). No pin skill or contract section names the package.

## Open issues (#160)

`gh issue list --state open --search rulepack` gives #9 (epic; stays, `Addresses #9`). `ValidateRuntimeUpdate`,
`frameworkcapabilities`, `PackIDCharset`, `pack_id`, `rule_pack`, `rulepackcontract`: nothing. The query `rule-pack`
gives #25, #26, #27 and `"rule processor"` gives #196: each names the word or `processor/rule`, not this package.

## Overlapping PRs

`gh pr list --state open`: #329 (draft) changes 13 files under `config/` (none is `config.go`) and its own
`openspec/changes/` directory; #332 (draft) has no files yet. No file overlap today; ledger and map appends at most.

## Problem shape

Admit-or-refuse at a seam, composing an existing validator: closest instance (tree)
`natsclient/kv_key_contract.go:94` — `func ValidateKVLiteralToken(token string) error {`, which `ValidateID` calls.
Not a new primitive: no adoption sweep; no collision table; no boundary moves (no intent check).

## Adopter seam

Public `pkg/`, but no consumer imports it (above). Operators reach `pack_id` through rule-processor config, which
`processor/rule`'s PR inventories; there an invalid value fails construction (`rule_factory.go:199`), a boot error.
