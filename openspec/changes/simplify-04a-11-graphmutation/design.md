# Design: simplify-04a-11-graphmutation

This design depends on `INVENTORY PASS` for `inventory.md`. Premises cite that file's sections.

## D1. A natsclient refusal before sending is `TransportUnavailable` (#268, HIGH)

`request` keeps its order: a classified reply, then no responders. It adds `errors.Is(err, natsclient.ErrNotConnected)`
and `errors.Is(err, natsclient.ErrCircuitOpen)` beside no responders, all three giving `TransportUnavailable`. Every
other error keeps `TransportCommitUnknown`. The premise is that natsclient returns both sentinels only before its one
send (inventory § 1: `request.go:258-264` against `:297`), and the only production requester is `*natsclient.Client`
(§ 4). Raw nats.go errors stay commit-unknown: when they happen relative to the send is not promised (§ 1). The doc
comments of `TransportUnavailable` and `IsDefinitelyNotCommitted` say "no request was sent, or no responder took it".

Options:

- (a) Reuse `TransportUnavailable`. Recommended: no new symbol, and `IsDefinitelyNotCommitted` and projection are
  unchanged.

- (b) A new `TransportNotSent` outcome. It adds an exported constant whose only reader is the package itself (shape 4).
- (c) A rule in projection. This is the second rule #268 rejects, and lifecycle would inherit it.
- (d) Do nothing. Reconnects and an open breaker keep quarantining writes that were never sent.

Observable: each of Create, Reconcile, Append and Delete returns a `*TransportError` with outcome `unavailable`, and
`errors.Is` reaches the sentinel. Through projection and lifecycle, the result is not committed,
`MutationUnavailable`, transient.

Tests:

- `TestClientReportsRefusalBeforeSendingAsUnavailable` (graphmutation): every operation × both sentinels on the fake,
  plus a real never-connected `natsclient.NewClient("nats://unused")` (pattern
  `natsclient/request_integration_test.go:128-136`; it binds no port).

- Two new rows in `pkg/projection/commit_ambiguity_test.go`'s table (`:176`), `natsclient.ErrNotConnected` and
  `ErrCircuitOpen` → `CommitNotCommitted`, `MutationUnavailable`. These fail on the base commit.

- The flip: `classify_mutation_error_test.go:74-78` becomes `transport(TransportUnavailable, …)` → not committed,
  unavailable, transient, under "Not delivered". Its "until #268" comment goes.

Lifecycle has no "until #268" row and needs no test edit for D1. ADR-091 `:69` is kept: this is not a lost reply after
possible delivery.

## D2. Client refusals are invalid, not committed (#268, #137 item a)

A nil client, a nil context, an operation `Subject` refuses, a `json.Marshal` failure, and a `Create` with a nil
`Entity` (checked before sending, removed from the reply check at `client.go:87`) return
`errs.ClassifiedCode(errs.ErrorInvalid, graph.ErrorCodeInvalidRequest, err)`. No request is made. Projection already
maps that code to not committed, `MutationInvalid`, class invalid (`isDefiniteFailure`, `mutation_client.go:503-507`).

Options:

- (a) Invalid with `invalid_request`. Recommended.
- (b) `TransportUnavailable`. It would make them transient, and retry loops would repeat a request that cannot succeed.
- (c) Leave them. #268 stays open.

The cost of (a): a projection-built refusal has no code, while this one carries `invalid_request`; both are invalid
and not committed.

Tests:

- `TestClientRefusesNilContext` asserts `errs.IsInvalid` and the code.
- A new `TestClientRefusesUnencodableAndEntitylessRequests` covers an Append whose `Object` is `math.NaN()` and a Create
  with no entity: invalid, code `invalid_request`, zero fake calls.

## D3. One subject function, one provider check (#137)

`Subject(op Operation) (string, error)` returns the prefix of `SubjectFamily` plus the operation for the four
admitted operations, and refuses any other. The family branch and `TestResolveSubjectRejectsNonCanonicalFamily` go.

The minimal graph-ingest edit:

- `canonicalMutationProvider` becomes `requireMutationProviderPort(ports *component.PortConfig) error`, with the same
  checks and messages.

- `component.go:312` calls it.
- `canonicalMutationRoutes` calls it, then fills subjects from `graphmutation.Subject` into one literal
  `[]canonicalMutationRoute` table.

The other test callers rename (inventory § 1); the family loop at `mutation_port_contract_test.go:39-43` goes.
Observable: routes, Validate refusals and the legacy-port refusal are unchanged.
`TestCanonicalMutationRoutesComeOnlyFromTypedProvider`,
`TestCanonicalMutationRoutesRejectLegacyCompanionPort` and `TestDefaultConfigDeclaresRequiredMutationProvider` pass
unedited. The family keeps its one home (`graph-transport-boundary` `:17`; #106).

## D4. Text follows the code

- The spec delta (requirement "One mapping…").
- The projection doc comment `mutation_client.go:419-422`.
- `docs/contract.md`: one sentence (a write the client refused before sending is not committed).
- The ledger: the `internal/graphmutation` contract names D1–D3; the projection `known_risks` sentence on #268 goes.

## Open issues and merge order

Issues #268 and #137 are retired (tasks 3.1–3.3). #106 stays (kept constraint). #132 and #223 name the
word. #275 and #273 edit other hunks of graph-ingest files. Either order works; the second to land merges `origin/main`.

## Owner questions

None. The fix is the one #268 names. No pin ADR decision changes.
