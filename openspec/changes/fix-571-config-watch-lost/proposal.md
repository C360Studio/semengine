# Proposal: fix-571-config-watch-lost

Approved-by: cglusky (2026-11-22) <https://github.com/C360Studio/semengine/issues/571#issuecomment-7081641424> sha256:5cc4e35ed779

Track: `quick`. Capabilities modified: `config-manager` (one requirement added) and `operator-surface` ("Health and
service listings are served as at the pin": a `config` sub-status joins the `nats` one). Two packages, `config` and
`service`; one exported method added.

## Why

When `config.Manager`'s KV watch ends outside `Stop`, configuration changes stop reaching the process, and nothing
says so (#571). Both watch loops, `processWatcher` (`config/manager.go:633`) and `processFamily`
(`config/key_family.go:254`), return on a closed updates channel with no log, metric or health change. Whether and
when nats.go closes that channel (a long broker outage, a lost watch consumer) is not probed and not promised here;
the fix does not depend on when it happens, only on saying so when it does.

## What Changes

- A watch loop that sees its updates channel close while the run context is still live logs an error naming the
  watch (`config` or the key family's prefix) and marks the watch lost. The mark stays until the process restarts:
  nothing reopens the watch, as with the entity-state follower's lost feed.
- `(*Manager).Health() health.Status`: healthy while every watch runs; unhealthy, with the message
  `config watch lost (<names>); restart the process`, once any is lost. Before `Start` and after `Stop` it is healthy.
- The service manager's `/health` adds a `config` sub-status from that method when it has a config manager, beside
  the `nats` one, so a lost watch answers 503.
- `/readyz` is unchanged: a process with a lost config watch still serves its traffic on the configuration it has.
- Tests: in `config`, a watch loop fed a watcher whose channel closes logs the error and turns `Health` unhealthy, and
  a loop ended by `Stop` does neither (this half shows the test can fail); in `service`, `/health` carries a healthy
  `config` sub-status for a manager with a started config manager. The 503 for a lost watch rests on `Health()`'s
  tests and `health.Aggregate`, which already answers unhealthy for any unhealthy sub-status: a `service` test cannot
  lose a real watch without relying on unpromised nats.go behaviour or a test hook.

## Non-goals

- Reopening the watch. A restart is the declared remedy, as for the follower.
- A metric. The log line and `/health` are the signal the issue asks for.
- Probing nats.go's close behaviour.

## Impact

`config/manager.go`, `config/key_family.go`, `service/service_manager_http.go` and their tests; the two specs above.
One PR, `Closes #571`.
