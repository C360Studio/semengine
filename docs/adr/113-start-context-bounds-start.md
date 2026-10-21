# ADR-113: The Context Given to Start Bounds Start; Stop Is the Only Stop

## Status

**Accepted (2026-10-18).** The owner accepted the record as written on #282 (comment 6250318494). Drafted on PR #304
(`Addresses #282`), its own documents-only pull request ahead of the implementing change, on the owner's go of
2026-10-18 (the ruling's point 3 had placed it in that change). It records these owner rulings and adds no decision of
its own; what it adds is the reason, the alternatives rejected and the consequences.

- 2026-10-16, #282 comment 6225878599: option 2. The context given to `Start` bounds only the `Start` call, and
  `Stop(ctx)` is the only way to stop a started component.
- 2026-10-18, #282 comment 6249934379: the full record, in five points. Points 1 and 2 are the Decision below.
- 2026-10-16, #131 comment 6224282566: the keyed pool's one stop path, `Shutdown(ctx)`.
- 2026-10-05, #9 comment 5994720412 item 2: in natsclient, the context that bounds setup no longer parents handlers.

The implementing change is PR 2 of the order on #282, after the guard tidy (#196) and before #291; this record does not
close #282. Mechanics (what each check does, lock and join order) belong to that change's `lifecycle-suite` delta (pin
`docs/adr/README.md`). Tree cites are `path:line` at `bf0b012`, unchanged in these files since `c3c3222`, where #282's
counts were measured. Pin cites are at SemStreams `8b99efe9`.

## Context

Four terms. A *lifecycle owner* is a type with `Start(ctx)` and `Stop(ctx)`: a component or a service. *Continuing work*
is what it leaves running when `Start` returns: subscriptions, request handlers, worker pools, loops. A *controlled
stop* stops taking new work, drains and joins what is running, and releases every handle, within the bound of the
context given to `Stop`. The *composition root* is the code that builds and wires the process, usually `main`.

The carried contract gives a started component two ways to stop. "Stop is controlled while the Start context is still
live", and "Ending the Start context first makes Stop an abort" (`component/lifecycle.go:49-53`). Two checks of the
lifecycle suite (`internal/harness/lifecycletest`, the checks every owner's test runs) hold every owner to that
(`internal/harness/lifecycletest/lifecycletest.go:183-224`). #282 names two costs:

- A caller cannot bound startup. A deadline on `Start`'s context also stops the component once it fires, yet
  graph-ingest's `Start` connects to NATS, sets up buckets and sweeps `ENTITY_STATES` before it returns
  (`processor/graph-ingest/component.go:981-1019`).
- A `main` that passes its signal context to `Start` and calls `Stop` on SIGTERM races the abort against the controlled
  stop on every ordinary shutdown. Only the doc comment and the suite tell its author not to.

The pin's component manager shows the cost. It keeps a cancel per component (`service/component_manager.go:622-628`) and
cancels in two orders: before `Stop` when a failed start's cleanup is pending, after `Stop` otherwise (`:1033-1042`).
The manager has to choose an order that belongs to each component's cleanup.

One layer down, this repository already keeps the two contexts apart. natsclient's `Connect` returns within its context,
which "does not bound later reconnects" (ADR-112, `docs/adr/112-natsclient-contract.md:121-127`). Its metrics poller
runs under a root the client owns, which "outlives Connect's dial-scoped context on purpose"; `Close` cancels and joins
it (`natsclient/client.go:602-604`). Consumer handlers run under a root the client owns, because "the setup context must
not parent handlers that outlive it" (`natsclient/stream.go:455-459`). The ruling behind it, #9 comment 5994720412 item
2, named the failure: a setup deadline cancelled every later handler while messages kept arriving. The keyed pool's
constructor context stops nothing: "Shutdown is the only stop" (`internal/dispatch/keyed_pool.go:115-118`). The
component contract is the one place left with two stops.

Two stop verbs exist today. `Stop(ctx)` is the verb of a lifecycle owner, and the lifecycle suite runs against `Start`
and `Stop`. `Shutdown(ctx)` is the background-work verb for a type with a constructor and no `Start` whose stop waits on
I/O or a callback (`openspec/specs/background-work/spec.md:20-22`; the keyed pool). Both make the same promise: stop,
wait, return the context's error if the bound wins, and let a later call try again. This record changes neither name.

## Decision

**The contract, as behaviour.** `Start(ctx)` does its startup work under `ctx` and returns when the component is ready;
if `ctx` ends first, `Start` returns that error and holds nothing. Ending `ctx` after `Start` returned stops nothing.
`Stop(ctx)` is the only stop: nil only after everything is released; if `ctx` ends first it returns that error and a
later `Stop` tries again. Inside `Start` the component creates the one run context for its continuing work, keeps only
its cancel, and ends it inside `Stop`. The *run context* is the context that work runs under after `Start` returns; it
does not end when the Start context ends.

**Who controls the lifecycle.** The composition root cancels its context on SIGTERM. The component manager, the one
owner that watches that context, calls `Stop` on every component in reverse start order under a bounded stop context.
The parent's cancel reaches a component through the manager's `Stop`, not through an inherited context. A component's
run context ends only in `Stop`. How the manager receives and watches the root context is change 3's to name.

**No constructor is named.** How a component makes its run context is its own choice (the owner: "not sure we need to be
specific about the type of run context they create"). The suite check that enforces the contract cancels the Start
context after `Start` returns, requires the owner to keep running, then requires `Stop` to release everything. Any run
context that dies with the Start context fails that check, whatever built it, so a named constructor would add nothing
the check does not hold. The ban on new root contexts gains one narrow exception, worded by this behaviour: a lifecycle
owner's `Start` creates the one run context for its continuing work, keeps it only as a cancel, and ends it inside
`Stop`.

**Note, 2026-10-21 (change `setup-04a-03-boot-service-component-manager`).** The Decision above leaves "how the
manager receives and watches the root context" to change 3; that change names it here, and it is not a new ruling.
`service.ComponentManager` watches no root context. Its `Start` takes the composition root's context only as the
bound on `Start`, as `Manager` does with `StartAll`, and passes that same context to each component's `Start`; the
manager keeps no per-component context or cancel and never ends the context it was given. The composition root is
what reacts to the root context ending: on SIGTERM it calls `StopAll` (or the manager's `Stop`) with a fresh bounded
context, and the component manager's `Stop` stops every component in reverse start order under that bound. So the
parent's cancel still reaches a component through the manager's `Stop` and never through an inherited context; only
the word "watches" in the second paragraph moves from the component manager to the composition root. Two things
follow. A `Stop` that arrives while components are still starting waits for every `Start` to return, or returns its
own context's error first, and a caller cuts a slow boot short only by ending the context it gave `Start`. The three
composition roots already shut down this way (semsource `cmd/semsource/run.go:222`, semteams `cmd/semteams/main.go:937`
and semboids `cmd/semboids/main.go:465`, read for this change's design).

## Alternatives rejected

- **Keep the carried contract (option 1 on #282; doing nothing).** The run context descends from the Start context,
  which the composition root keeps live until `Stop` returns. Its costs are those in Context: two ways to stop, the race
  in a `main` wired the obvious way, startup that cannot be bounded, and two cancel orders in the manager.
- **`Run(ctx)` for components.** The manager starts components in phases and begins the next phase only when every
  `Start` in the current one has returned (pin `service/component_manager.go:578-611`, `:670`); `Run` returns only when
  the work ends, so it never says "ready". And once `Run` is called, the caller has no way to hand it a stop bound: the
  drain would run under a budget the component picks, not the manager's stop context.
- **The lifecycle guard (`internal/lifecycleguard`, the shared Start/Stop state machine) holds the run context and
  cancels it.** The cancel order belongs to each owner's cleanup. graph-ingest cancels its run context after it drains
  its consumers, shuts down its pool and drains its subscriptions, and before it joins its status loop
  (`processor/graph-ingest/component.go:1104-1143`); the setup plan says "Shutdown order belongs to each resource owner;
  do not universally cancel before draining" (`docs/setup-plan.md:295`). A cancel held by the guard picks one order for
  every owner: the pin manager's problem, one layer down.

## Consequences

- **The suite.** 2 of its 8 checks change, `ControlledStopUnderLiveStartAuthority` and `AbortStopPreservesCause`
  (`internal/harness/lifecycletest/lifecycletest.go:97-106`, `:183-224`). The suite's own test double, which can be
  told to break one rule at a time so that each check is shown to catch its own violation, gains one such break: an
  owner whose work dies with its Start context.
- **Adopters, in the same pull request.** Every owner whose test runs the suite is held to the changed check. The
  metrics server hands the Start context to every served request as its base (`metric/handler.go:54-55`, `:145`).
  graph-ingest derives its run context from the Start context (`processor/graph-ingest/component.go:972`); its `start`
  splits into startup work under the Start context (connect, bucket setup, the snapshot sweep) and continuing work under
  the run context (subscriptions, handlers, pool, status loop). The third is the guard's test owner. natsclient has this
  shape already; the guard changes nothing, because the cancel order is the owner's.
- **Change 3.** The ported component manager holds no per-component cancel, so the pin's two cancel orders
  (`service/component_manager.go:1033-1042`) do not come over.
- **Rules.** The exception above is added everywhere the ban on new root contexts is stated: the developer contract
  (`.agents/contracts/semengine-developer.md:219-221` and `:232-233`), the reviewer contract
  (`semengine-reviewer.md:242-245` and `:255`, where a new root and `context.WithoutCancel` are `BLOCKING`), the
  architect contract (`semengine-architect.md:405-409`) and the `AGENTS.md` row "never invent roots" (`AGENTS.md:71`),
  with its row in `.agents/rules-index.md`. The `LifecycleComponent` doc comment's Stop rule is rewritten to this
  contract in plain words, as ADR-108 decision 3(e) asks.
- **Authors outside this module.** A component written to the carried contract, whose continuing work descends from its
  Start context, stops working once the Start context it was given ends. Inside this module the changed check catches
  that. Outside it the suite is internal and only the doc comment says so, the gap ADR-108 decision 3(e) already
  records.
- **Unchanged.** One `Start` per instance (pin ADR-095). A failed `Start` whose own cleanup fails keeps what it could
  not release for a later `Stop` (`lifecycle-suite`, "Failed start whose own cleanup fails"). Boot-only composition and
  always-exit shutdown (pin ADR-094). ADR-094's rule that a signal does not cancel work before owners drain (`:98-101`)
  now holds by construction: the signal's context cannot end a component's run context.
- **Superseded for this module.** Pin ADR-094's "Terminal shutdown remains Start-owned" (`:44`) and its rule processor
  work that "descend[s] from the Rule processor's Start context" (`:65-66`); its other "Start-owned" work (`:91`,
  `:101`, `:107`) reads as run-context work. The `LifecycleComponent` doc comment's Stop rule
  (`component/lifecycle.go:49-53`), since PR #273 the plain-words form of the carried sentence #282 quotes ("Start
  accepts the context that owns continuing work"). ADR-108 decision 3's "controlled and abort Stop"
  (`docs/adr/108-processor-shell.md:75`), which now means this record's rules. The same carried rule is spelled in the
  `lifecycle-suite` spec (`openspec/specs/lifecycle-suite/spec.md:19-20`, `:87-88`), in `docs/setup-plan.md:293-294`,
  in the `lifecycletest` row of `docs/admission-ledger.yaml:176-177`, in `pkg/errs/shutdown.go:17` ("Start-owned
  runtime completion"), in `processor/graph-ingest/keyed_ingest.go:220-221` ("when Start's context ends"), and in the
  context rules of the developer (`:215-218`, `:235-237`), architect (`:401-404`) and reviewer (`:237-241`) contracts
  under `.agents/contracts/`; the implementing change rewrites each to this contract.

## Related

Pin ADR-094 and ADR-095 (lifecycle) · ADR-108 (the processor shell; decision 3) · ADR-112 (natsclient's `Connect`
honours its context) · `openspec/specs/lifecycle-suite/spec.md` · `openspec/specs/background-work/spec.md` · #282
comments 6225878599 and 6249934379 · #131 comment 6224282566 · #9 comment 5994720412 item 2 · #196 (the guard tidy, PR 1
of the order) · #291 (graph-ingest `Start` during a reconnect, PR 3) · #109 (the processor shell)
