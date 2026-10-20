# Proposal: setup-04a-03-boot-service-base

## Why

Change 3 (`setup-04a-03-boot`) of epic #9 ports the boot-layer packages one per pull request (#159). The owner set the
order on 2026-10-19 (#9 comment 6270875286): `health`, `internal/logforwarderpolicy`, `component/flowgraph`, `config`,
`pkg/rulepack`, `composition`, then `service`. `service` is about 11,800 pin lines at SemStreams `8b99efe9`. The owner
accepted its split by lifecycle owner into seven pull requests on 2026-10-20 (#9 comment 6296023694, option B; the
order is `inventory.md` § 2.3). This change is PR 1: `BaseService`, which every service embeds, and the files the
later owners share (1,475 pin lines, under the ~3,000-line cap).

## What Changes

- Port `base.go`, `pprof.go`, `graph_runtime.go`, `service_configs.go`, `schema.go`, `openapi.go`, `openapi_types.go`,
  `openapi_registry.go` and `configurable.go` into `service`, with a rewritten `doc.go` and `README.md` covering only
  this surface. Dispositions per file are in the design (D1).
- `BaseService` follows ADR-113: ending Start's context stops nothing, and `Stop(ctx)` is the only stop (D2).
- `MaybeStartPProf` returns a server that `Shutdown(ctx)` stops and waits for, as the owner ruled (D3). This changes
  the call in semsource, semteams and semboids.
- Unread surface is dropped: three options, `OnHealthChange`, `RegisterMetrics`, three OpenAPI helpers,
  `GetOpenAPISpec`, `Configurable`, the second `ErrAlreadyStopped`, and `NewBaseServiceWithOptions`' unread
  configuration argument (Q1).
- Carry pin ADR-058 into `docs/adr/` with a port note; add the `service` and ADR-058 ledger rows, a
  `docs/repository-map.md` row and `service` in `scripts/cover-check.sh`.

## Capabilities

- New: `service-lifecycle` (`specs/service-lifecycle/spec.md`): `BaseService` Start/Stop under ADR-113 and its
  health check, the profiling server's Shutdown, and `WireGraphRuntime`'s retention backstop.

## Impact

- New package `service` (PR 1 of 7). semsource, semteams and semboids change their `MaybeStartPProf` call; semteams
  and semboids drop one `nil` from `NewBaseServiceWithOptions` if Q1 is yes.
