# Proposal: setup-04a-03-service-startup-seam

## Why

Port-refactor #364. The service manager reaches the component manager through `componentStartup`, an unexported
interface with one implementation (`service/service_manager.go:40-47`), and hands it the startup metric writer after
construction (`setStartupMetricWriter`, `service/component_manager_lifecycle.go:320-324`). The owner asked on
2026-10-24 to step back: "pragmatic and idiomatic over clever is our goal in semengine. i am not sure we have the right
owner but it smells like we need a common spot to manage this?" All seven `service` pull requests have landed (#393,
`b3a4fa9`), so every caller is in the tree. This is the first of three serial `service` port-refactors: #364 first,
then #363 and #362 (owner ruling on #364, 2026-10-24).

## What Changes

- The `semengine_startup_units` gauge reads the startup counts when it is collected, from where they already live (the
  service manager's outcomes and the component manager's records), instead of being pushed by both managers. The
  component manager no longer touches the gauge: its writer field, `setStartupMetricWriter` and
  `recordComponentStartupMetrics` go.
- `componentStartup` goes. The service manager and `ComponentsImplementing` check for the framework's
  `*ComponentManager`, as the SemStreams pin did; any other service named `component-manager` keeps the pin's
  zero-count path.
- The readiness check for an admitted component missing from health goes: it cannot fire once the component manager
  is the concrete type.
- The test double `componentStartupDouble` goes; its tests run against a real `ComponentManager`.

Observable behaviour is unchanged: `/readyz`, `/services`, the gauge's nine series and values, the refusal of a foreign
collector before any listener binds, and a component manager started on its own publishing nothing.

## Capabilities

`operator-surface`: one ADDED requirement stating the startup gauge's series and that it reports the counts as they
are when collected (design D1).

## Non-goals

Refusing a `component-manager` that is not the framework's (the heartbeat already does; the manager keeps the pin's
tolerance). #362 and #363, which follow this pull request. No exported surface changes.
