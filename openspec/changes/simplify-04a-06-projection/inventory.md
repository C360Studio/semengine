# Inventory: simplify-04a-06-projection

base: `a9aaf68111b3571ff8ebe8dfb47f94502820a218` (origin/main `e3aa72b` plus the empty claim commit)

Question: what in `pkg/projection` (and its leaf `pkg/projection/contract`) does #138 ask to remove or merge, who reads
each piece, and which callers convert a mutation request's error into a commit state. Consumers read for symbol use
(`docs/inventory-scope.md`): semconnect (typed mutation client), semsource (imports), and semteams and semboids. For
semteams and semboids these symbol counts fall outside the question the scope table names for them, so they are
counted here and no decision rests on them alone. Their local working trees were read with grep. Every consumer
imports `github.com/c360studio/semstreams/pkg/projection` (23 files), because none has cut over yet. Pin decision
records come from `gh api repos/C360Studio/semstreams/contents/docs/adr?ref=8b99efe9`.

This change carries no package from the pin. `pkg/projection` is already ported (ledger row `pkg/projection`,
`repair-before-port`), so no pin probe is owed. Non-test lines: `mutation_client.go` 538, `mutation_types.go` 182,
`contract.go` 34, `doc.go` 4, `contract/contract.go` 163.

## 1. The claimed gaps (#138, re-measured; its cites were at PR #93 head `7bf1eb8`)

- Item 1, five interfaces: `pkg/projection/mutation_types.go:157-180` — `type EntityCreator interface {` …
  `type AuthoritativeReader interface {`.
  `git grep -n -E 'EntityCreator|PredicateReconciler|TripleAppender|EntityDeleter|AuthoritativeReader'` finds only
  this file. None of the four consumers names them (§ 4). The directives `mutation_types.go:3` —
  `//revive:disable:max-public-structs …` and `:182` — `//revive:enable:max-public-structs` disable a rule
  `revive.toml` does not enable (`grep -n max-public-structs revive.toml`: nothing). CONFIRMED.
- Item 2a, aliases: `pkg/projection/contract.go:12-34` — `type WriteMode = contract.WriteMode` …
  `func ValidateContracts(contracts []Contract) error {`. **The premise "no file outside pkg/projection uses the
  aliases" is false**:
  - in this repo, `processor/graph-ingest/commit_ambiguity_integration_test.go:94` — `contract := projection.Contract{`
    (and `:97-98`, `:202-206`, `:342-346`), plus `pkg/projection/mutation_client.go:24`, `:89`, `:114`, `:191`, `:225`;
  - consumer production code: semconnect `gateway/cs-api/projection_contracts.go:43` — `return projection.Contract{`;
    semsource `graph/contract.go:69` — `return projection.Contract{`; semboids `internal/boidgraph/contract.go:28` —
    `return projection.Contract{`; semteams `cmd/semteams/approvalpause/projection.go:52` —
    `contract := projection.Contract{… Groups: []projection.PredicateGroup{{… Mode: projection.ModeReconcile …`;
  - the aliases' own test: `pkg/projection/contract_test.go:47` — `func TestContractLiteralCompilesAgainstAliases`.
- Item 2b, clone: `pkg/projection/mutation_client.go:114-123` — `func cloneContract(contract Contract) Contract {`,
  the same lines as `payloadregistry/registry.go:224-233` — `func cloneContract(c contract.Contract) contract.Contract {`.
  CONFIRMED.
- Item 2c, receipt commit: `pkg/projection/mutation_types.go:89-94` — `type MutationReceipt struct {` …
  `Commit CommitState`. Readers of the receipt's `Commit`: only tests (`pkg/projection/*_test.go`, 20 lines;
  `processor/graph-ingest/commit_ambiguity_integration_test.go:116`, `:130`, `:152`, `:168`, `:224`, `:252`, `:359`,
  `:374`, `:378`). The four consumers read `.Commit` only on `*MutationError` (semteams
  `approvalpause/projection_test.go:121`, `:297`, `:324`; `projection_integration_test.go:95`), and none builds a
  `MutationReceipt` that sets `Commit`. The spec reads the receipt: `openspec/specs/projection-mutation/spec.md:32`,
  `:38`, `:56`, `:61`, `:67`, `:73`, `:78`. CONFIRMED.
- Item 3, dead layers (all CONFIRMED):
  - `mutation_client.go:413` — `return triples, metadata, nil`: `metadata` is never assigned in `canonicalizeTriples`
    (`:357-414`).
  - `:224` — `_, _, triples, metadata, err := c.canonicalizeGroupMutation(` (five return values).
  - `:181-183` — `if response.Entity.ID != entity.ID {`: `graphmutation.Client.Create` already refuses this at
    `internal/graphmutation/client.go:87-89`.
  - `:247-248` — the `default:` after `validateAppendResponse` (`client.go:200-211` refuses any other outcome).
  - `:524-538` — `cloneDetail` sorts keys and then inserts them into a map.
  - `:75-86` — `func isNilRequester(requester mutationRequester) bool {` uses `reflect`; `NewMutationClient` has
    already refused a nil client at `:38`.
- Item 4, two mappings: `mutation_client.go:427-432` (`mutationFailure`), `:455-468` (`isDefiniteFailure`) against
  `pkg/lifecycle/graph_emit.go:88-115` (`classifyEmitError`). Case `TransportDeadline`
  (`internal/graphmutation/client.go:142-143`, the context ended before the request was sent): projection reports
  not-committed, lifecycle reports commit-unknown. `graph_emit.go:117-128` — `lifecycleMutationOperation`'s
  `default:` (`:125-126`) returns `MutationOperationAppend`. CONFIRMED.
- Item 5, failure outcomes: none of the exported methods (`:137`, `:189`, `:223`, `:254`, `:269`) says which `Commit`
  or `Kind` its errors carry, and `Reconcile`'s doc (`:187-188`) does not say that a failed read reports operation
  `read-authoritative`. CONFIRMED.
- Comment item a: `mutation_client.go:470-472` —
  `Operation: operation, Kind: MutationUnavailable, Class: errs.Classify(err)…` sets `Kind` and patches it later
  (`:483`, `:490-507`, `:441`). When the caller's context is already done, `request` returns `TransportDeadline` and
  the error reads `Kind` `MutationUnavailable`, which `mutation_types.go:124` documents as "no available mutation
  responder". Commit and class are already: not committed, and transient, because `errs.IsTransient` lists
  `context.Canceled` and `DeadlineExceeded` at `pkg/errs/errs.go:170-171`.
- Comment item b: `mutation_client.go:39` —
  `invalidMutationError(MutationOperationCreate, errors.New("NATS client is required"))` against `:54` —
  `return nil, errors.New("mutation requester is required")`. CONFIRMED.

## 2. Every spelling of "the commit state of a mutation request's error"

| Site | Spelling |
| --- | --- |
| `pkg/projection/mutation_client.go:455-468` | refusal-code list plus transport outcome → not committed, else unknown |
| `pkg/projection/mutation_client.go:470-509` | code/class → `Kind` |
| `pkg/projection/mutation_client.go:275-282` | read: a classified error → not committed; an unclassified one → `mutationFailure` (commit-unknown; § 7, D-1) |
| `internal/graphmutation/client.go:55-61` | `IsDefinitelyNotCommitted`: transport `unavailable`/`deadline` |
| `internal/graphmutation/client.go:162` | any unclassified requester error other than no-responders → transport `commit_unknown`, including natsclient's pre-send refusals (§ 7, D-2) |
| `pkg/lifecycle/graph_emit.go:88-115` | three codes → lifecycle sentinels; any other classified reply → `ErrEmitFailed` keeping its class (#257); no-responders → not committed; else unknown |
| semteams `cmd/semteams/approvalpause/projection.go:160-172` | consumer copy: builds `MutationError` by hand, with no `Class` on commit-unknown (#258) |
| `processor/graph-ingest/canonical_mutations.go:58-75` | server-side metric outcome label from graph-ingest's own error; a different fact (no commit state) |

`MutationOperation` (`mutation_types.go:96-110`) repeats `graphmutation.Operation`. It cannot be replaced: a public
signature may not name an `internal/` type (`harness-boundaries` spec).

## 3. Adjacent claims

- Specs: `openspec/specs/projection-mutation/spec.md` (five requirements; three of them read the receipt).
  `error-classification` names no projection symbol.
- Ledger: `docs/admission-ledger.yaml:2065-2090` (`pkg/projection`, `repair-before-port`) and `:649-664`
  (`pkg/projection/contract`, **`carry`**: `ledger:check` fails a `carry` row that differs from the pin,
  `Taskfile.yml:61-62`). The row's `known_risks` (`:659-661`) still says "pkg/projection is ported in a later change".
- ADR-111 (`docs/adr/111-durable-execution-primitive.md`): `:19-20`, the typed write client "with three outcomes:
  committed, not committed, commit unknown"; `:66`, the receipt "held for one call (`mutation_types.go:80-83`)", a
  cite at `522bf3a`, the SHA its `:11` pins SemEngine facts to (today `:89-94`); `:194`, "`pkg/lifecycle` and
  `pkg/projection` stay what ADR-047, ADR-049 and ADR-091 made them"; Decision 3, a commit-unknown is never retried.
- Open PRs (`gh pr list --state open --json number,title,changedFiles,files`, 2026-10-15):
  - #266 (draft, PR 5, head `4177812`; change `simplify-04a-06-metric-tls-resource`): code in `metric`,
    `internal/tlsutil`, `internal/resource`. Its task 6.2 edits `docs/admission-ledger.yaml`, other rows: a shared
    file. Its design `:84-87` states the same rule: whichever lands second merges `origin/main` first;
  - #242–#248 (Dependabot: `go.mod`/`go.sum`, `package*.json`, a workflow);
  - none touches `pkg/projection`, `pkg/projection/contract`, `payloadregistry`, `pkg/lifecycle` or
    `processor/graph-ingest`.
- Unclaimed wave PRs touching neighbours: PR 3 (`payloadregistry`, #219), PR 8 (`graph`'s `ExactEntityReader`
  interface, #132 item 3; `internal/graphmutation`), and 9b (`pkg/lifecycle`, #154 `:22`, #257).

### Open issues naming the package

Queries: `gh issue list --state open --search "projection" --limit 200`, and the same for `contract`.

| Issue | Names it by | What it asks |
| --- | --- | --- |
| #138 | title, body | items 1–5 and comment items a–b (§ 1); item 4 spans `pkg/projection` and `pkg/lifecycle` |
| #257 | `pkg/projection/mutation_client.go:450-462` | lifecycle reports a possibly-stored write as retryable; fix with #138 item a |
| #154 | `:22`, `graph_emit.go:88-125` "one of four copies" | lifecycle's half of the mapping, among other lifecycle items |
| #258 | `*projection.MutationError` | a `MutationError` literal without `Class` reads transient (`pkg/errs` zero value) |
| #20 | `pkg/projection/mutation_client.go` (pin) | open question on graph-ingest's server half (comment 2026-10-13); the client half was done by #93 |
| #132 | item 3, `graph.ExactEntityReader` (`pkg/projection/mutation_client.go:32` holds one) | the reader interface has one implementation (PR 8) |
| #24, #9, #223, #161 | the word, through rulings or examples | #223 lists PR 9; the others carry rulings or examples |

The other hits (#7, #15–#17, #25–#36, #38, #74, #80, #90, #95, #103, #106, #108–#110, #133–#135, #139, #141, #153,
and #162, #169, #174, #189, #195–#198, #216, #219) name the word, not the package.

## 4. Readers of the surface #138 names

Symbol counts across the four consumers (23 files): the aliases `Contract` (15 files), `ModeReconcile` (6),
`PredicateGroup` (4), `ValidateContracts` (2); the five interfaces 0; `MutationReceipt` 4 files, two of them
production: semboids `internal/boidgraph/publisher.go:45` —
`Reconcile(ctx context.Context, request projection.ReconcileMutation) (projection.MutationReceipt, error)` and
`internal/sim/lifecycle.go:68` —
`Delete(ctx context.Context, request projection.DeleteMutation) (projection.MutationReceipt, error)`. Neither file
reads the receipt's `Commit`. Both lines are in semboids' own one-method interfaces.

`MutationUnavailable` as a retry signal: semboids `internal/boidgraph/publisher.go:222` and
`internal/sim/lifecycle.go:182`, `:202` — `case projection.MutationRevisionConflict, projection.MutationUnavailable:`.
semteams builds one at `approvalpause/projection.go:164`.

`cloneContract` readers: `pkg/projection/mutation_client.go:94`; `payloadregistry/registry.go:192` and `:241`
(through `cloneContracts`, read at `:257`, `:294`).

## 5. Problem shape

Classify an error from an observed outcome into a verdict (not committed or commit unknown) plus a class. The closest
existing instance is `mutationFailure` and `isDefiniteFailure` in `pkg/projection`, built on
`graphmutation.IsDefinitelyNotCommitted`. Other hand-built copies of that verdict: `pkg/lifecycle/graph_emit.go:88-115`
and semteams `cmd/semteams/approvalpause/projection.go:160-172`.

## 6. Pin decision records

None is carried; this change ports nothing. Search: the pin's ADRs grepped for `pkg/projection`, `MutationClient`,
`CommitUnknown`, `commit_unknown`, `MutationReceipt`. Hits:

- **ADR-091** (Graph Mutation Authority Without Semantic Ownership): `:69` — "A lost reply after possible delivery is
  `commit_unknown`; matching later content does not prove which request wrote it."; `:100` — "`commit_unknown` without
  automatic retry; the component owns any later operation-specific retry decision." ADR-111 `:194` keeps
  `pkg/projection` as ADR-091 made it.
- **ADR-103**: `:53` — "`projection.Contract` literals keep compiling"; `:71` — "The contract data types move to a leaf
  package with aliases in `pkg/projection`". It governs the aliases.
- ADR-056, ADR-059, ADR-081 name the package; none decides anything #138 touches.

## 7. Defects found outside #138 (for filing)

Both defects below are the same failure: a write that was never sent is reported commit-unknown and fatal, so under
ADR-111 Decision 3 nobody retries it.

- **D-1, HIGH.** `pkg/projection/mutation_client.go:281-282` —
  `mutationFailure(MutationOperationReadAuthoritative, err)`. If `Reconcile`'s own read fails with an unclassified
  error, it reports commit-unknown although nothing was written. Real triggers: the exact-entity read times out under
  load (`nats.ErrTimeout`); the caller's deadline passes during the read; the client is reconnecting
  (`ErrNotConnected`) or its breaker is open (`ErrCircuitOpen`). `TestReconcileReadFailureIsNotCommitted`
  (`commit_ambiguity_test.go:233`) covers only a classified reply.
- **D-2, HIGH.** `internal/graphmutation/client.go:162` —
  `return &TransportError{Operation: operation, Outcome: TransportCommitUnknown, Err: err}`. It wraps natsclient's
  refusals that happen before sending: `natsclient/request.go:259` — `return nil, ErrNotConnected` and `:264` —
  `return nil, ErrCircuitOpen`, both reached through `RequestClassified` (`natsclient/errors.go:233`). Both mappers
  then read them as commit-unknown. Real triggers: a broker reconnect, and the circuit breaker opening. Same file,
  MEDIUM, because they need a caller bug, not a real-world condition: `:136-152`, refusals made before sending (nil
  client, nil context, `ResolveSubject`, a `json.Marshal` failure on a statement `Object` that cannot be encoded)
  are returned unwrapped and also read as commit-unknown. The package belongs to PR 8.
- **D-3, LOW.** `docs/admission-ledger.yaml:659-661`: stale `known_risks` text.

## Adopter seams (existing surface)

- The five interfaces: no consumer names them. semboids declares its own interfaces at the call site
  (`publisher.go:45`, `lifecycle.go:68`), as Go expects.
- `MutationReceipt.Commit`: no consumer reads it; a reader would find out through a compile error.
- `MutationError` built by hand: semteams (`approvalpause/projection.go:160-172`) has to know the code list, the
  transport outcomes and the class rule. It already gets the class wrong (#258) and finds out nowhere.
- `MutationUnavailable`: semboids retries on it (§ 4). Today a caller's context that ended before sending reads
  `MutationUnavailable`, not committed, transient (§ 1, comment item a). semboids' retry is therefore safe for it,
  and `errors.Is(err, context.Canceled)` still tells the cases apart.
