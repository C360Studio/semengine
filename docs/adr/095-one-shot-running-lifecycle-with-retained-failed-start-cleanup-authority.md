# ADR-095: One-Shot Running Lifecycle with Retained Failed-Start Cleanup Authority

> **Ported from SemStreams.** This record is
> `docs/adr/095-one-shot-running-lifecycle-with-retained-failed-start-cleanup-authority.md` at SemStreams commit
> `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`. It came with the component manager, whose `Start` and `Stop` carry its
> lifecycle rules for components, in the third pull request of package `service`'s port (OpenSpec change
> `setup-04a-03-boot-service-component-manager`); its admission-ledger row is
> `docs/adr/095-one-shot-running-lifecycle-with-retained-failed-start-cleanup-authority.md` in
> `docs/admission-ledger.yaml`. The text below is unchanged. Read it with these facts about this repository:
>
> - **Superseded by ADR-113:** "A deadline is a failed exit result, not authority for later running-generation
>   rejoin" (the first Decision paragraph). Here a `Stop` whose context ends first returns that context's error and
>   leaves what it did not release for the next `Stop`, which tries again
>   (`docs/adr/113-start-context-bounds-start.md`). The rest of that sentence stands: a `Stop` after one that
>   returned nil is a no-op, and a `Stop` while another runs is refused as transient, not joined.
> - **Kept:** one `Start` per instance, and the failed-`Start` rule (the second Decision paragraph): a failed
>   `Start` makes one rollback; if the rollback fails, the next `Stop` runs it again and another `Start` is refused.
>   `internal/lifecycleguard` implements it, and `service.ComponentManager` composes the guard. The guard replaces the
>   fields the Decision names (`cleanupPending`, `startDone`). The manager's record of each component also keeps
>   whether that component's `Stop` has returned nil, so a retried `Stop` calls only the components that did not stop.
> - **Not about `service`:** the JetStream clauses (the retained `jetstream.ConsumeContext`, native Drain, the
>   rule that `Client` catalogues no consumer, and "acknowledgement follows the required effect") govern `natsclient`
>   and `processor/graph-ingest`. The component manager has no consumer and acknowledges nothing.
> - The links to `openspec/changes/simplify-one-shot-lifecycle-ownership/` and
>   `openspec/changes/archive/2026-08-21-require-restart-for-config-activation/` under References are the pin's, and
>   neither is carried.

## Status

**Accepted (2026-08-17).** This decision supersedes only ADR-094's managed-consumer, resumable running-Stop,
drain-and-delete, name-routed child-catalog, and retained repeated-result mechanics. ADR-094 remains immutable history.

## Context

ADR-094 correctly made composition boot-only, controlled shutdown always-exit, and dirty recovery independent of
`Close`. Its restart-safe lifecycle mechanics also selected stateful managed handles, resumable Stop/delete operations,
and retained repeated results. Production inventory distinguishes terminal Stop of a successfully running process from
cleanup of a Start that returned error after acquiring resources.

Running Stop has no measured production caller for concurrent executor election, deadline rejoin, or result replay.
Failed Start does have measured cleanup re-entry: 21 rollback paths and seven Start-finalization owners can retain
resources after bounded rollback fails. Treating those two conditions as one generalized lifecycle either overbuilds
normal Stop or discards real failed-Start cleanup authority.

Delivery ownership, server backlog observation, durable topology deletion, and acknowledgement settlement are also
different responsibilities. Combining them in a Client catalog or stateful wrapper hides duplicate ownership and asks
adopters to know consumer names and cleanup policy the framework can observe directly.

## Decision

Running owners retain the exact native `jetstream.ConsumeContext` returned at the delivery commit point. All fallible
stream, consumer, policy, and observation setup finishes before `Consumer.Consume`. Controlled Stop fences admission,
initiates native Drain or Shutdown, awaits exact native Closed while callback authority remains live, cancels remaining
runtime work, awaits owner done/WaitGroup, performs terminal cleanup, aggregates errors, closes terminal transport, and
exits. A deadline is a failed exit result, not authority for later running-generation rejoin. Completed repeated Stop
and Close are nil no-ops; concurrent Stop/Close and retained result replay are not contracts.

Failed Start is distinct. An owner publishes its cleanup record before acquisition can escape and retains `startDone`
where Stop can race Start. It attempts one bounded synchronous rollback. Cleanup authority clears only after success.
If rollback fails or expires, every exact handle remains retained in `cleanupPending`, another Start is rejected, and
manager Stop may retry cleanup with its caller context.

Client does not catalog, replace, rediscover, drain, or delete component-owned children. Duplicate local
`(stream,durable)` identity fails at boot through sealed canonical validation where derivable, otherwise through a
minimal reject-only identity-plus-owner-token claim. It never stops or replaces the incumbent. Normal Stop and Client
Close never delete durable topology; namespace-scoped fixtures or administration delete only identities they recorded.
Outstanding-work observation remains read-only and separate from lifecycle authority.

Acknowledgement follows the required effect. Pre-pool graph poison is counted and ACK-dropped under its existing policy
without borrowing the keyed convergence claim; zero backlog never promotes that disposition to semantic completeness.
After keyed admission, graph effect precedes durable guard, which precedes ACK. Replayable effects declare stable
idempotency or durable progress; external lanes that cannot do so declare explicit at-most-once limits. Plain Ack is
not server-confirmed settlement. `DoubleAck(ctx)` is used only where a declared latency/failure SLO requires server
confirmation, and its failure remains replay-safe and non-clean.

## Consequences

`Generation` and `Operation` retire after owner migration. The shared-helper ceiling is one stateless context wait
helper plus the bounded failed-Start rollback helper. There is no managed lifecycle wrapper, drain-and-delete surface,
lifecycle catalog, deletion knob, or name-routed lifecycle operation.

ADR-094's boot-only composition, dedicated Rule activation, raw-root retirement, always-exit controlled shutdown,
dirty recovery, and proof gates remain accepted. Controlled and dirty real-process proof, effect-to-ACK inventories,
and honest external-effect guarantees remain release gates; this decision does not claim their implementation.

## References

- [ADR-094: Boot-Only Composition and Observable Rule Activation](094-boot-only-composition-and-observable-rule-activation.md)
- `openspec/changes/simplify-one-shot-lifecycle-ownership/`
- `openspec/changes/archive/2026-08-21-require-restart-for-config-activation/`
