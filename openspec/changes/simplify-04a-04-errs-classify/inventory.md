# Inventory: simplify-04a-04-errs-classify

base: `db48e5b159a268930e5501f275c07f0141d212d4` (main `67d0ee9` plus the empty claim commit)

Question: which code reads the class `errs.Classify` gives an error that carries no class, and which code builds a
commit-unknown `projection.MutationError`. Pin facts are read from a tarball of SemStreams at `8b99efe9`
(`gh api repos/C360Studio/semstreams/tarball/8b99efe9`), never a local checkout. Repositories read for consumer use:
semsource, semteams, semboids, semconnect (`docs/inventory-scope.md`), at their local checkouts, named below.

An **unclassified error** is a non-nil error whose chain holds no `*errs.ClassifiedError` and none of the sentinels
that `errs.IsTransient`, `errs.IsFatal` or `errs.IsInvalid` list.

## 1. The claimed gap

- `pkg/errs/errs.go:234-235` — `// Default to transient for unknown errors to allow retry` / `return ErrorTransient`.
  Confirmed; the issue's `:279-280` is an older main (pin: `pkg/errs/errs.go:279`).
- `pkg/errs/errs.go:221` — `return ErrorTransient // Default for nil`. No production caller passes nil (each caller
  below guards `err != nil` first); out of scope.
- `pkg/lifecycle/graph_emit.go:110` — `Class: errs.Classify(err), Commit: projection.CommitUnknown, Err: err,`.
- `pkg/projection/mutation_client.go:467` —
  `Operation: operation, Kind: MutationUnavailable, Class: errs.Classify(err), Commit: commit, Err: err,`; and
  `:477` — `mapped.Class = classified.Class` (a classified reply keeps its class, commit-unknown included).
- Changing the default does not close the commit-unknown half. `IsTransient` follows `Unwrap` (`errs.go:159-171`), and
  `MutationError.Unwrap` returns `Err` (`pkg/projection/mutation_types.go:150-155`). A commit-unknown error wrapping
  `context.DeadlineExceeded` (a listed sentinel, `errs.go:170`) or a transient `internal` reply
  (`processor/graph-ingest/mutation_runtime.go:263-265`, `rejectInternal`) reads `IsTransient == true` whatever its
  `Class`. Real trigger, already in a test: `pkg/lifecycle/graph_emit_contract_test.go:69` —
  `{name: "deadline", err: context.DeadlineExceeded},`.

## 2. Every spelling of "the class of an error nobody classified"

| Site | Spelling | Today |
| --- | --- | --- |
| `pkg/errs/errs.go:234-235` | `Classify` default | transient |
| `natsclient/errors.go:326-327` | `classForHeader` `default:` | transient (unreachable today: `Classify` returns three values) |
| `natsclient/errors.go:366-371` | `classifiedFromHeader`, unknown wire class | invalid ("a caller seeing IsInvalid won't retry") |
| `pkg/projection/mutation_client.go:501` | `classifiedAppendFailure`, unknown class string | transient |
| `pkg/errs/errs.go:17` | `ErrorTransient ErrorClass = iota`: the zero value | a literal without `Class` reads transient |
| `processor/graph-ingest/keyed_ingest.go:304-315` | `IsInvalid`/`IsFatal` → Terminate, else Retry | retry; stream lane, effect fenced by the applied-sequence guard (ADR-111 Decision 3 allows it); no `Classify` |

Pin spellings: `processor/agentic-loop/component.go:1330-1336` ("Fatal is checked first and means one thing here: an
external effect may already have happened"); `processor/agentic-tools/write_todos.go:253`, `:263` (`errs.WrapFatal` on
commit-unknown); `agentic/agentrun/milestone_settlement.go:188-198` (`switch semerrs.Classify(err)`, `default:` at
`:193` → Retry at `:197`, "Unknown resolves to Transient (errs.Classify's default)"; package is `defer-exclude`,
`docs/tier1-cross-check.md:90`).

### `Classify` callers (`git grep -n 'Classify(' -- '*.go'`)

| Caller | Reads the class for | Fatal default | New Unknown class |
| --- | --- | --- | --- |
| `natsclient/errors.go:319` `classForHeader` (via `RespondError` `:173`, `SubscribeForRequests` `request.go:450`) | `X-Error-Class` header | unclassified handler error sent `fatal` | needs a fourth wire value; `default:` (`:326-327`) sends it `transient` unless changed |
| `pkg/lifecycle/graph_emit.go:110` | commit-unknown `MutationError.Class` | fatal for `nats.ErrTimeout`, decode errors; still transient via a wrapped deadline: override needed | same gap: override needed |
| `pkg/projection/mutation_client.go:467` | every unclassified `MutationError` | no-responders (not committed) turns fatal unless set; `graph_emit.go:105` already sets it transient | same, and every switch must learn Unknown |
| `pkg/errs/errs_test.go:128-136` | table | `:132`, `:136` flip; `:136` loses its #146 sensitivity | same |
| `natsclient/errors_test.go:37` | `unclassified_defaults_transient` | flips | flips |
| `natsclient/client_sentinels_test.go:25` | sentinels | unchanged | unchanged |
| `pkg/lifecycle/manager_test.go:1677`, `:1682`, `:1745` | reread class; contention class | `:1745` fails: `ErrUpdateRetriesExhausted` is a bare `errors.New` (`pkg/lifecycle/errors.go:122`), returned at `manager.go:453-454`, `:656-657`, `:768-769` | same |
| `pkg/lifecycle/graph_emit_contract_test.go:182` | classified refusal | unchanged | unchanged |

`MutationCommitUnknown` build sites: `pkg/lifecycle/graph_emit.go:108-111`; `pkg/projection/mutation_client.go:436-440`
(`unknownMutation`), `:471-473`. Production readers of `MutationError.Class`, `IsTransient` or `IsFatal` on it in this
repository: none (`git grep -n 'MutationError' -- '*.go' ':!*_test.go'`).

Tests that assert the class of a commit-unknown error (each flips under the change):

- `pkg/projection/commit_ambiguity_test.go:80-95`, `:109` — `mutationErr.Class != test.class` (the reply's class).
- `processor/graph-ingest/commit_ambiguity_integration_test.go:135` —
  `assert.Equal(t, errs.ErrorTransient, mutationErr.Class)`; `:137-139` — `errors.As` to `*errs.ClassifiedError`,
  "the reply's classification stays on the error", class transient, code `internal`.
- `processor/graph-ingest/commit_ambiguity_integration_test.go:249` —
  `assert.Equal(t, errs.ErrorInvalid, mutationErr.Class, ...)` (commit-unknown after an oversized response).
- `processor/graph-ingest/commit_ambiguity_integration_test.go:381-383` — append: `errors.As` to
  `*errs.ClassifiedError`, class transient, code `internal`.

These three integration tests (`TestIntegration_TypedClientPreservesCommitAmbiguity` `:74`,
`TestIntegration_OversizedResponseAfterCommitIsCommitUnknown` `:181`,
`TestIntegration_AppendAfterUnconfirmedWriteIsCommitUnknown` `:300`) run only under `-tags integration`.

Handler sites whose error may be unclassified (wire class changes from `transient` to `fatal`):
`processor/graph-ingest/canonical_mutations.go:398` — `return nil, fmt.Errorf("%v: %w", accountingErr, batchErr)`;
`processor/graph-ingest/query.go:354`, `:426`, `:429` (marshal failures, payload-fit invariant); `query.go:397` —
`return nil, err` (the `encodedSize` size-check error). The typed client already reads `:398` as commit-unknown (no
code, `isDefiniteFailure` false).

Wire readers in this repository that pass the reconstructed class on: `pkg/projection` (above);
`graph/exact_entity.go:73-75` — `response, err := r.requester.RequestClassified(...)` / `return nil, err`, the exact
reader `pkg/lifecycle` rereads through (`manager_test.go:1677-1682` pins that the reread's class reaches the caller). A
query handler's unclassified error now reaches a lifecycle reread as fatal.

### A second commit-unknown spelling: a defect

`pkg/lifecycle/graph_emit.go:98-99` — `return fmt.Errorf("%w: %s: %w", ErrEmitFailed, operation, err)` for any classified
reply whose code is not revision-mismatch, not-found or exists. graph-ingest's transient `internal` reply after an
unconfirmed write (`mutation_runtime.go:263-273`) arrives here and stays `IsTransient == true`, with no commit state.
The typed client calls the same reply commit-unknown (`mutation_client.go:450-462`). This is a defect of the same class
as #167: a write that may be stored, reported retryable. It is not optional maintenance. #138 item a describes the
mapping split that causes it.

## 3. Adjacent claims

- Specs: `projection-mutation` "Commit ambiguity is preserved" (commit state only, no class);
  `transport-client:417-430` (sentinels transient; unchanged); `graph-transport-boundary:52-53` (`WrapTransient`;
  unchanged); `graph-entity-writes:241` (explicit transient; unchanged). No spec covers `pkg/errs`.
- ADRs: ADR-111 `:93-101` (Decision 3; names #167 the defect's home); ADR-112 `:141-142` (`ErrNotConnected`
  transient by `Classify`; sentinel, unchanged), `:144-146` (no responders: "Not promised: its class, which waits for
  #93's handling of raw nats.go errors"), and `:152` ("Not promised here: `Classify`'s default for unknown errors
  (#167)").
- Ledger: `docs/admission-ledger.yaml:432-451` (`pkg/errs`; `known_risks` `:444-446` states the transient default).
- Docs: `pkg/errs/doc.go:15-16`; `docs/contract.md:59-64` (commit unknown: re-read before retry).
- Open PRs (`gh pr list --state open --json number,title,changedFiles,files`): #240 (draft, test-infra, 25 files):
  integration tests and natsfixture only. It touches no planned file; it does touch other
  `processor/graph-ingest/*_integration_test.go` files, not `commit_ambiguity_integration_test.go`. #235 (draft,
  vocabulary, 44 files): edits `docs/admission-ledger.yaml` vocabulary rows only; same file, other rows. Dependabot
  #242-#248: `go.mod`, `go.sum`, `package*.json`, `ci.yml`. Merge order: any; whichever lands second merges `origin/main`.
- Open issues (`gh issue list --state open --search` on `errs`, `Classify`, `IsTransient`, `commit-unknown`,
  `MutationError`):
  - #167: retired here.
  - #138 (`pkg/projection` port-refactor), items 4 and a: lifecycle and projection map one transport outcome two ways.
    This change edits the same two functions for class only. The consolidation stays with #138. The `ErrEmitFailed`
    defect above is filed as its own issue (task 4.2) and fixed with that consolidation (design, owner question 2).
  - #20 (projection commit ambiguity; open question on graph-ingest's `rejectInternal`): stays; graph-ingest's
    classification is unchanged here.
  - #162 (text-classification check): stays; no text classification added.
  - #223: epic, `Addresses #223`. #146: closed. #132, #139, #222, #9, #7, #174, #129, #94-#96, #105, #32, #169: name the
    word, not this surface.

## 4. Who it is for

Recommended option adds no exported symbol and no method on an exported type. `ErrUpdateRetriesExhausted` keeps its
name and static type `error`. The Unknown option would add an exported `ErrorClass` constant needing a contract line
and a wire value.

## 5. Problem shape

Fail-closed default for an unrecognised value at a seam, plus an explicit class on a typed outcome. Closest instances:
`natsclient/errors.go:351-372` (unknown wire class → invalid), `pkg/lifecycle/graph_emit.go:102-107` (explicit
transient on no-responders), `pkg/errs/errs.go:341-345` (`ErrRevisionMismatch`, a classified sentinel),
`pkg/errs/errs.go:282-287` with `:251-260` (`Classified`: sets a class, keeps the text, and inherits the inner Code and
Detail). Adopted; no new primitive, so no adoption sweep. Collision table: not triggered (no durable, communication or
coordination primitive).

## 6. Pin decision records

No package is carried from the pin; ported `pkg/errs`, `pkg/projection`, `pkg/lifecycle`, `natsclient` are edited.
`grep -l -e 'errs.Classify' -e 'Classify(' -e 'ErrorTransient' -e 'pkg/errs' -e 'CommitUnknown' -e 'commit-unknown'`
over the pin's `docs/adr/*.md`: ADR-050 (names `pkg/errs` as a shape example only, `:128`; not governing) and ADR-060
(governing): `:135` "transient | invalid | fatal (REUSE; no new class)", `:394` "No new error class. The set stays
{transient, invalid, fatal}", `:443-444` `internal` is transient. ADR-091 (amends ADR-060, `:3-6`) `:99-100`:
"surfaces `revision_mismatch` and `commit_unknown` without automatic retry". Neither is ported to `docs/adr/`.

## Adopter seam inventory

Reached from outside: `errs.Classify`, the `X-Error-Class` value of a `SubscribeForRequests` handler error, and
`projection.MutationError.Class` / `IsTransient` / `errors.As(*errs.ClassifiedError)` on a commit-unknown error.

1. **What must they know?** (a) An error they return unclassified is fatal to `Classify` and on the wire; one they
   want retried must be classified transient. (b) A commit-unknown error is fatal; re-read before acting again. Two
   items.
2. **If they do nothing:** an unclassified handler error reaches the remote caller as fatal and is not retried, an
   error surfaces rather than a silent loss. A commit-unknown error is no longer retried by an `IsTransient` branch, or
   by a branch on the class `errors.As` finds. Pin agentrun's switch would quarantine (unacked, redeliverable), not drop.
3. **Where they find out:** typed runtime error (the class), plus `pkg/errs/doc.go` and `docs/contract.md`.
4. **What should they have to know:** only (a). (b) is carried by the error itself.

Measured consumer use: semsource `v1.0.0-beta.6-4-g4093d3c` `processor/code-context/http_error.go:70`, `:79` reads
`ClassifiedError.Class` by `errors.As` (would see fatal on a commit-unknown error); semteams `v0.1.0-12-g0f1a20b0`
`cmd/semteams/approvalpause/projection.go:160` builds a commit-unknown `MutationError` without `Class` (zero value:
transient; not reached by this change), `:164` calls `errs.Classify`; semboids `37dbdb0`
`internal/boidgraph/publisher.go:226`, `internal/sim/lifecycle.go:206` switch on `Kind` (unaffected); semconnect: no
hits.
