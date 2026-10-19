# Inventory: port-logforwarderpolicy

base: `9ad64071e42ffeee734c6b25bcd1f37baf9873c2` (origin/main `e45977b` plus the claim). Pin paths at SemStreams
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, read from the pin tarball
(`gh api repos/C360Studio/semstreams/tarball/8b99efe9`).

Status: INVENTORY PASS (semengine-reviewer, 5fe3231).

## Admission

- `openspec/changes/archive/2026-10-01-setup-03b-contract-boundary/design.md:229`: `health`,
  `internal/logforwarderpolicy`, `internal/componentadmission` under `service`, disposition carry.
- `openspec/changes/archive/2026-10-01-setup-04a-foundation/design.md:165`: change 3 lists `component/flowgraph
  composition config health internal/logforwarderpolicy pkg/rulepack service`.
- Order: #9 comment 6270875286 (second of seven). No ledger row exists yet
  (`git grep -n logforwarderpolicy docs/admission-ledger.yaml` is empty).

## Pin probe

go1.26.4 darwin/arm64, package only; the lifecycle suite does not apply (no owner).

- `go test -race -count=1 -p 1 -cpu 1 ./internal/logforwarderpolicy`: ok.
- `go test -count=5 -cpu 1 -p 1 -shuffle=on`: ok (seed 1792426789567786000). Coverage 90.0%.
- Test-text rules (`time.Sleep`, Skip, build tags, `t.Parallel`): grep empty. Pinned revive and gofmt: no findings.
- Uncovered blocks: `policy.go:38-42` (second JSON value), `policy.go:77-79` (empty source dropped).
- Behaviour probe (a throwaway test in the pin copy, removed): `""`, whitespace, `null` and nil give INFO and
  `["flow-service.websocket"]`; `{} {}` gives `multiple JSON values`; `" warn "` gives WARN;
  `["", " a ", "a", "flow-service.websocket.x"]` gives `["flow-service.websocket","a","flow-service.websocket.x"]`;
  `INFO+2` is refused; `ValidateFields("warn", nil)` is refused while `Resolve` accepts `warn`.

## Shape sweep (pin `internal/logforwarderpolicy`)

1. Second spelling of the four-level vocabulary: `policy.go:65` (`case "DEBUG", "INFO", "WARN", "ERROR":`) and
   `policy.go:94` (`func parseLevel(level string) slog.Level {`, whose default maps anything to INFO, relying on
   `ValidateFields` having run). The pin also has `internal/bootstrapobservability/bootstrap.go:284` (not admitted).
   Optional.
2. Second spelling across packages of the strict single-value JSON decode: `policy.go:32-43`; in this tree
   `types/service.go:23-34`, `component/port_codec.go:214-226`, `processor/graph-ingest/component.go:597-609`.
   Optional.
3. Copied shell: none (no Start/Stop/watch). Layered interface: none.
4. Exported surface: `Policy`, `Resolve` and `ValidateFields` each have a pin production reader:
   `service/log_forwarder.go:17` (`logforwarderpolicy.Resolve(rawConfig)`), `:58`
   (`logforwarderpolicy.ValidateFields(c.MinLevel, c.ExcludeSources)`), `:72` (`logforwarderpolicy.Resolve(nil)`);
   `internal/bootstrapobservability/bootstrap.go:260` (not admitted). Notes, all optional:
   - `policy.go:63`, `func ValidateFields(minLevel string, _ []string) error {`: the second parameter has no reader.
   - `LogForwarderConfig.Validate` (`service/log_forwarder.go:57`) has no pin production caller (tests only), so
     `ValidateFields` is reached in production only through a method nothing calls; `service`'s surface audit decides
     it.
   - The doc comment `policy.go:16` ("consumed by boot composition") names a consumer, `internal/bootstrapobservability`,
     that is not ported (`openspec/changes/archive/2026-10-06-authority-one-spelling/inventory.md:225`).
   - Dead behaviour: `policy.go:14`, `const mandatoryWebSocketExclusion = "flow-service.websocket"`, names a source of
     a service the pin retired (`service/register_test.go:42-43` asserts `flow-service` is gone; archive
     `2026-08-27-flow-authoring-retirement`); no pin production code logs that source. The pin's own design makes
     recursion prevention structural, not by exclusion (`docs/proposals/gh955-bootstrap-logger-design.md:419`, R12).
5. Classification by error text: none in production (`policy_test.go` matches messages; test side only).

## Who reads it (#190)

- Pin importers: `service/log_forwarder.go:9` and `internal/bootstrapobservability/bootstrap.go:17`. Nothing else.
- In this tree: no importer until `service`'s log-forwarder PR, last in the order.
- The policy's behaviour readers at the pin are `pkg/logging.NATSLogHandler` (`isExcluded`, `nats_handler.go:134`),
  built only at `internal/bootstrapobservability/bootstrap.go:266`. Neither is ported: `docs/tier1-cross-check.md:99`
  (`pkg/logging` "not ported: no tier-0 package imports them"). `service.LogForwarder` only logs the values at Start
  (`service/log_forwarder.go:104-106`) and returns them from `Config()` (`:122`). So, ported as the set stands, the
  `log-forwarder` service would accept and validate settings that forward nothing (design, owner question Q1).

## Pin ADRs (#157)

All 85 records in the pin's `docs/adr` were searched (`logforwarderpolicy`, `log-forwarder`, `logforwarder`,
`exclude_sources`, `min_level`, `NATSLogHandler`, `flow-service.websocket`, `ValidateFields`): one hit.

- ADR-058 (boot lifecycle phases) names `LogForwarder` only in a list of service examples (`058-boot-lifecycle-phases.md:56`).
  It governs `service`, not this package.
- Read by title: ADR-094 (boot-only composition) seals service config at boot; ADR-098 (logs and metrics are
  out-of-band substrate observability) governs `pkg/logging`'s role.
- None binds this package: its code implements none of these decisions. ADR-058 and ADR-094 come with `service` and
  `composition` (#9 comment 6099868722). No pin guidance (contract sections or skills) names the package.

## Open issues (#160)

One `gh issue list --state open --search` per word: `logforwarderpolicy` gives #9 (the epic; stays, `Addresses #9`).
`logforwarder`, `log-forwarder`, `"log forwarder"`, `LogForwarder`, `exclude_sources`, `min_level` give nothing.
`bootstrapobservability` gives #74, and `flow-service` gives #17 and #7; each names the word, not the package.

## Overlapping PRs

`gh pr list --state open`: #318 (health, draft) touches only `openspec/changes/setup-04a-03-boot-health/` today. Its
task 3.1 appends a row to `docs/admission-ledger.yaml`, as this PR will: an append-only text overlap.

## Problem shape

Strict decode with defaults, normalize, then validate. The closest instance in the tree is `types/service.go:23-34`
(same shape and message text). Not a new primitive, so no adoption sweep; the collision table is not triggered; no
boundary moves.

## Adopter seam

The package is `internal/`: no consumer can import it. The outside seam is the `log-forwarder` service config
(`min_level`, `exclude_sources`), reachable only once `service` lands; that PR inventories it.
