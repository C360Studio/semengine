# graph-query

The component that answers the graph's read requests on `graph.query.*`.

## Overview

`graph-query` sits in front of graph-ingest, which holds entities, and graph-index, which holds the relationship and
name indexes. It keeps no state of its own and writes nothing.

`graph.QueryVerbs()` (`graph/query_verbs.go`) decides what it serves: at start it subscribes a handler to each verb
declared for `graph-query`, and fails to start if a declared verb has no handler. Any other `graph.query.*` subject is
left alone, so a consumer can serve its own verb there.

## Verbs

| Verb on `graph.query.` | Answers from | Reply |
| --- | --- | --- |
| `entity`, `batch`, `prefix` | `graph.ingest.query.` same name | bare, as graph-ingest sends it |
| `byName` | `graph.index.query.byName` | enveloped, as graph-index sends it |

These four are forwards: the request and reply bytes pass unchanged, neither decoded nor checked, so `min_revision`
reaches graph-index on `byName` and an authority read on the other three ignores it.

A refusal from the verb behind a forward reaches the caller with its class, code, detail and message. A request that
gets no answer is refused by `graph-query` itself, class transient, code `internal`, naming the subject: nothing
responding, `query_timeout_ms` passing, the handler's context ending (`natsclient`'s limit on a request handler, or the
client closing), or a lost connection.

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
| `query_timeout_ms` | int | 15000 | The bound on each request `graph-query` makes to graph-ingest or graph-index, in milliseconds. `0` means the default. Negative is refused, and so is anything above 9223372036854, which does not fit a Go duration. |

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

The one input is required and has to be this port: a different name, subject, kind or interface, a `timeout` other
than `1s` (the value it takes when none is written) or `retries`, which nothing would read, a second input, or any
output is refused. Its `description` is yours to set, and so is `external`, which says requests come from outside the
composition.

## Health

`Health` is running or stopped, from `component.Lifecycle`. It keeps no state that could leave it degraded, and
`ErrorCount` never lowers `Healthy`.

`DataFlow` counts every request a forward receives, with its bytes; the reply's bytes are not counted. `ErrorCount`, and
`DataFlow`'s `ErrorRate` with it, counts the requests that got no answer, the ones `graph-query` refuses itself, so an
outage of graph-ingest or graph-index shows here. A refusal the responder sent is an answer and is not counted: "no such
entity" is ordinary traffic.

## Starting and stopping

`Start` needs a NATS client, and its context bounds the subscribing only. `Stop`
drains each verb's subscription, and returns once every running handler has returned. If its context ends first, it
returns that error and keeps the rest for the next `Stop`. If a failed `Start` cannot clean up after itself, `Start`
reports both errors and a later `Stop` finishes the job.

## Dependencies

- Registered by a composition root through `Register`; importing the package registers nothing.
- Consumers read `graph.QueryVerbs()` for subjects and spell none as a literal.
