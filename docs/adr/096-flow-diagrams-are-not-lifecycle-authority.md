# ADR-096: Flow Diagrams Are Not Lifecycle Authority

> **Ported from SemStreams.** This record is `docs/adr/096-flow-diagrams-are-not-lifecycle-authority.md` at
> SemStreams commit `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`. It came with the component manager, which the one
> Decision paragraph that stands describes, in the third pull request of package `service`'s port (OpenSpec change
> `setup-04a-03-boot-service-component-manager`); its admission-ledger row is
> `docs/adr/096-flow-diagrams-are-not-lifecycle-authority.md` in `docs/admission-ledger.yaml`. The text below is
> unchanged. Read it with these facts about this repository:
>
> - **Only the fourth Decision paragraph stands** ("ComponentManager reads the existing configuration once during
>   construction..."), as `docs/adr/100-compositions-are-validated-diagrams-are-projections.md` also says. The
>   component manager reads the boot configuration once, in `NewComponentManager`, subscribes to no later change,
>   and is the only owner of the component handles; the registry it builds from is sealed after the build and gives
>   out declarations, not components (`component.Registry`'s doc comment says the same).
> - **The rest names code this repository does not have.** `flowstore`, the Flow builder, `POST
>   /flows/{id}/publish-component-configs` and the Flow health, metrics and message endpoints were never ported, so the
>   other Decision paragraphs and the Consequences about the Flow builder are SemStreams' history. ADR-100 lists
>   which of them it supersedes.
> - The migration guide and the archived change under References are the pin's and are not carried; the pin's own
>   note says the guide was retired.

## Status

**Accepted (2026-08-18).** This decision records the owner-approved PR #990 boot-only disposition. It supersedes only
ADR-094's Flow activation clauses. ADR-094 remains immutable history; ADR-095 and the active lifecycle recovery ledger
remain the sole authority for lifecycle mechanics and proof.

## Context

SemStreams needs a useful visual Flow authoring model, but no current product needs a second lifecycle authority
derived from that model. Runtime state, component ownership, deployment verbs, and status streams made a saved diagram
look like a running process even though ComponentManager is the runtime owner.

The useful capability is smaller: save and validate a diagram, compile its nodes into ordinary component configuration
candidates, explicitly publish those candidates, and reboot the process when the operator chooses.

## Decision

`flowstore.Flow` contains identity, authoring metadata, canvas nodes and connections, audit fields, and a compare-and-
swap version. It contains no component lifecycle state or claim about current runtime membership. Diagram create,
update, and delete affect only flowstore.

Engine validates and compiles. Compilation validates first, rejects duplicate instance names, and returns detached,
enabled component configuration candidates. Engine owns no component or service lifecycle.

The only diagram-to-config write is explicit `POST /flows/{id}/publish-component-configs`. It sorts component instance
names and upserts them sequentially through the existing Config Manager component write. It never infers deletion from
an omitted node. Partial failure reports the exact persisted names and failed name; retrying is safe. Success reports
that the running process is unchanged and reboot is required.

ComponentManager reads the existing configuration once during construction and builds that fixed component set. It
does not subscribe to later component or model-registry configuration changes. Registry admits defensive declaration
values during boot, seals after composition, and does not expose live component handles. ComponentManager remains the
sole owner of the concrete handles.

Flow lifecycle routes, state, tools, metrics, timestamps, logs, and streams retire without aliases. No replacement
monitor is introduced. Retained Flow health, metrics, and message endpoints are saved-diagram observations: they query
component names declared in the diagram and do not claim the diagram owns or activated those components.

Rule code, storage, watchers, and current behavior are unchanged. This decision does not advance the separate Rule
hot-reload target.

## Consequences

The Flow builder remains useful for authoring, validation, connection discovery, and producing component configuration
for a later process. The framework has one runtime composition owner and does not add a shutdown state machine to Flow.

Publication is upsert-only, so omission is never deletion. Process supervision remains responsible for reboot. A
foreign shared-bucket platform identity fails Config Manager Start before arbitration, watchers, writes, or dependent
construction; detached running mode is removed so publication can report exact progress. Ordinary Config Manager
persistence, lifecycle mechanics, ACK ordering, NATS shutdown, and recovery remain unchanged.

This is a breaking pre-v1 simplification. Removed surfaces receive no compatibility aliases. SemStreams documents
downstream impact, but sister repositories remain read-only and their owners perform their own migrations.

## References

- [ADR-094](094-boot-only-composition-and-observable-rule-activation.md)
- [ADR-095](095-one-shot-running-lifecycle-with-retained-failed-start-cleanup-authority.md)
- [Disposition](../../openspec/changes/archive/2026-08-21-require-restart-for-config-activation/pr990-boot-only-disposition.md)
- [Archived change](../../openspec/changes/archive/2026-08-21-require-restart-for-config-activation/)
- [Migration guide](../operations/migration-boot-only-flow-activation.md)
  — migration guide retired with #1093; see [ADR-100](100-compositions-are-validated-diagrams-are-projections.md)
  and [`migration-beta162-to-beta163.md`](../operations/migration-beta162-to-beta163.md).
