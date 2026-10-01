# What SemEngine guarantees

SemEngine is a Go framework for building applications around a knowledge graph stored in NATS: you send it facts
about things, it stores and indexes them, answers queries, and runs rules and step-by-step workflows over them. Its
core needs no outside service, so it runs offline and on small edge machines. The owner describes it as "TrustGraph
and Temporal had a tiny baby — a pragmatic, Go-idiomatic, NATS-based, offline-first and edge-capable baby."

This page lists what the engine promises at its base level (tier 0, explained below): how writes report their
outcome, what survives a restart, how undeliverable messages stay visible, how rules and workflows behave, and how you
watch changes and operate it. It also says what is left to your application. The reasoning and the test for each
promise are in the [design record for this contract](../openspec/changes/archive/2026-10-01-setup-03b-contract-boundary/design.md).

**Status:** no engine code is in this repository yet. This page states what the first code release (called slice 04A)
must implement; each promise below becomes a test when that code lands. Until then, the code it describes lives in
[SemStreams](https://github.com/C360Studio/semstreams) at commit `8b99efe9`, and SemEngine is being built by copying
the needed parts out of it.

A few words used throughout:

- **Entity:** one thing in the graph (a file, a sensor, a person), with an ID and a set of facts about it.
- **Fact (triple):** one statement about an entity: subject, predicate, object, plus where it came from and when.
- **Consumer:** an application built on SemEngine. The first four are semsource, semconnect, semboids and semteams.
- **NATS stream:** a NATS JetStream log of messages. **KV bucket:** a NATS key-value store.
- **Provider:** an outside service the engine calls, such as an embedding model or a large language model (LLM).

## Tiers: what runs where, and what degrades

A tier is a level of capability. A deployment runs at the highest tier whose providers are reachable. If a provider
goes away, the deployment drops to the tier below and keeps working.

| Tier | Needs | Adds | If the provider is lost |
| --- | --- | --- | --- |
| 0 | NATS only | the whole graph, keyword search (BM25, computed in process), rules, workflows, clustering, change feeds, the operator endpoints | — (nothing outside NATS to lose) |
| 1 | an embedding service | search by meaning ("neural" search) | drops to tier 0; keyword search still answers |
| 2 | an LLM service | nothing yet: reserved for summaries, review, query classification, and agent features | drops to tier 1 or 0 |

The rest of this page is about tier 0, because that is what every deployment gets.

## What the engine guarantees at tier 0

### Identity and authority

- Every entity ID follows one fixed grammar, and every deployment has its own **authority**: an
  `org.platform` prefix that says which deployment owns an entity.
- The engine refuses a write that would create an entity under another deployment's authority. The error code is
  `authority_foreign`.
- An entity owned elsewhere can enter only through an **import lane** that an operator declares. It is stored as a
  read-only copy. You can always refer to a foreign entity by its ID without copying it.
- Each deployment's minted platform ID is unique.

### Writes and their outcomes

- Writes go through a typed client with three operations: create, reconcile (set an entity's facts to a given
  state), and delete.
- A reconcile can carry the **revision** you last read. It applies only if the entity is still at that revision.
  Otherwise you get a `revision-conflict` error that names both revisions, and nothing changes.
- Every write reports one of three outcomes:
  - **committed:** the change is stored;
  - **not committed:** it was rejected before anything was stored (invalid request, revision conflict, entity not
    found);
  - **commit unknown:** the engine cannot tell whether it was stored. A storage timeout is the usual cause.
- On *commit unknown*, re-read the entity and compare before you retry. The engine does not offer a lookup of past
  outcomes by request ID.
- "The stream accepted my message" is not the same as "my data is stored". Data is stored once the entity's state is
  in the `ENTITY_STATES` KV bucket and its content is in the object store.

### Recovery after a restart

What happens to messages in flight depends on how your input reaches the engine.

| Input path | After a broker or process restart | Your part |
| --- | --- | --- |
| NATS stream in memory | accepted messages that were not yet applied are lost; stored entities survive | re-publish from your source (semsource does this) |
| NATS stream on disk | unacknowledged messages are delivered again; the final state is the same as if each was applied once | nothing |
| request/reply (no stream) | you have the reply, or a *commit unknown* | handle *commit unknown* as above |

If a stream is deleted and recreated, re-sending your current data applies it. Old sequence numbers do not cause it
to be skipped.

### Delivery and parked input

- The ingest component acknowledges a message only after its effect is stored and its duplicate-detection mark is
  saved. A message being processed for a long time signals progress, so the broker does not hand it out again.
- A message that cannot be decoded is acknowledged, dropped and counted, so that it does not block the queue.
- After 3 failed deliveries, the broker stops retrying. Three is the engine's own default for a JetStream input
  (`MaxDeliver`); a plain NATS consumer retries without limit unless you set it. The engine records each such
  **parked** message in the `MAX_DELIVERY_EVENTS` stream (kept on disk for 7 days). It reports each one as a record
  and as a metric, so a parked message is visible rather than silently gone.

### Rules and entity workflows

- **Rules:** a rule watches for entity changes that match a condition and then runs actions. The actions are
  `publish`, `add_triple`, `remove_triple`, `update_triple`, `reconcile_predicates` and `update_kv`, plus the three
  workflow actions below. Rules are loaded from JSON files called rule packs, and a pack's ID is validated when it is
  loaded. A pack that names an action type the engine does not know stops the program at startup, with the type and
  the file named, rather than failing later when the rule fires.
- **Entity workflows:** an entity can follow a declared set of phases, for example `active → culled | expired`.
  - The engine checks each move against the allowed transitions and stores the phase as facts on the entity itself,
    so a workflow survives a restart because the entity does.
  - Updates are checked against the entity's revision, so two concurrent moves cannot both win.
  - Rules move entities between phases with `lifecycle_transition`, `lifecycle_complete` and `lifecycle_fail`.
- Not in this release:
  - durable execution beyond phases: a record of each attempt with one owner for its final outcome, a guard that
    stops an outside call from running twice after a restart, typed reasons an attempt is blocked and may be
    retried, and replay of unfinished work at startup. They are planned as their own piece of work
    ([#24](https://github.com/C360Studio/semengine/issues/24)).

### Observing changes

- The graph's current state is the `ENTITY_STATES` KV bucket, and clusters are in `COMMUNITY_INDEX`. To follow
  changes, watch those buckets. A watcher started later first receives every current value, then each change.
- The engine provides a view helper (`pkg/graphview`) that gives you a snapshot followed by a stream of changes.
- For browsers and other remote clients, the websocket output component streams changes to stored state. It does
  not relay raw input. In SemStreams (commit `8b99efe9`) it reads a NATS subject; copying it into SemEngine changes
  that.
- For operators, the service's HTTP endpoint `GET {prefix}kv/{bucket}/watch` streams the same changes as server-sent
  events.

### Operator endpoints and metrics

Each running engine serves the following. `{prefix}` is the path the component manager is mounted under.

| What | Where |
| --- | --- |
| liveness and readiness | `/health`, `/healthz`, `/readyz` (ready only once every component the program started is ready) |
| services and their health | `/services`, `/services/health` |
| API description | `/openapi.json`, with a browsable version at `/docs` |
| component health, list, types, status, config | `{prefix}health`, `{prefix}list`, `{prefix}types` and `{prefix}types/{id}`, `{prefix}status/{name}`, `{prefix}config/{name}` |
| how the running components are wired, as a graph | `{prefix}flowgraph`, `{prefix}validate`, `{prefix}paths` |
| message trace by ID, KV query and watch | the message logger's HTTP endpoints |
| storage report | the storage observability endpoint |
| metrics | a Prometheus endpoint |

The `flowgraph` response and the metric names are part of the contract. Slice 04A will check in a dashboard
definition that uses the metric names, and a test that fails if one disappears. To see how a program's components are
wired without any UI, render it as a Mermaid diagram with the composition command-line tool (`composition/cli`).

## What is your job as a consumer

- **Register what you use.** The engine has no list of "all components". Your `main` registers each component it
  wires into the program's startup, and each payload type it uses (a payload type is the Go type of a message body,
  which the engine looks up by name to decode), so your program builds only what it names. The sketch below is
  illustrative: the package paths and signatures are SemStreams' at commit `8b99efe9` (with the module path swapped,
  it compiles there), and SemEngine's are fixed when each package is copied over in slice 04A.

```go
package main

import (
    "log"

    "github.com/c360studio/semengine/component"
    "github.com/c360studio/semengine/message"
    "github.com/c360studio/semengine/payloadregistry"
    graphindex "github.com/c360studio/semengine/processor/graph-index"
    graphingest "github.com/c360studio/semengine/processor/graph-ingest"
    graphquery "github.com/c360studio/semengine/processor/graph-query"
    "github.com/c360studio/semengine/storage/objectstore"
)

func main() {
    components := component.NewRegistry()
    for _, register := range []func(*component.Registry) error{
        graphingest.Register,
        graphindex.Register,
        graphquery.Register,
        objectstore.Register,
    } {
        if err := register(components); err != nil {
            log.Fatal(err)
        }
    }

    payloads := payloadregistry.New()
    for _, register := range []func(*payloadregistry.Registry) error{
        message.RegisterPayloads,
        objectstore.RegisterPayloads,
    } {
        if err := register(payloads); err != nil {
            log.Fatal(err)
        }
    }
    // Build the service and component managers from these registries and start them.
}
```

- **Own your public interfaces.** HTTP APIs, MCP (Model Context Protocol) tools for AI agents, and any other front door
  are yours (semsource's MCP gateway, semconnect's `cs-api`).
- **Own your UI.** UI work for SemEngine-based apps lives in `semteams/ui`, using only the documented endpoints above.
- **Own your domain.** Vocabularies, query lenses (how a search in your field is resolved and ranked), prompts and
  policy for your field stay in your repository.

## What is deliberately not in the engine

| Left out | Why |
| --- | --- |
| Agent features (tool calling, approvals, governance) | They need an LLM, so they belong to tier 2, which is empty for now. semteams brings them when it moves over. |
| The SemStreams graph gateway | Each consumer already has its own public interface, so a shared one would be a second front door to maintain. |
| LLM features (community summaries, answer generation, review) | Tier 2 is reserved and empty at the first release; tier 0 must run with no provider. |
| A SemEngine UI | The engine serves data and endpoints; drawing them is an application's job. A shared component is extracted only when a second app wants the same view. |

## Where to look

- The full contract, with each promise's test and the reasoning behind it: the
  [design record for this contract](../openspec/changes/archive/2026-10-01-setup-03b-contract-boundary/design.md).
- Which SemStreams package is copied, adapted or left out, and why: the
  [admission ledger](admission-ledger.yaml).
- Which other repositories may be read, and for what: [inventory scope](inventory-scope.md).
- The plan and the order of work: [setup plan](setup-plan.md).
- The four starter consumers: [semsource](https://github.com/C360Studio/semsource),
  [semconnect](https://github.com/C360Studio/semconnect), [semboids](https://github.com/C360Studio/semboids),
  [semteams](https://github.com/C360Studio/semteams).
