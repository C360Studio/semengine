# ADR-115: Three HTTP Surfaces: the Front Door, Debug Tools, and Output Components

## Status

**Proposed (2026-11-11).** Drafted with the `alpha4-output-websocket` design (PR #592, `Closes #34`). It records these
owner rulings and adds no decision of its own; what it adds is the plain description, the alternatives and the
consequences:

- 2026-11-11, #34 comment 6797377552: HTTP surfaces have three roles; 03B D14 governs and pin ADR-090 decision 6 is
  not carried; application SSE, if ever wanted, is a second wire format on the output component; GraphQL
  subscriptions are out of scope.
- 2026-11-11, #34 comment 6797028887: ACME is deferred until a consumer needs it.
- The owner, same conversation: "message-logger was never meant for production - it was meant as debugging tool ...
  be careful about message-logger being just for people. it's a debug tool and tests and agents have relied on it in
  the past."

Tree cites are at `b7de1e0`; pin cites are at SemStreams `8b99efe9`.

## Context

A SemEngine process serves HTTP in several places, and until now nothing said which one an application should use for
what. The service manager's server answers health, readiness, component listings, OpenAPI and, with #588, GraphQL
(`service/service_manager_http.go:395-399`, `:248-250`). On routes that the service mounts beside them, the message
logger answers entries, traces and a server-sent-events (SSE) watch of any KV bucket
(`service/message_logger_http.go:40-45`), and the storage report answers its report
(`service/storage_observability_http.go:159`). The profiling server and the metrics server have their own listeners.

The SSE watch opens one NATS watch per HTTP client (`service/message_logger_kv_watch.go:138`). That is fine for a
person or a test looking at a bucket, and wrong for an application with many users: ADR-081 names it the trap. At the
pin, ADR-090 decision 6 said application reads go through the GraphQL gateway and "raw KV is an owner/operator seam,
not an application fallback". 03B D14 (2026-10-01) later made `output/websocket` the engine's transport for applied
state. This record says which rule wins and why.

## Decision

**1. The service HTTP server is the request/response front door.** An application asks it questions: GraphQL through
the gateway, health and readiness, component and flow listings, OpenAPI. It is gated by readiness, described in
OpenAPI, and the place authentication (#84) will attach. It does not stream data to applications.

**2. Debug and operator routes are debugging tools.** The message logger (including its SSE KV watch), pprof and the
storage report let people, tests and agents look inside a running system. They stay supported, and tests and agents
may rely on them. They are not an application's production contract, and they are not built for many concurrent
watchers: each SSE client opens its own watch.

**3. Output components are the application streaming plane.** Live data leaves a flow through an output component on
its own port. The component shares one upstream watch across its clients and sheds a slow client's data rather than
slowing the others. `output/websocket` is the first. Over applied state it streams the current state as it changes, not
a history: a client always ends at each entity's latest value, and may miss values in between (ADR-081's merge per
key). A consumer that needs every revision reads a stream with its own consumer, not an output component.

**What an app developer uses.** Ask the server; subscribe to an output. For "what is entity X now?" or "which entities
match?", call the gateway. For "keep me current as entities change", compose `output/websocket` with an applied-state
input and connect to its port. For a high-rate feed that never enters the graph (semboids' frames), compose it with a
NATS subject input. Do not build an application on the debug SSE watch.

**What this changes from the pin.** ADR-090 decision 6 is not carried for streaming. An application may stream applied
state through an output component (03B D14). Request/response reads still go through the gateway, as #588 applies it.

**Not decided here, and out of scope.** SSE for applications, if a consumer asks, becomes a second wire format on the
same output component, never an application route on the service server. GraphQL subscriptions are out of scope.
The `/metrics` endpoint is the monitoring contract (03B D15) and is not one of these roles.

## Alternatives rejected

- **Promote the debug SSE watch to the application contract.** It opens a watch per client, sits beside routes built
  for inspection, and would turn a debugging tool's shape into a compatibility promise.
- **Stream from the service server (GraphQL subscriptions or an app SSE route).** It puts long-lived connections on
  the readiness-gated front door, and needs a subscription runtime no consumer has asked for.
- **Keep ADR-090 decision 6 as written.** It leaves applications no supported way to observe change except polling,
  which D14 ruled against.

## Consequences

- An application's streaming contract is the output component's documented wire format and config, versioned with
  that component. Debug routes may change shape without a migration note to applications.
- Each output instance binds its own port and owns its own TLS through `internal/tlsutil`. ACME is deferred.
- The debug SSE watch keeps its per-client watch. Converting it (#477) is optional, since it is not built for many
  watchers.
- A future application transport (SSE, another socket protocol) is added as a wire format of an output component,
  not as a new server.

## Related

ADR-081 (one watch, many local readers); ADR-109 (readiness); ADR-113 (lifecycle context); 03B D14 and D15
(`openspec/changes/archive/2026-10-01-setup-03b-contract-boundary/design.md:609-645`); pin ADR-090 decision 6;
issues #34, #84, #477, #588.
