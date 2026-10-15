# Proposal: simplify-04a-06-projection

## Why

Issue #138 (epic #223, PR 9a; owner split of PR 9 into `pkg/projection` then `pkg/lifecycle`, #223 comment
6200037173). `pkg/projection` has five exported interfaces nobody reads, a contract clone copied in `payloadregistry`,
a commit state spelled twice (on the receipt and on the error), dead branches inside the client, and exported methods
that do not say how they fail. Most important: its error-to-commit-state mapping is unexported. `pkg/lifecycle` therefore
keeps its own copy, which disagrees with projection's (#138 item 4) and reports a possibly-stored write as retryable
(#257). 9b fixes lifecycle by calling the mapping this change exports.

## What Changes

- Export one mapping, `projection.ClassifyMutationError(operation, err) *MutationError`, from the error of a graph
  mutation request to its commit state, kind, class, code and detail. Every `MutationClient` method uses it.
- `MutationReceipt` loses `Commit`: a receipt is returned only on success, and a failure's commit state is on the
  `*MutationError`.
- Remove `EntityCreator`, `PredicateReconciler`, `TripleAppender`, `EntityDeleter`, `AuthoritativeReader`, and the two
  revive directives for a rule the repo does not enable.
- Remove the client's dead layers: the unchanged `metadata` return values, the `Create` entity check graphmutation
  already makes, `Append`'s unreachable `default:`, the sort in `cloneDetail`, and the `reflect` nil check. A nil NATS
  client is refused with one error shape.
- `contract.Contract.Clone` replaces both copies of the contract clone (Q2).
- Doc comments on every exported method say which `Commit` and `Kind` each failure carries.
- Kept: the `pkg/projection` aliases of the contract types (Q1).

## Capabilities

- Modified: `projection-mutation`.

## Impact

- `pkg/projection/mutation_client.go`, `mutation_types.go`, `contract/contract.go`, and their tests.
- `payloadregistry/registry.go` (Q2).
- `processor/graph-ingest/commit_ambiguity_integration_test.go` (it reads `receipt.Commit`).
- `docs/admission-ledger.yaml` (`pkg/projection` contract; `pkg/projection/contract` disposition, Q2).
- Exported surface: two symbols added (`ClassifyMutationError`, `Contract.Clone`); five interfaces and one field removed.
  Consumers measured: none reads what is removed.
- Closes #138. Addresses #223. 9b (#154, #257) adopts D1.
