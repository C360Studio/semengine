# Proposal: setup-04a-03-boot-service-manager

## Why

Change 3 (`setup-04a-03-boot`) of epic #9 ports the boot-layer packages one per pull request (#159). `service` is split
by lifecycle owner into seven pull requests (owner, 2026-10-20, #9 comment 6296023694, option B; order in
`openspec/changes/archive/2026-10-20-setup-04a-03-boot-service-base/inventory.md` § 2.3). PR 1, `BaseService` and the
shared files, merged as `3453d9b` (#343). This change is PR 2: the service manager (`Manager`), which composes,
starts and stops the services and serves the process's operator routes, and the metrics service (`Metrics`) whose
server it drives. Two lifecycle owners in one pull request under the owner's waiver, 2,960 non-test pin lines at
SemStreams `8b99efe9`.

## What Changes

- Port `service_manager.go`, `service_manager_config.go`, `startup_metrics.go`, `graph_triples_http.go`,
  `middleware.go`, `dependencies.go`, `registry.go`, `register.go` and `metrics.go` with their tests. `RegisterAll`
  registers the metrics service; the component manager's constructor follows in PR 3.
- `StartAll`'s context bounds `StartAll` and `StopAll(ctx)` is the only stop (ADR-113): ending that context stops no
  service, loop or listener, request contexts no longer derive from it, and a `StopAll` given an already-ended
  context stops nothing. When `StopAll`'s time runs out it still cuts the open connections, as SemStreams does, and a
  later `StopAll` waits for any handler still finishing (owner question Q-B). A service whose `Stop` returns
  `errs.ErrAlreadyStopped` counts as stopped (the SS#1218 proof the owner accepted).
- The four concrete `*ComponentManager` checks become one unexported interface the component manager (PR 3)
  satisfies; `StorageObservabilityServiceName` moves beside the route prefixes.
- Unread surface is dropped: seven `Manager` lookups, `StopHealthListener`, the `Manager`'s embedded `BaseService`
  with its `Start` and `Stop`, two `Registry` listings, `Metrics.Port`, `Path`, `URL` and `ConfigSchema`, and
  `Dependencies.ToolRegistry` and `LifecycleManager`.
- A service payload key written in another letter case is refused, through the check `config` uses, moved to
  `internal/keycase`. The health publisher counts a failed publish. If the owner agrees (Q-A), `/graph/triples`
  validates `subject` and `predicate`, sorts before its limit and fails a read error. `/openapi.json` names a slice
  apart from its element (`Closes #349`).
- Carry pin ADR-030 (HTTP middleware) and ADR-094 (boot-only composition) into `docs/adr/` with port notes.

## Capabilities

- Modified: `service-lifecycle` (`specs/service-lifecycle/spec.md`): the manager's `StartAll`/`StopAll`, the
  mandatory component manager, the health publisher, the metrics service and service configuration keys.
- New: `operator-surface` (`specs/operator-surface/spec.md`): what the manager serves before and after boot commits,
  `/readyz`, `/health`, `/healthz`, `/services`, `/services/health`, `/openapi.json`, `/docs`, `/graph/triples`, the
  middleware seam and the dedicated health listener.

## Impact

- `service/` grows by the manager and the metrics service; new `internal/keycase`; `config` calls the moved check.
  `docs/contract.md`'s operator table gains `/graph/triples`.
- semsource, semteams and semboids compose the manager as they do at the pin. Until PR 3 a composition needs a
  `component-manager` service of its own, because the manager refuses to boot without one. A `StopAll` given an
  already-ended context now stops nothing; each consumer already passes a fresh bounded one, except semboids on
  `beta.160`, whose `StopAll(duration)` predates `StopAll(ctx)`.
