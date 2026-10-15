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
  `type AuthoritativeReader interface {`. `git grep -n -E 'EntityCreator|PredicateReconciler|TripleAppender|EntityDeleter|AuthoritativeReader'`
  finds only this file. None of the four consumers names them (symbol count, § 4). The directives
  `mutation_types.go:3` — `//revive:disable:max-public-structs …` and `:182` — `//revive:enable:max-public-structs`
  disable a rule `revive.toml` does not enable (`grep -n max-public-structs revive.toml`: nothing). CONFIRMED.
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
- Item 2c, receipt commit: `pkg/projection/mutation_types.go:89-94` — `type MutationReceipt struct {` … `Commit CommitState`.
  Readers of `receipt.Commit`: only tests (`pkg/projection/*_test.go`, 22 reads; `commit_ambiguity_integration_test.go:116`,
  `:130`, `:152`, `:168`, `:224`, `:252`, `:359`, `:374`). The four consumers read `.Commit` only on `*MutationError`
  (semteams `approvalpause/projection_test.go:121`, `:297`, `:324`; `projection_integration_test.go:95`), and none builds
  a `MutationReceipt` that sets `Commit`. The spec reads the receipt: `openspec/specs/projection-mutation/spec.md:32`,
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
  not-committed, lifecycle reports commit-unknown. `graph_emit.go:117-128` — `lifecycleMutationOperation`'s `default:`
  returns `MutationOperationAppend`. CONFIRMED.
- Item 5, failure outcomes: none of the exported methods (`:137`, `:189`, `:223`, `:254`, `:269`) says which `Commit`
  or `Kind` its errors carry, and `Reconcile`'s doc (`:187-188`) does not say that a failed read reports operation
  `read-authoritative`. CONFIRMED.
- Comment item a: `mutation_client.go:470-472` — `Operation: operation, Kind: MutationUnavailable, Class: errs.Classify(err)…`
  sets `Kind` and patches it later (`:483`, `:490-507`, `:441`). When the caller's context is already done,
  `request` returns `TransportDeadline` and the error reads `Kind` `MutationUnavailable`, which `mutation_types.go:124`
  documents as "no available mutation responder". Commit and class are already right: not committed, and
  transient, because `errs.IsTransient` lists `context.Canceled` and `DeadlineExceeded` at `pkg/errs/errs.go:170-171`.
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
| `pkg/lifecycle/graph_emit.go:88-115` | three codes → lifecycle sentinels; any other classified reply → `ErrEmitFailed` keeping its class (#257); no-responders → not committed; else unknown |
| semteams `cmd/semteams/approvalpause/projection.go:160-172` | consumer copy: builds `MutationError` by hand, with no `Class` on commit-unknown (#258) |
| `processor/graph-ingest/canonical_mutations.go:58-75` | server-side metric outcome label from graph-ingest's own error; a different fact (no commit state), not an adopter |

`MutationOperation` (`mutation_types.go:96-110`) repeats `graphmutation.Operation`. It cannot be replaced: a public
signature may not name an `internal/` type (`harness-boundaries` spec).

## 3. Adjacent claims

- Specs: `openspec/specs/projection-mutation/spec.md` (five requirements; three of them read the receipt).
  `error-classification` is not changed.
- Ledger: `docs/admission-ledger.yaml:2065-2090` (`pkg/projection`, `repair-before-port`) and `:649-664`
  (`pkg/projection/contract`, **`carry`**: `ledger:check` fails a `carry` row that differs from the pin, `Taskfile.yml:61-62`).
  The row's `known_risks` (`:659-661`) still says "pkg/projection is ported in a later change".
- ADR-111 (`docs/adr/111-durable-execution-primitive.md:19-20`, three outcomes; `:66` cites `mutation_types.go:80-83`
  for the receipt; Decision 3, a commit-unknown is never retried).
- Open PRs (`gh pr list --state open --json number,title,changedFiles,files`, 2026-10-15):
  - #266 (draft, PR 5: `metric`, `internal/tlsutil`, `internal/resource`; 0 files yet; no code overlap; possibly
    `docs/admission-ledger.yaml`, other rows);
  - #242–#248 (Dependabot: `go.mod`/`go.sum`, `package*.json`, a workflow);
  - none touches `pkg/projection`, `pkg/projection/contract`, `payloadregistry`, `pkg/lifecycle` or
    `processor/graph-ingest`.
- Unclaimed wave PRs touching neighbours: PR 3 (`payloadregistry`, #219), PR 8 (`graph`'s `ExactEntityReader`
  interface, #132 item 3; `internal/graphmutation`), and 9b (`pkg/lifecycle`, #154 `:22`, #257).

### Open issues naming the package

Queries: `gh issue list --state open --search "projection" --limit 200`, and the same for `contract`.

| Issue | Names it by | Disposition |
| --- | --- | --- |
| #138 | title, body | **Retired** (`Closes #138`), tasks 3.1–3.6. The lifecycle half of item 4 lies in `pkg/lifecycle`, under #154 (`:22`) and #257 (9b) |
| #257 | `pkg/projection/mutation_client.go:450-462` | Stays, for 9b: lifecycle's switch to the mapping. 9a supplies the mapping (D1) |
| #258 | `*projection.MutationError` | Stays: the zero class lives in `pkg/errs`; D1's mapping always sets `Class`, and the consumer literal is semteams' own |
| #20 | `pkg/projection/mutation_client.go` (pin) | Stays: the open question is graph-ingest's server half (comment 2026-10-13); the client half was done by #93 |
| #132 | item 3, `graph.ExactEntityReader` (`pkg/projection/mutation_client.go:32` holds one) | Stays, for PR 8; if that PR makes the reader a concrete type, projection's field type follows there |
| #24, #9, #223, #161 | the word, through rulings or examples | #223 is addressed (PR 9a line); the others name the word, not work in this package |

The other hits (#7, #15–#17, #25–#36, #74, #80, #90, #95, #106, #108–#110, #133–#135, #139, #141, #162, #169,
and #174, #195–#197, #216, #219) name the word, not the package.

## 4. Who it is for (new and changed surface)

- New `projection.ClassifyMutationError` (design D1): framework API. Contract line: the spec delta's new requirement.
  It gets a test in package `projection_test` that calls it the way a product would. Production readers:
  `pkg/projection`'s own methods (this change), `pkg/lifecycle/graph_emit.go:88-115` (9b), and semteams
  `approvalpause/projection.go:160-172` (at cutover).
- New `contract.Contract.Clone` (D5): plumbing. Readers: `pkg/projection/mutation_client.go:94` and
  `payloadregistry/registry.go:192` (Q2).
- Removed: five interfaces (no reader); `MutationReceipt.Commit` (tests only, § 1).

Symbol counts across the four consumers (23 files): the aliases `Contract` (15 files), `ModeReconcile` (6),
`PredicateGroup` (4), `ValidateContracts` (2); the interfaces 0; `MutationReceipt` 4 (tests, none setting `Commit`).

## 5. Problem shape

Classify an error from an observed outcome into a verdict (not committed or commit unknown) plus a class. The closest
existing instance is `mutationFailure` and `isDefiniteFailure` in this package, built on
`graphmutation.IsDefinitelyNotCommitted`. The design exports that existing function; it adds no new interpreter.
Adoption sweep, one line per adopter: `pkg/lifecycle/graph_emit.go:88-115` (9b, #257);
semteams `cmd/semteams/approvalpause/projection.go:160-172` (consumer, at cutover).

## 6. Pin decision records

None is carried; this change ports nothing. Search: the pin's ADRs grepped for `pkg/projection`, `MutationClient`,
`CommitUnknown`, `MutationReceipt`. Hits: ADR-056, ADR-059, ADR-081 (they name the package; none decides
anything this change touches) and **ADR-103** (`:53`: "`projection.Contract` literals keep compiling"; `:71`: "The
contract data types move to a leaf package with aliases in `pkg/projection`"). ADR-103 governs the aliases (Q1).

## 7. Defects found outside #138 (for filing; not added)

- **D-1** `pkg/projection/mutation_client.go:281-282` —
  `mutationFailure(MutationOperationReadAuthoritative, err)`: a `Reconcile` whose own read fails with an unclassified
  error (`nats.ErrTimeout`, or the caller's deadline during the read) reports commit-unknown and fatal although nothing was
  written. Real trigger: the exact-entity read times out under load. `TestReconcileReadFailureIsNotCommitted`
  (`commit_ambiguity_test.go:233`) covers only a classified reply.
  Severity HIGH (a write that was never sent is never retried, per ADR-111 D3).
- **D-2** `internal/graphmutation/client.go:136-152`: a refusal before the request is sent (nil client, nil context,
  `ResolveSubject`, a `json.Marshal` failure on a statement `Object` that cannot be encoded) leaves `request` unwrapped,
  and both mappers read it as commit-unknown. It errs on the safe side; MEDIUM. Package belongs to PR 8.
- **D-3** `docs/admission-ledger.yaml:659-661`: stale `known_risks` text. Fixed in this change only if Q2 changes the row.

## Adopter seams

- `ClassifyMutationError`. (1) What they must know: pass the request's error unchanged, or wrapped with `%w`, and name
  the operation. Two facts. (2) If they do nothing and keep building `MutationError` by hand, they keep #258's
  unset class. (3) Where they find out: nowhere, for hand-built errors; a typed value, for callers of the mapping.
  (4) What they should have to know: nothing beyond calling it.
- Removing `MutationReceipt.Commit`: a reader gets a compile error (the strongest signal). None was measured.
- Removing the interfaces: a compile error; none was measured. In Go, the consumer declares the interface it needs.
- Widening `MutationUnavailable` (Q3): callers that branch on it take the same action (not committed, retry is safe).
  `errors.Is(err, context.Canceled)` still tells the cases apart.
