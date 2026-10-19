# flowgraph

Builds a graph of how components connect through their declared ports, and reports what the graph shows: which
components are connected, which have no connections, and which ports nothing matches.

```go
import "github.com/c360studio/semengine/internal/flowgraph"
```

The package is internal to SemEngine. In SemStreams the `composition` package is its one reader; `composition` comes
to SemEngine in a later change. Until then this package's tests are its only callers.

## Building a graph

`BuildFromDeclarations` is the one way to build a graph. It takes the `component.Declaration` of every component
instance: the port shape the component `Registry` records when it admits an instance, or derives offline from a
configuration (`Registry.Declare`). It adds one node per instance and connects the ports.

```go
graph, err := flowgraph.BuildFromDeclarations(declarations)
if err != nil {
    return fmt.Errorf("build flow graph: %w", err)
}
```

It returns an error, and no graph, when the declarations cannot form a valid one:

- an empty or repeated instance name;
- a port with no connection identity (no subject, bucket or address);
- more than one writer port on one KV bucket;
- a graph-mutation port that is not a required request port on the graph-mutation subject family with the
  graph-mutation interface type and version, or graph-mutation ports with not exactly one input (the provider) among
  them.

## What the graph connects

Each port has an interaction pattern (`component.InteractionPattern`) that decides how it is matched. An edge records
the two ports, the pattern and the connection ID (subject, bucket or address) they share.

| Pattern | What joins |
| --- | --- |
| `stream` | An output joins every input on another component whose subject matches. A JetStream port also matches on each of its stream's subjects. |
| `request` | An output and an input on different components with matching subjects join. Two ports of one direction on different components join only when their subjects differ and still match (one holds a wildcard); on the same subject they do not. |
| `watch` | A watch output (a KV writer) joins every watch input on the same bucket, its own component's included. |
| `read` | A watch output joins every read input on the same bucket, its own component's included. |
| `store` | Every store output joins every store input on another component. |
| `network` | Never joined. Two binds on one network address are refused by `component.Registry` when it admits a component, not here. |
| `http-client` | Never joined. |
| `timer` | Never joined. |

Subject matching is this package's own rule: `*` stands for one token and `>` for one or more trailing tokens, and
either side of a pair may hold the wildcard. When one side holds a wildcard, the edge records the other side's
subject. `SubjectCovers(filter, pattern)` answers a different, one-way question: whether every subject `pattern` can
name also matches `filter`.

## What the analysis reports

```go
result := graph.AnalyzeConnectivity()
for _, orphan := range result.OrphanedPorts {
    log.Warn("orphaned port",
        "component", orphan.ComponentName,
        "port", orphan.PortName,
        "issue", orphan.Issue)
}
```

`AnalyzeConnectivity` returns a `FlowAnalysisResult`:

- `ConnectedComponents`: groups of components joined by edges. A component with no edge is a group of one.
- `DisconnectedNodes`: components with no edge.
- `OrphanedPorts`: ports no edge reaches. Network, http-client, store and timer ports are never orphans. Each orphan
  carries an `Issue`, one of the package's `Issue` constants, and the port's `Required` flag.

The analysis reports facts and no severity. Whether an orphan or a lone component is a problem is the reader's
judgment.

## JetStream inputs fed by plain NATS publishers

```go
for _, w := range graph.ValidateStreamRequirements() {
    log.Warn("JetStream input fed by plain NATS publishers",
        "subscriber", w.SubscriberComp,
        "port", w.SubscriberPort,
        "publishers", w.PublisherComps)
}
```

`ValidateStreamRequirements` returns one warning for each JetStream input port that a plain NATS output port feeds,
naming each publishing component once, sorted by subscriber component and then port.

## Concurrency

A `FlowGraph` has no locks. Build it and read it on one goroutine.
