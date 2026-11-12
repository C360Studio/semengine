# Consumer migration: `pkg/fusion`

For the authors of SemSource's code-context component and its fusion lenses, who move from SemStreams' `pkg/fusion`
to SemEngine's. It says what you change at your cutover and what you can leave alone. The port breaks old shapes where
that gives a better result (owner, 2026-11-02). Nothing here was run against your code: it comes from reading the
SemEngine package and the SemStreams pin `8b99efe9`. Your call sites at SemSource `8737c7ea` are
`processor/code-context/component.go` (`NewEngine` with `WithSignals` and `WithMetrics`, `NewBodyResolver`,
`MapStoreResolver`, `fusionnats.New`), `processor/code-context/exact_seed.go` (a `RetrievalClient` wrapper),
`source/fusion/fusiontest/memgraph.go` (a `RetrievalClient`), and the test doubles in
`processor/code-context/http_error_test.go`, `processor/code-context/scope_test.go` and
`internal/governance/go_callgraph_impact_integration_test.go`.

Carried unchanged: the `Lens` interface and `ResolveMode*`, `Request` and its wants, `Budget`, `Node`, `Ref`, `Miss`,
`Unhydrated`, `BodyResolver` and `MapStoreResolver`, the facet caps, and ranking. Not carried: the SemStreams
sub-query engine (`engine.go`, `subquery.go`, `evidence.go`, `client.go`) and `lensregistry`.

## Changes that stop your build (loud)

Each of these is a compile error. Fix them first.

- **`NewEngine` takes a logger and options and returns an error.** It was
  `NewEngine(graph, body) *Engine` followed by `.WithSignals(s).WithMetrics(r)`. It is now
  `NewEngine(graph, body, logger, opts...) (*Engine, error)`, with `WithSignals(s)` and `WithMetrics(r)` as options.
  A nil client or a nil logger is refused with an error; a nil body resolver is still allowed. A registry that already
  holds a different collector under a counter's name makes `NewEngine` return that refusal and no engine. Nothing
  changes an engine after it is built. *Do:*

  ```go
  engine, err := fusion.NewEngine(client, fusion.NewBodyResolver(c.storeRegistry), c.logger,
      fusion.WithSignals(fusionvocab.New(cco.Parents)),
      fusion.WithMetrics(c.metrics),
  )
  ```

- **`Fuse` and `BodyResolver.ResolveBody` refuse a nil context** with an error. The pin did not check.
- **`RetrievalClient` has no `Status`, and `Resolve` returns a `Resolution`.** `IndexStatus`, `IndexState` and its
  constants, and `ErrReadinessUnknown` are gone. `Resolve(ctx, ResolveQuery)` returns
  `Resolution{Seeds, UnreadableRecords, FailedRecords}` where it returned `[]Seed`. *Do:* in `fusiontest.MemGraph`
  delete `Status`, and wrap the seeds: `return fusion.Resolution{Seeds: seeds}, nil`. A client over the graph's
  semantic search copies the reply's `unreadable_records` and `failed_records` into the two counts; any other
  resolve leaves them 0. A wrapper such as `exact_seed.go` returns the inner client's `Resolution` unchanged, never a
  rebuilt one, or the two counts are lost.
- **`fusionnats.New` returns an error, and the client has no `Status` and no `Close`.** It was
  `fusionnats.New(nats, timeout) *Client`, which accepted a nil client and bound a readiness watch on its first
  `Status` call. It is now `New(requester, timeout) (*Client, error)`: a nil requester and a negative timeout are
  refused. A zero timeout is passed to the requester, which for `*natsclient.Client` means its
  `DefaultRequestTimeout` (5 s, the pin's own default). The client starts nothing and watches no key, so there is
  nothing to close. *Do:*

  ```go
  graph, err := fusionnats.New(deps.NATSClient, 0)
  if err != nil {
      return err
  }
  ```

  and delete any `Status` call (`internal/governance/fusion_gateway_integration_test.go` has one). SemSource calls no
  `Close`.
- **`fusionvocab.New` takes the ontology's ancestor function.** `fusionvocab.New()` imported the BFO/CCO hierarchy; the
  hierarchy now lives in semvocab, which depends on SemEngine, so SemEngine cannot import it. *Do:* import
  semvocab's `catalog/cco` yourself (code-context already does) and call `fusionvocab.New(cco.Parents)`. The score is
  the number of ancestors `parents` returns for the class, as before. `New(nil)` is allowed and scores every class 0,
  so ranking then runs as if no ontology were attached.

## Changes that do not stop your build (silent)

Read these. Nothing fails to compile, and a consumer that reads the JSON can be wrong without an error.

- **The wire is `contract_version` `"2"`.** Check it before you rely on anything below.
- **`index` is gone from the response.** The pin sent the graph index's readiness record there. Read `deferred`
  (and `defer_reason`) to learn whether you got an answer. The engine asks for no status; a read that replies
  `index_not_ready` is what defers. A reader of `index.ready` now sees a missing key.
- **`view_revision` is gone from the `graph` facet.** It came from the status read that no longer happens, and the
  facet's reads were never one snapshot (the facet's doc says so). *Do:* stop reading it.
- **Four `defer_reason` values are gone: `hard_stop`, `status_unknown`, `bootstrap_incomplete`, `unrecognized_state`.**
  They came from the readiness gate. `defer_reason` is now empty, `index_not_ready` or `all_seeds_unhydrated`. Code
  that switches on the old values never matches; treat any non-empty value as "no answer yet, try again".
- **A graph reset is an error, not a deferral.** At the pin a reset deferred as `hard_stop`. Now a fatal error such as
  `graph_state_reset_required` from `Resolve` or `Entities` is returned by `Fuse` as the error and no response. The
  same fatal error from a read after the seeds (relations, a relation target, paths, impact, graph) does not fail the
  call: that read is skipped and the answer is labelled `relations_truncated`, `paths_truncated`, `impact.truncated`
  or `graph.truncated`. The reason: graph-ingest refuses a *poisoned* entity (one whose stored value it refuses to
  read) with that same fatal error, and a reply cannot tell it from a reset. *Do:* handle the `Fuse` error in
  code-context's error path; do not parse its text.
- **A poisoned seed is listed, not fatal, over `fusionnats`.** graph-ingest refuses the whole batch when one entity in
  it is poisoned. `fusionnats.Client.Entities` then reads each requested ID alone: the poisoned seed is listed in
  `unhydrated` with reason `error`, the other seeds answer, and if every seed is poisoned the response is `deferred`
  with `all_seeds_unhydrated`. A seed not found when read alone is listed `not_found`; any other error from those
  reads, including an ended context, still fails the call. A `prefix`
  resolve whose page holds a poisoned entity still fails the call (#578). *Do:* read `unhydrated` on an
  answered response, not only on a deferred one.
- **A relations read that replies `index_not_ready` still defers the whole response**, nodes already built included, as
  at the pin. Relation targets, paths, impact and graph reads do not: they are skipped and labelled.
- **A node's refs are listed by predicate, then target.** Every edge list is sorted and de-duplicated before the cap
  of 12 per role, so the same graph gives the same refs whatever order the reply used. The pin kept reply order, so
  the 12 kept (and the order within a role) can change when a role has more than 12.
- **Graph facts and evidence list in key order.** Facts are sorted by predicate, datatype, then value before the 64-fact
  cap; each fact's or edge's evidence is sorted by source, timestamp, confidence, then context, with repeats dropped,
  before the cap of 8. Pin output followed the order the triples were stored in.
- **New labels, all omitted when zero or false.** `unreadable_records` and `failed_records` on the response (only
  after a word search, `nl` resolve; omitted at 0 and never set for symbol or prefix resolves; they also ride on a
  miss and on a deferral that follows the search's answer); `relations_truncated` on a node; `paths_truncated` on the
  response. Where the pin sent a short answer, a truncated flag may now be set; an answer whose search could not read
  records is no longer the same as an answer that read them all. A path now ends, marked `truncated`, at the last node
  it could read and never names an unread node.
- **`impact.truncated` and `graph.truncated` are set in more cases:** a skipped read for an affected node's file
  (impact), and an incoming counterpart whose read failed (graph). A counterpart that reads as absent keeps its edge
  without evidence and does not set `graph.truncated`.
- **Two counters, on the injected registry only.**
  `semengine_fusion_skipped_reads_total{facet}` (`relations`, `paths`, `impact`, `graph`) is new, and each `Fuse` call
  logs one Warn line per facet that skipped a read (count and first error). The body counter is now
  `semengine_fusion_body_hydration_failures_total{reason}`; the pin's was
  `semstreams_fusion_body_hydration_failures_total`, and it fell back to Prometheus' default registerer when no
  registry was given. Now an engine built without `WithMetrics` has no counters at all, and two engines built on one
  registry share one series. A body failure also logs a Warn line.
  *Do:* update dashboards and alerts to the new names, and pass `WithMetrics` if you want them.

## What you can leave alone

- The `Lens` you wrote, its `ResolveMode` values and `Hydrate` handles (ADR-063's contract is unchanged, and
  `*storeregistry.Registry` is checked at compile time to be a `StoreResolver`).
- `Request`, `Budget`, `Want*`, `Scope`, `IncludeScores`, and the node, path, impact and graph shapes, apart from the
  fields named above.
- `WithSignals` and the predicate-weight half of `fusionvocab` (read from `vocabulary`'s registry).

A deployment with no graph-embedding sets `"embedding_enabled": false` in graph-query's config. A word question
(`nl` lens) is then refused invalid `not_enabled`, naming `semantic`, instead of a transient `internal` refusal a
retrying caller would repeat without success (#579). Left at its default, true, the old refusal stays. Symbol and
prefix lenses answer either way.

## Still open

- A `prefix` resolve whose page holds a poisoned entity fails the call (#578).
