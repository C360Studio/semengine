# Proposal: setup-04a-03-boot-service-manager

## Why

Change 3 (`setup-04a-03-boot`) of epic #9 ports the boot-layer packages one per pull request (#159). `service` is split
by lifecycle owner into seven pull requests (owner, 2026-10-20, #9 comment 6296023694, option B; order in
`openspec/changes/archive/2026-10-20-setup-04a-03-boot-service-base/inventory.md` § 2.3). PR 1, `BaseService` and the
shared files, merged as `3453d9b` (#343). This change is PR 2: the service manager (`Manager`) and the metrics service
(`Metrics`) it drives, two lifecycle owners in one pull request under the owner's waiver (2,960 non-test pin lines at
SemStreams `8b99efe9`).

## What Changes

- Port `service_manager.go`, `service_manager_config.go`, `startup_metrics.go`, `graph_triples_http.go`,
  `middleware.go`, `dependencies.go`, `registry.go`, `register.go` and `metrics.go` with their tests, under ADR-113.
- The four concrete `*ComponentManager` checks in the manager become one small unexported interface the component
  manager (PR 3) satisfies; `StorageObservabilityServiceName` moves to a shared file.
- The design settles the rest (claim stub; the design replaces this section).

## Capabilities

- To be named by the design (`service-lifecycle` modified, and the operator surface the manager serves).

## Impact

- `service/` grows by the manager and the metrics service; `register.go`'s `RegisterAll` registers both.
