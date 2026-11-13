<!-- markdownlint-configure-file { "MD013": false } -->
# Inventory: fix-604-stop-always-stops

base: bfbfd6bcff61380609c8476ac7b60870d3ec8c90 (branch `gemini/fix-604-stop-timeout`, matching `origin/main` `f7ab6003` plus empty claim commit)

Question: how does SemEngine handle caller-supplied `ctx` expiry or cancellation before and during terminal teardown (`Stop`, `StopAll`, `Shutdown`, `Close`) across lifecycle guards, managers, services, components, transport helpers, and background-work primitives?

Repositories read per `docs/inventory-scope.md`: `semengine` at `bfbfd6bcff61380609c8476ac7b60870d3ec8c90`; `semsource` at `be3dd560611ee2157d160856c51814581402e533` (`v0.1.0-alpha.4` consumer of `service` and `component`); `semsparql` at `e7f827c445e0e11c990869f5ef70550a91880482` (imports `natsclient`, `vocabulary`, `graph`; does not import `service` or `component`); `semconnect` (`d0d06e00`), `semteams` (`4df44ae8`), and `semboids` (`ab4e0fbc`) remain on `semstreams` in `go.mod`. Carries no package from the SemStreams pin.

## 1. The claimed gap

Issue `#604` (`ComponentManager.stopComponents` skips remaining components when stop context expires) reports that when one child component exhausts the stop context budget during `ComponentManager.Stop(ctx)`, remaining started components in reverse start order are skipped without deregistering provided stores or initiating teardown:

- `service/component_manager_lifecycle.go:184-187` — `if err := ctx.Err(); err != nil { failures = append(failures, componentStopError(record.name, err)); continue }`
- `service/component_manager_lifecycle.go:190-191` — `cm.deregisterProvidedStores(record.name)` / `if err := lifecycle.Stop(ctx); err != nil {`
- `service/service_manager.go:709-715` — `for _, admitted := range started { ... if err := admitted.service.Stop(ctx); err != nil && !errors.Is(err, errs.ErrAlreadyStopped) {` (when `ComponentManager.Stop(ctx)` exhausts `ctx`, subsequent services in reverse `m.order` receive an already-ended `ctx`).
- `internal/lifecycleguard/guard.go:109-111` — `if err := ctx.Err(); err != nil { return err }` (every owner delegating `Stop` to `lifecycleguard.Guard` — `BaseService`, `LogForwarder`, `service.Manager`, `ComponentManager`, `Metrics`, `MessageLogger`, `HeartbeatService`, `MetricsForwarder`, `StorageObservabilityService`, `config.Manager`, and every component composing `component.Lifecycle` — returns `ctx.Err()` before running `cleanup(ctx)` when handed an already-ended `ctx`).

Searches run across the repository:

- `gopls workspace_symbol -matcher=fuzzy Stop`, `gopls implementation component/component.go:84:6` (`LifecycleComponent`), and `gopls implementation service/service.go:42:6` (`Service`): enumerated all `Stop`, `StopAll`, `Shutdown`, and `Close` implementations in the tree.
- `git grep -n "ctx\.Err()" -- '*.go'` and `git grep -n "stops nothing\|stop nothing"`: located every pre-flight `ctx.Err()` check and every spec/doc/test statement about ended stop contexts.

## 2. Every current spelling of terminal teardown under an ended or expiring `ctx`

Two incompatible spellings exist in the tree today, and Spelling A further divides on what an idle owner returns when handed an already-ended `ctx`:

### Spelling A — Teardown initiates unconditionally; `ctx` bounds only drain/join waiting

- Contract & ADR statements:
  - `docs/adr/113-start-context-bounds-start.md:54-55` — `Both make the same promise: stop, wait, return the context's error if the bound wins, and let a later call try again.`
  - `docs/adr/113-start-context-bounds-start.md:60-61` — `` `Stop(ctx)` is the only stop: nil only after everything is released; if `ctx` ends first it returns that error and a later `Stop` tries again. Inside `Start` the component creates the one run context for its continuing work, keeps only its cancel, and ends it inside `Stop`. ``
  - `openspec/specs/background-work/spec.md:53-55` — `` `Shutdown(ctx) error` SHALL be used for a goroutine that does not qualify for `Close`: it stops the work, waits for it, and returns `ctx.Err()` when the context ends first; the work then exits once what it waited on returns, and a later `Shutdown` returns nil. ``
  - `component/lifecycle.go:54-56` — `Stop(ctx) is the only stop. It drains, joins and cleans up within ctx's bound and returns nil only once everything is released; if ctx ends first, it returns that context's error and a later Stop tries again.`
- **Spelling A1 — Initiates teardown unconditionally; returns `nil` when unstarted or already joined by an earlier call, and reports `ctx.Err()` when `Stop`/`Close` runs on an active owner under an ended `ctx`:**
  - `natsclient/client.go:634-637,703-707` — `When ctx ends first, or has already ended, Close returns ctx.Err() at once, even while another Close is draining, and the cleanup continues; a later Close waits for the same completion.`
  - `natsclient/client.go:984-1006` — `Subscription.Drain(ctx)` issues `s.sub.Drain()` once via `s.drainOnce` and enters a single `select` between `<-s.done` and `<-ctx.Done()`.
  - `metric/handler.go:204-210,226-254` — `metric.Server.Stop(ctx)` returns `nil` when `s.server == nil` and `requests.Pending() == 0` (`:204-210`), otherwise calls `requests.CloseAdmission()`, `httpServer.Shutdown(ctx)`, falls back to `httpServer.Close()` when `ctx` expired, and records `if err := ctx.Err(); err != nil { stopErr = errors.Join(stopErr, err) }` (`:249-251`).
  - `internal/harness/lifecycletest/lifecycletest.go:219-237` — `CheckStopReturnsItsEndedContext` requires `Stop` on a started owner given a cancelled `stopCtx` to return an error matching `stopCtx.Err()` or `context.Cause(stopCtx)` (treating `nil` as a violation).
  - Inner component `cleanup(ctx)` functions that initiate every drain/cancel step without early return and end with `errors.Join(cleanupErr, ctx.Err())` (currently behind `Guard.Stop`'s pre-flight check):
    - `processor/graph-ingest/component.go:553-593` — `Component.cleanup(ctx)` issues `binding.handle.Drain()` on all consumers, runs `WaitConsumerHandlers`, cancels `ingestSubmitCancel`, runs `ingestPool.Shutdown(ctx)`, cancels `ingestPoolCancel`, drains all subscriptions, cancels `c.cancel()`, waits `<-c.statusDone` vs `<-ctx.Done()`, closes `entityCache`, and returns `errors.Join(cleanupErr, ctx.Err())` (`:592`).
    - `storage/objectstore/component.go:513-539` — `Component.cleanup(ctx)` drains `c.consumers` and `c.subscriptions`, runs `WaitConsumerHandlers`, cancels `c.cancel()`, waits `<-c.checkDone` vs `<-ctx.Done()`, and returns `errors.Join(cleanupErr, ctx.Err())` (`:538`).
- **Spelling A2 — Initiates teardown unconditionally; returns `nil` immediately if no work is in flight even when `ctx` has already ended:**
  - `internal/dispatch/keyed_pool.go:285-308` — `KeyedPool.Shutdown(ctx)` sets `p.stopped = true` and launches `p.drain()`, then checks `case <-p.stopDone: return nil` before `select` on `<-ctx.Done()` (`TestShutdownAfterJoinReturnsNilUnderEndedContext` at `keyed_pool_test.go:255`).
  - `internal/cache/coalescing_set.go:195-215` — `CoalescingSet.Shutdown(ctx)` closes `c.shutdown` via `c.closeOnce`, then checks `case <-c.done: return nil` before `select` on `<-ctx.Done()`.
  - `internal/httpadmission/admission.go:91-93` — `Requests.Wait(ctx)` returns `nil` immediately when `r.idle == nil` (no admitted HTTP requests in flight).
  - `natsclient/owned_delivery.go:232-258` — `Client.WaitConsumerHandlers(ctx, handle)` returns `nil` immediately when all handler deliveries have already finished (`:246-250`).
  - `internal/kvwatch/kvwatch.go:39-65` — `kvwatch.Stop(ctx, watcher)` calls `watcher.Stop()` and drains `watcher.Updates()` under `context.WithTimeout(context.WithoutCancel(ctx), DrainBudget)` (`5s`).
  - `service/pprof.go:76-88` — `PProfServer.Shutdown(ctx)` refuses `ctx == nil`, returns `nil` on a nil receiver, and calls `s.server.Shutdown(ctx)` (`:83`).
  - `service/service_manager_http.go:211-225` — `stopHTTP` calls `requests.CloseAdmission()`, `server.Shutdown(ctx)`, `server.Close()` when `Shutdown` returns an error, `<-serveDone`, and `requests.Wait(ctx)`.
  - Inner `cleanup`/`stop`/`release` functions (currently behind `Guard.Stop`'s pre-flight check) that initiate teardown across their owned resources and return `nil` if their channel/wait is already closed:
    - `service/base.go:227-249` — `BaseService.stop(ctx)` sets `StatusStopping`, calls `cancel()`, and selects `<-done` vs `<-ctx.Done()` (`LogForwarder` at `service/log_forwarder.go:32-36` promotes `BaseService.Stop` directly).
    - `service/metrics.go:162-184` — `Metrics.cleanup(ctx)` calls `server.Stop(ctx)` and `m.BaseService.Stop(ctx)`, joining errors.
    - `config/manager.go:551-589` — `config.Manager.release(ctx)` closes subscriber channels, spawns `cm.stopWatches(ctx, watchers, cancel)` (which calls `kvwatch.Stop`) on `workDone`, and selects `<-done` vs `<-ctx.Done()`.
    - `gateway/graph-gateway/component.go:132-135` — `Component.cleanup(ctx)` calls `c.requests.CloseAdmission()` and returns `c.requests.Wait(ctx)` (returns `nil` when idle even if `ctx` has ended, which interacts with `CheckStopReturnsItsEndedContext` unless `Guard.Stop` reports `ctx.Err()`).
    - `processor/graph-clustering/component.go:362-378` — `Component.cleanup(ctx)` calls `c.runCancel()` first, then selects `<-done` vs `<-ctx.Done()` on `c.feeds` and `c.ticks`.

### Spelling B — Pre-flight `ctx.Err()` refusal ("given an ended context, `Stop` stops nothing") or sequential early return in `cleanup(ctx)` before initiating teardown of remaining owned resources

- Guard, manager, `Stop` wrapper, and fixture gates:
  - `internal/lifecycleguard/guard.go:109-111` — `if err := ctx.Err(); err != nil { return err }` (doc comments at `:10-13` and `:92-98`; also at `:129-130`, `case <-ctx.Done(): return ctx.Err()` while `startDone != nil` without re-checking whether `startDone` closed concurrently).
  - `service/service_manager.go:677-679` — `if err := ctx.Err(); err != nil { return err }` in `Manager.StopAll` before `m.lifecycle.Stop(ctx)` (`:683`) and `m.stopHealthListener(ctx)` (`:687`).
  - `service/component_manager_lifecycle.go:184-187` — `if err := ctx.Err(); err != nil { failures = append(failures, componentStopError(record.name, err)); continue }` skips `cm.deregisterProvidedStores(record.name)` (`:190`) and `lifecycle.Stop(ctx)` (`:191`). (`deregisterProvidedStores` at `service/component_manager.go:506-522` swaps `provided` to `nil` and deletes matching entries, making repeated calls a no-op.)
  - `service/message_logger.go:515-519` — `if ctx != nil && ctx.Err() == nil { ml.lifecycleMu.Lock(); ml.watchesClosed = true; ml.lifecycleMu.Unlock() }` (leaves `openWatchStream` at `:606-615` unfenced when `Stop` is called with an ended `ctx`).
  - `service/metrics.go:152-156` — `if ctx != nil && ctx.Err() == nil { m.mu.Lock(); m.lifecycleCalled = true; m.mu.Unlock() }` (leaves `claimManagerServer` at `:207` unfenced when `Stop` is called with an ended `ctx`).
  - `internal/harness/natsfixture/stop.go:26-28` — `if err := ctx.Err(); err != nil { return fmt.Errorf("natsfixture: stop not begun: %w", err) }` when `len(f.remaining()) > 0` (`Shared.Stop` at `internal/harness/natsfixture/shared.go:163-174` delegates to `s.f.Stop(ctx)`, which returns `nil` at `stop.go:23-25` when `len(f.remaining()) == 0`, tested at `shared_integration_test.go:29` `TestSharedNeverResetStartsNothing`).
- Eight `cleanup(ctx)` implementations that return early on `<-ctx.Done()` or `sub.Drain(ctx)` error before canceling `runCancel` or initiating teardown of remaining owned resources:
  - `service/heartbeat.go:188-196` — calls `hb.runCancel()`, then returns on `case <-ctx.Done():` (`:191-192`) before `hb.BaseService.Stop(ctx)` (`:194`), leaving `BaseService` in `StatusRunning` with `healthMonitor` (`service/base.go:211`) uncancelled.
  - `service/metrics_forwarder.go:190-198` — calls `mf.runCancel()`, then returns on `case <-ctx.Done():` (`:193-194`) before `mf.BaseService.Stop(ctx)` (`:196`).
  - `service/storage_observability.go:302-310` — calls `s.runCancel()`, then returns on `case <-ctx.Done():` (`:305-306`) before `s.BaseService.Stop(ctx)` (`:308`).
  - `service/message_logger.go:529-550` — returns early when `ml.joinRetry(ctx)` (`:530`) or `ml.drainSubscriptions(ctx)` (`:533`) returns an error, before canceling `ml.runCancel` (`:542`), calling `ml.endWatchStreams(ctx)` (`:545`), and calling `ml.BaseService.Stop(ctx)` (`:548`).
  - `processor/graph-embedding/component.go:411-435` — returns early when `sub.Drain(ctx)` fails (`:415-417`, including when `ctx` is already ended because `Subscription.Drain` waits on asynchronous native closure) before `c.runCancel()` (`:419`), loop joins (`:421-430`), and `c.coalescer.Shutdown(ctx)` (`:432`).
  - `processor/graph-index/component.go:308-332` — returns early when `sub.Drain(ctx)` fails (`:312-314`) before `c.runCancel()` (`:316`), and returns early on `<-ctx.Done()` waiting for `c.feeds` (`:322-323`) before `c.pool.Shutdown(ctx)` (`:326`) and `c.poolCancel()` (`:331`).
  - `processor/graph-query/component.go:212-228` — returns early when `sub.Drain(ctx)` fails (`:216-218`) before `c.runCancel()` (`:220`) and `<-c.supervised` (`:224`).
  - `output/websocket/websocket.go:263-295` — returns early on `c.sub.Drain(ctx)` error (`:264-266`) before `c.closeClients()` (`:268`) and `c.runCancel()` (`:287`), and returns early on `<-ctx.Done()` waiting for `<-c.served` (`:273-274`) or `c.requests.Wait(ctx)` (`:283-285`) before `c.runCancel()` (`:287`).
- Harness reference double and tests asserting Spelling B:
  - `internal/harness/lifecycletest/refowner_test.go:149-164` — `if ctx != nil && ctx.Err() != nil { ... return ctx.Err() // the bound is not a join: keep the worker for a later Stop }` before `o.stopOnce.Do(func() { close(o.stopCh) })` (`:170`), and `TestAbortStopThenFinishJoinsWorker` at `:454-482`.
  - `internal/lifecycleguard/guard_test.go:278-352` — `TestGuardRefusesNilAndEndedContexts`
  - `service/base_lifecycle_test.go:103-126` — `TestStopWithAnEndedContextStopsNothing`
  - `service/service_manager_test.go:506-525` — `TestStopAllWithAnEndedContextStopsNothing`
  - `service/component_manager_prop_test.go:88-92,128,489-499` — `cmPropHistory.stopEnded`
  - `processor/graph-ingest/component_integration_test.go:119` — `TestComponent_Stop_Timeout` asserts `Stop` with an expired context refused before `cleanup`.
  - `processor/graph-index/lifecycle_integration_test.go:277-285` — comment describing `sub.Drain` ordering before `runCancel`.

## 3. Adjacent claims on the territory

- **Current specs:**
  - `openspec/specs/service-lifecycle/spec.md:29-40,186-196,406-421,700-704,947-951,1090-1094,1192-1193,1361-1370` — eight requirements/scenarios specify that `Stop(ctx)` / `StopAll(ctx)` given an already-ended context returns its error and stops nothing, and `:420-421` mentions components "not reached because `ctx` ended".
  - `openspec/specs/component-lifecycle/spec.md:30-47` — `A shell SHALL be running from the moment the start function given to Start returns nil until the cleanup function begins` (`component/lifecycle_type.go:73-77` sets `l.running = false` inside `Guard`'s `cleanup` wrapper).
  - `openspec/specs/operator-surface/spec.md:56-62,209-216` — `/readyz` answers `NOT READY` from the moment `StopAll` begins, and `StopAll` stops the health listener whether or not `StartAll` ran.
  - `openspec/specs/lifecycle-suite/spec.md:27-41` — `CheckStopReturnsItsEndedContext` requires `Stop` with an already-ended context to return an error carrying that context's error or cause; `CheckRepeatedStopIsNoop` requires `Stop` after a `nil` `Stop` to be a no-op within its bound.
  - `openspec/specs/config-manager/spec.md:26-49` — `Stop is the only stop` and `Stop honours its context` (does not carry the "ended context stops nothing" clause in the spec, though `config/README.md:211`, `config/doc.go:94`, and `config/manager.go:535-538` state it in prose/comments).
  - `openspec/specs/background-work/spec.md:12-60` — three shapes (`Run(ctx)`, `Close()`, `Shutdown(ctx)`).
- **Docs, Go doc comments, and ledger rows:**
  - `service/README.md:54-56,153-155,275-281,463-464,811-813` and `service/doc.go:47-49,133-136,237-245,344-346` — prose stating an ended `Stop`/`StopAll` context stops nothing.
  - `service/base.go:215-216`, `service/service_manager.go:659-660`, `service/component_manager_lifecycle.go:132-133` — Go doc comments on `BaseService.Stop`, `Manager.StopAll`, and `ComponentManager.Stop` stating an ended context stops nothing.
  - `config/README.md:211`, `config/doc.go:94`, `config/manager.go:535-538` — prose/comments stating an ended `ctx` stops nothing.
  - `component/lifecycle.go:77-79` — doc comment stating `nil and ended contexts refused`.
  - `docs/admission-ledger.yaml:3158-3159,3172,3291` — `service` ledger contract notes recording `a Stop given an ended context stops nothing`, `a StopAll with an ended context stops nothing`, and `(BaseService's guard refuses a nil or ended context)`.
- **Active OpenSpec changes on `main`:**
  - `openspec/changes/setup-04b-02-lexical-processor/` — modifies `graph-transport-boundary`, `composed-stack`, `graph-embedding`; adds `graph-lexical`, `entity-text`. Does not touch `service-lifecycle`, `component-lifecycle`, or `lifecycleguard`.
- **Open pull requests (`gh pr list --state open --json number,title,changedFiles,files`):**
  - `#649` (`gemini/fix-604-stop-timeout`, draft, 0 changed files on empty claim commit `bfbfd6bc`): this claim PR.
  - `#648` (`claude/lexical-component`, draft, 15 files, `setup-04b-02-lexical-processor` PR C): adds `processor/graph-lexical/component.go` composing `component.Lifecycle`. Inspected via `gh pr diff 648`: `graph-lexical`'s `cleanup(ctx)` calls `c.runCancel()` unconditionally before `select` on `<-c.feeds` vs `<-ctx.Done()`. Touches `internal/smoke/stack_integration_test.go`, `AGENTS.md`, `.agents/rules-index.md`, and `openspec/changes/setup-04b-02-lexical-processor/*`; zero file overlap with `internal/lifecycleguard`, `service`, `config`, `component`, or the four existing components in §2.
  - `#598` and `#597` (Dependabot `go.mod` / `go.sum` bumps): zero file or capability overlap.
- **Open issues (`gh issue list --state open --limit 200` searched for `service`, `component`, `lifecycleguard`, `Stop`, `StopAll`, `Shutdown`):**
  - `#604` — `ComponentManager.stopComponents skips remaining components when stop context expires` (`v0.1.0-alpha.5`, claimed by PR `#649`).
  - `#196` — `service: StartAll calls service Start concurrently without enforcing cross-service readiness` (`v0.2.0`, startup ordering only).
  - `#570` — `service: ComponentManager.Status mixes running count into Healthy` (`v0.2.0`, status reporting only).
  - `#564` — `service: drop FlowGraph validation adapters once composition owns the checks` (`v0.2.0`, validation adapters only).
  - `#641` — `graph-embedding: in-flight worker embed call uses component runCtx without per-call timeout` (`v0.1.0-alpha.5`, worker HTTP call timeout; complements `runCancel()` cancellation during `Stop`).
  - `#361` — `lifecycle-suite: add a component conformance adapter alongside Owner` (`v0.2.0`, test harness ergonomics).

## 4. Who it is for

No new exported symbol, type, method signature, port, NATS subject, KV bucket, or configuration key is proposed or needed (`git grep` / `gopls` check: existing signatures `Start(context.Context) error` and `Stop(context.Context) error` on `lifecycleguard.Guard`, `component.Lifecycle`, `service.Service`, `service.Manager`, `config.Manager`, and `component.LifecycleComponent` already cover every caller).

## 5. The problem shape

Problem shape: **terminal teardown under a caller-bounded context** — initiate shutdown across all owned admissions, drains, listeners, and child run contexts immediately, and use `ctx` solely to bound waiting for graceful drain and goroutine joins, preserving retry authority if a join outlasts `ctx`.

Closest existing instances in this repository:

- Single-owner HTTP/server teardown: `metric/handler.go:195-278` (`Server.Stop`) and `service/service_manager_http.go:211-225` (`stopHTTP`).
- Multi-resource component teardown: `processor/graph-ingest/component.go:553-593` (`Component.cleanup`) and `storage/objectstore/component.go:513-539` (`Component.cleanup`), both of which initiate every drain and cancellation and aggregate join errors via `errors.Join(cleanupErr, ctx.Err())`.
- Transport & background-work teardown: `natsclient/client.go:629-708` (`Client.Close`), `natsclient/client.go:984-1006` (`Subscription.Drain`), `natsclient/owned_delivery.go:232-258` (`Client.WaitConsumerHandlers`), `internal/kvwatch/kvwatch.go:39-65` (`kvwatch.Stop`), `internal/dispatch/keyed_pool.go:285-308` (`KeyedPool.Shutdown`), and `internal/cache/coalescing_set.go:195-215` (`CoalescingSet.Shutdown`).

## 6. Decision records

Carries no package from the SemStreams pin. Governing SemEngine ADRs already in `docs/adr/`:

- `docs/adr/113-start-context-bounds-start.md:52-64` — `ADR-113: Start's context bounds Start; Stop is the only stop` (decides that `Start` creates one private run context ended in `Stop`, and `Stop(ctx)` / `Shutdown(ctx)` make the same promise: stop, wait, return `ctx.Err()` if the bound wins, and let a later call try again).
- `docs/adr/108-processor-shell.md:1-95` — `ADR-108: One processor shell (component.Lifecycle)` (decides components compose `component.Lifecycle` instead of hand-rolling state machines).
- `docs/adr/095-one-shot-running-lifecycle-with-retained-failed-start-cleanup-authority.md` — one-shot running lifecycle and retained failed-`Start` cleanup authority.
- `docs/adr/094-boot-only-composition-and-observable-rule-activation.md` — boot-only composition sealing in `service.Manager` and `ComponentManager`.
- `docs/adr/112-natsclient-contract.md` — `natsclient.Client.Close` contract under bounded and ended contexts.

## Same-class collision table (runtime lifecycle & teardown coordination)

| Dimension | Inventory evidence |
|---|---|
| Semantic class | Coordinating `Start(ctx)` / `Stop(ctx)` / `Shutdown(ctx)` / `Close(ctx)` transitions, run-context cancellation, drain initiation, and bounded join waiting |
| Owners | `internal/lifecycleguard.Guard` (`guard.go:29-145`); `component.Lifecycle` (`component/lifecycle_type.go:28-92`); `service.BaseService` (`service/base.go:56-249`) & `service.LogForwarder` (`service/log_forwarder.go:32-36`); `service.Manager` (`service/service_manager.go:55-738`); `service.ComponentManager` (`service/component_manager_lifecycle.go:29-209`); `service.{Metrics,MessageLogger,HeartbeatService,MetricsForwarder,StorageObservabilityService}`; `config.Manager` (`config/manager.go:56-589`); `metric.Server` (`metric/handler.go:39-278`); `service.PProfServer` (`service/pprof.go:33-88`); `natsclient.Client` (`natsclient/client.go:629-708`), `natsclient.Subscription` (`natsclient/client.go:984-1006`), & `Client.WaitConsumerHandlers` (`natsclient/owned_delivery.go:232-258`); `internal/kvwatch.Stop` (`internal/kvwatch/kvwatch.go:39-65`); `internal/httpadmission.Requests` (`internal/httpadmission/admission.go:26-98`); `internal/dispatch.KeyedPool` (`internal/dispatch/keyed_pool.go:285-308`); `internal/cache.CoalescingSet` (`internal/cache/coalescing_set.go:195-215`); `internal/harness/natsfixture.{Fixture,Shared}` (`stop.go:19-47`, `shared.go:163-174`) |
| Catalogs | `service.Registry` (`service/registry.go:9-48`); `component.Registry` (`component/registry.go:28-210`); `storeregistry.Registry` (`storage/storeregistry/storeregistry.go:42-105`) |
| Status | `BaseService.Status()` / `Health()` (`service/base.go:252-330`); `component.Lifecycle.Health()` (`component/lifecycle_type.go:100-125`); `ComponentManager` per-record `componentState` (`service/component_manager.go:123-139`); `semengine_service_status` gauge (`metric/core.go:30-38`, updated via `service/base.go:281-286`); `health.service.<name>` & `health.component.<name>` NATS subjects (`service/service_manager.go:622-657`, `service/component_manager_lifecycle.go:236-282`) |
| Lifecycle | One-shot `Start(ctx)` -> running -> `Stop(ctx)` -> terminal stopped (`Guard.used`, `Guard.terminal`, `Guard.stopping`, `Guard.startDone` at `guard.go:31-42`); failed `Start` rollback via `pkg/lifecyclecleanup.RollbackFailedStart` (`pkg/lifecyclecleanup/lifecyclecleanup.go:36-57`) |
| Ownership | Single-instance per owner; `ComponentManager` owns `StoreProvider` registration/deregistration (`service/component_manager.go:459-522`); `service.Manager` claims `Metrics` server (`service/metrics.go:207`) |
| Readers | `/health`, `/readyz`, `/components/*` HTTP routes (`service/service_manager_http.go:75-185`, `service/component_manager_http.go:24-150`); `HeartbeatService` (`service/heartbeat.go:157-171`); `lifecycletest` adapters (`*_lifecycle_test.go`) |
| Writers | Process composition root (`Manager.StartAll`/`StopAll`, `config.Manager.Start`/`Stop`, `natsclient.Client.Close`); `Guard.Start`/`Stop`; `ComponentManager.startComponents`/`stopComponents` |
| Recovery | Failed `Start` runs `RollbackFailedStart` (5s detached budget); failed or cut-short `Stop` leaves `Guard.terminal == false` and `componentState.stopped == false` (`StateFailed`) so a subsequent `Stop` retries unreleased resources |

## Context retention & detachment check on inventoried packages

- `git grep -n "context\.Context" internal/lifecycleguard/ component/ service/ config/ processor/ output/ gateway/ storage/` verified: zero production structs retain `context.Context`; lifecycle owners hold only synchronized private `context.CancelFunc` fields (`BaseService.cancel`, `Component.runCancel` / `poolCancel` / `cancel`, `config.Manager.cancel`).
- Terminal bounded detachments in the inventoried teardown paths: `pkg/lifecyclecleanup/lifecyclecleanup.go:52` (`context.WithTimeout(context.WithoutCancel(parent), failedStartRollbackTimeout)`, 5s) for synchronous failed-`Start` rollback, and `internal/kvwatch/kvwatch.go:45` (`context.WithTimeout(context.WithoutCancel(ctx), DrainBudget)`, 5s) for draining `jetstream.KeyWatcher` delivery closure.

## Adopter seam inventory

Evaluated from the perspective of a consumer composition root author (`semsource` `cmd/semsource/run.go:152-172,227-232`) and a component author (`semsource` `processor/ast-source/component.go:1029-1055`, `storage/filestore/component.go:95-110`, `gateway/mcp-gateway/component.go:148-156`):

1. **What must they know today?**
   - That passing a cancelled signal `ctx` directly to `StopAll(ctx)` or `Stop(ctx)` silently leaves everything running (`guard.go:109-111`, `service_manager.go:677-679`).
   - That even when they pass a fresh 30s `stopCtx` (`cmd/semsource/run.go:227-229`), if one slow component consumes all 30s during `ComponentManager.Stop(stopCtx)`, `ComponentManager` skips `Stop` and store deregistration on all remaining components (`component_manager_lifecycle.go:184-187`), and `service.Manager.stopAll` then passes the expired `stopCtx` to remaining services whose `Guard.Stop` refuses to run cleanup (`guard.go:109-111`).
   - Component authors writing `cleanup(ctx)` must know not to return early on `sub.Drain(ctx)` before calling `c.runCancel()`, or their worker goroutines keep running past `Stop`.
2. **What happens if they do nothing today?**
   - In `cmd/semsource/run.go:229-232`, when `manager.StopAll(stopCtx)` returns `context.DeadlineExceeded` after one component stalls, `run()` returns and runs `defer configMgr.Stop` (`:168-172`) and `defer nc.Close` (`:152-156`) while skipped components (such as `storage/filestore` whose `c.store.Close()` was never called, or processors whose `runCancel` was never called) and skipped services are still actively running against NATS and local stores.
   - Note in consumer code: `semsource` `processor/ast-source/component.go:1039-1041` also has an early `if err := ctx.Err(); err != nil { return fmt.Errorf("join seed: %w", err) }` before canceling `c.cancelFuncs` (`:1046-1048`).
3. **Where do they find out today?**
   - Log noise, race detector failures on teardown, unclosed store handles, or reading `service/README.md:54-56` — nowhere at compile time or boot time.
4. **What SHOULD they have to know?**
   - Only one rule: call `Stop(ctx)` / `StopAll(ctx)` with a bounded non-nil context; even if that deadline expires before drain completes, every started service and component has already been told to stop and fence its admissions before `StopAll` returns.
   - **The gap:** today `ctx` gates *whether shutdown is initiated* in `Guard`, managers, four services, four components, and `natsfixture.Fixture`, instead of bounding *only how long `Stop` waits for graceful drain and goroutine joins*.
