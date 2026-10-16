# Tasks: simplify-04a-11-graphmutation

Each task names its outcome and the gate that proves it. Tests come first. Record each one failing on the base commit
in `notes/<task>.md`; a compile failure on a missing symbol counts as failing.

## 1. Design

- [x] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal, design and spec delta pass pre-owner review and are accepted
  by the owner (no owner question; built on the owner's 2026-10-16 instruction and split, #223 comment 6216623583;
  review LOW findings 1–7 folded into tasks 2.x–4.1). Gate: `task spec:check` and `task docs:check`.

## 2. Failing tests

- [x] 2.1 `TestClientReportsRefusalBeforeSendingAsUnavailable`: four operations × `ErrNotConnected`/`ErrCircuitOpen`
  on the fake, plus a never-connected `natsclient.Client`; outcome `unavailable`, `IsDefinitelyNotCommitted` true,
  `errors.Is` reaches the sentinel (D1). Gate: fails on the base commit.
- [x] 2.2 `pkg/projection/commit_ambiguity_test.go`: rows "not connected" and "circuit open" → not committed,
  unavailable, transient, for every write (D1). Gate: fails on the base commit.
- [x] 2.3 `TestClientRefusesNilContext` asserts invalid with code `invalid_request`; new
  `TestClientRefusesUnencodableAndEntitylessRequests` (NaN `Object` on Append, nil `Entity` on Create; no request
  sent) (D2). Gate: fails on the base commit.
- [x] 2.4 Callers use `graphmutation.Subject(op)`: `protocol_test.go`, `mutation_port_contract_test.go`,
  `pkg/lifecycle/manager_integration_test.go:101`, `:155`, `:178` (D3). Gate: fails to compile on the base commit.

## 3. Implementation

- [x] 3.1 `request` maps the two natsclient sentinels to `TransportUnavailable`; doc comments of `TransportUnavailable`
  and `IsDefinitelyNotCommitted` updated (D1, #268). Gate: 2.1, 2.2.
- [x] 3.2 The client's own refusals return `errs.ClassifiedCode(errs.ErrorInvalid, graph.ErrorCodeInvalidRequest, …)`;
  Create checks `Entity` before sending (D2, #268, #137 item a). Gate: 2.3; existing client tests pass.
- [x] 3.3 `Subject(op)` replaces `ResolveSubject`; `TestResolveSubjectRejectsNonCanonicalFamily` and the family loop
  removed; graph-ingest `requireMutationProviderPort` and one route table (D3, #137). Gate: 2.4; the three
  graph-ingest route and port tests pass unedited.
- [x] 3.4 Flip `pkg/projection/classify_mutation_error_test.go:74-78` to `TransportUnavailable` → not committed,
  unavailable, transient; drop the "until #268" text there and at `pkg/projection/mutation_client.go:419-422` (D1, D4).
  Gate: `task test:unit`.

## 4. Documentation

- [x] 4.1 `docs/admission-ledger.yaml`: the `internal/graphmutation` contract states D1–D3; the projection
  `known_risks` #268 sentence removed. `docs/contract.md`: a write the client refused before sending is not
  committed (D4). Gate: `task ledger:check`, `task docs:check`.

## 5. Verify, review and close

- [ ] 5.1 `task verify` passes on a clean tree at the implementation head; `internal/graphmutation` holds its
  coverage floor.
- [ ] 5.2 Cross-agent review of record on the implementation commit; `reviewed-by:` in the PR body.
- [ ] 5.3 Archive the change (`openspec archive simplify-04a-11-graphmutation`) as the last content commit. Gate:
  `task spec:check`.
