# Inventory: design-rule-core-split-plan

base: 4ead6360978d008649e0eb5e869086dcac2b36f1. Pin copy: `gh api repos/C360Studio/semstreams/tarball/8b99efe9`.
Question: how to cut the `processor/rule` port into PRs. Repositories read: semengine, semstreams at the pin, and
semteams and semboids at symbol level (`docs/inventory-scope.md`).

## Size

`find processor/rule -name '*.go'` = 163 files; production 53 files 17,243 lines, tests 110 files 34,224 lines.
expression: 1,417 + 2,110. Per-file list: `wc -l` over the pin copy.

## Imports against main

`go list -f '{{.Imports}}' ./processor/rule ./processor/rule/expression` at the pin:

- Present on main: component config graph graph/readiness internal/graphmutation message metric natsclient
  pkg/errs pkg/lifecycle pkg/projection pkg/rulepack pkg/types types vocabulary.
- Moved: pkg/cache -> internal/cache (ledger :829); pkg/resource -> internal/resource (:357);
  internal/lifecyclecleanup -> pkg/lifecyclecleanup (:2021).
- Not carried: agentic (:1853), agentic/agentrun (:1884), vocabulary/agentic (:1929); governance (no row).
- Symbols missing on main: `cache.WithEvictionCallback` (`expression/regex_cache.go:21`), `readiness.EnsureBucket`
  and `readiness.ProducerNames` (`readiness.go:191`, `:224`), `readiness.BucketGraphStatus` (now
  `graph.BucketGraphStatus`, `graph/constants.go:70`).
- `component/dependencies.go` on main has no `LifecycleManager` (ledger component row :2475, #102 ruling D); the
  pin reads it at `factory.go:160-161`.
- Test-only imports not on main: payloadbuiltins, processor/agentic-tools, agentic/research; internal/semantictest ->
  internal/harness/semantictest (:317).

## Pin probe

`go test -count=1 -race ./processor/rule/...`: ok. `go test -count=5 -cpu=1 -shuffle=on ./processor/rule/...`: ok
(seeds 1795602496010215000, 1795602495775027000). Text: `time.Sleep` 53 lines in 12 files; `t.Skip` at
`status_test.go:352`; 22 files `//go:build integration`, none with `t.Parallel`; production `context.Background|TODO`:
0; `WithoutCancel`: `stateful_evaluator.go:247` (bounded persist). Not run: the lifecycle suite and the integration tests.

## Shape sweep (findings; dispositions in design.md)

- Global rule-factory map filled from `init`: `rule_factory.go:124-127`, `expression_factory.go:443-449`,
  `test_rule_factory.go:212-218`.
- Production test factory: `test_rule_factory.go` (219 lines; read by 11 test files).
- Second decode spelling: `rule_loader.go:39` `definitionToRuleMap`; `config_validation.go:449` `definitionFromMap`.
- Four executor constructors: `actions.go:803`, `:820`, `:843`, `:894`; two processor constructors `processor.go:250`, `:255`.
- Dead config: `config.go:27` `MessageCache` (written `message_handler.go:92`, never read); `rule_loader.go:154-165`
  (parsed, only logged); `Consumer` struct in `config.go` (0 readers).
- `GenericJSON` from known fields: `message_handler.go:384` (with `time.Time`); `actions.go:2230` (leaves with deny/approve).
- Decode failure acknowledged: `message_handler.go:67-71`, `processor.go:1141`.
- Not found: classification by error text, a context stored in a struct.

## Open issues (`gh issue list --state open --search rule|expression`)

- Name the package: #25 #26 #27 #28 #294 #702 #703 #714 (milestone), #196 (`processor.go:102`,
  `cron_scheduler.go:54`), #195, #350, #704, #24, #9.
- Use the word only: #730 #712 #141 #110 #109 #635 #155 #554 #614 #84 #284 #734 #535 #134 #665 #484 #60 #300 #683
  #576 #111.
- #29: closed 2026-11-25 (owner's word).

## Open PRs

`gh pr list --state open`: #752 only (this claim).

## ADRs at the pin naming the package or its job

031 041 046 048 053 055 056 039 040 045 028 047 049 054 060 093 094 091; read by title: 083 088 108 113 (SemEngine).
Already in `docs/adr`: 060 074 083 088 094 102 104 108 109 111 112 113.

## Consumers (symbol level)

semboids `componentregistry/register.go:34` `rule.Register`; packs `configs/rules/zone-steering/*.json` (publish x6,
lifecycle_transition x1, core NATS input `configs/flock.json:198-233`). semteams production:
`internal/runtimecatalog/components.go:36`, `cmd/semteams/main.go:142`, `cmd/semteams/rule_config_service.go:33-92`.
Dead keys set in consumer configs: none (`grep` over semboids/configs and semteams/configs).

## Same-class owners for the family registry (for PR 2's own inventory)

`component.Registry`, `payloadregistry.Registry`, the pin's rule-factory map (`rule_factory.go:124`),
`lifecycle.Manager` workflow registration, `vocabulary.RegisterPack`.

Intent check: no boundary moves. The port set is ruled (#8 Q15, D12).
