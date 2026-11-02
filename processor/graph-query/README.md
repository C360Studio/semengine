# graph-query

The component that answers the graph's read requests on `graph.query.*`.

## Overview

`graph-query` sits in front of graph-ingest, which holds entities, and graph-index, which holds the relationship and
name indexes. It keeps no state of its own and writes nothing.

`graph.QueryVerbs()` (`graph/query_verbs.go`) decides what it serves: at start it subscribes a handler to each verb
declared for `graph-query`, and fails to start if a declared verb has no handler. The table declares no `graph-query`
verb yet, so today a started `graph-query` answers nothing. A request to a `graph.query.*` subject gets no responder.

## Configuration

An empty configuration is enough. This is the same configuration written out:

```json
{
  "type": "processor",
  "name": "graph-query",
  "enabled": true,
  "config": {
    "ports": {
      "inputs": [
        {
          "name": "graph_queries",
          "required": true,
          "config": {
            "kind": "nats-request",
            "subject": "graph.query.*",
            "interface": {"type": "graph.query", "version": "v1"}
          }
        }
      ]
    },
    "query_timeout_ms": 15000
  }
}
```

| Option | Type | Default | Description |
| --- | --- | --- | --- |
| `ports` | object | the port above | A `ports` block that is present is used as written; one that is absent takes the default |
| `query_timeout_ms` | int | 15000 | The bound on each request `graph-query` makes to graph-ingest or graph-index, in milliseconds. `0` means the default. Negative is refused. No handler makes such a request yet |

The default is above graph-ingest's 10 second budget on `batch` and `prefix`, so the slowest of those can finish. A
larger value gains little past `natsclient`'s limit on a request handler, 30 seconds by default, which also ends the
work.

The configuration is decoded strictly: a key not listed here is refused at construction, naming it. In the SemStreams
version `query_timeout` took a number of nanoseconds, and keys nothing read, such as `max_traversal_depth`, were
accepted and dropped. A configuration that still sets one of them fails at boot until the line is changed or deleted.

## Ports

| Direction | Name | Kind | Subject |
| --- | --- | --- | --- |
| input | `graph_queries` | nats-request | `graph.query.*` |

The one input is required and has to be this port: a different name, subject, kind or interface, a `timeout` or
`retries` that nothing would read, a second input, or any output is refused. Its `description` is yours to set, and
so is `external`, which says requests come from outside the composition.

## Health

`Health` is running or stopped, from `component.Lifecycle`. It keeps no state that could leave it degraded.

## Starting and stopping

`Start` needs a NATS client, and its context bounds the subscribing only. `Stop`
drains each verb's subscription, and returns once every running handler has returned. If its context ends first, it
returns that error and keeps the rest for the next `Stop`. If a failed `Start` cannot clean up after itself, `Start`
reports both errors and a later `Stop` finishes the job.

## Dependencies

- Registered by a composition root through `Register`; importing the package registers nothing.
- Consumers read `graph.QueryVerbs()` for subjects and spell none as a literal.
