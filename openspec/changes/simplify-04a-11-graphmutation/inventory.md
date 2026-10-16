# Inventory: simplify-04a-11-graphmutation

base: `f6fc698d699c0208115dbfb52738f397a9bdff88` (origin/main `c6d006d7ecdfb6429d282dcb68ff196fcaf953d4` plus the empty
claim commit)

Question: where does `internal/graphmutation` decide that a request was never sent, which callers read that answer,
and what does #137 ask to remove from the subject resolution and its graph-ingest caller. Scope: #268 and #137
(epic #223, PR 8a; owner 2026-10-16, #223 comment 6216623583). This change brings in no package from the pin.
Consumers are read only as `docs/inventory-scope.md` allows, and only through earlier measurements cited here.

## 1. The claimed gaps (re-measured)

### #268: a request refused before sending is labelled commit-unknown

- `internal/graphmutation/client.go:153` —
  `reply, err := c.requester.RequestClassified(ctx, subject, payload, c.timeout)`

- `internal/graphmutation/client.go:155-158` — `var classified *errs.ClassifiedError` …
  `return fmt.Errorf("graph mutation %s rejected: %w", operation, err)`

- `internal/graphmutation/client.go:159-160` — `if natsclient.IsNoResponders(err) {` … `Outcome: TransportUnavailable`
- `internal/graphmutation/client.go:162` —
  `return &TransportError{Operation: operation, Outcome: TransportCommitUnknown, Err: err}`

- The producer, natsclient, is code in this repository, not a library. `natsclient/errors.go:232-237`:
  `RequestClassified` calls `c.RequestWithHeaders`, and that function returns
  - `natsclient/request.go:258-259` — `if conn == nil || !conn.IsConnected() {` / `return nil, ErrNotConnected`
  - `natsclient/request.go:263-264` — `if c.Status() == StatusCircuitOpen {` / `return nil, ErrCircuitOpen`

  Both come before the only send, `natsclient/request.go:297` — `reply, err := conn.RequestMsgWithContext(reqCtx, msg)`.
  After `:297` the function returns nats.go's raw error, or `ClassifyReply`'s classified error (`errors.go:237`).
  Neither of those is one of the two sentinels.

- Library behaviour (ADR-112 / #170): ADR-112 §5 `docs/adr/112-natsclient-contract.md:184-186` — "While reconnecting,
  `Publish`, `Request`, `RequestClassified`, … return `ErrNotConnected` at once (P15)". P15 was measured at nats.go
  v1.54.0 and nats-server v2.15.0 (`:12-15`). What is not promised: whether a raw nats.go error from `:297` was
  returned before the message left the client. If the connection drops between `:258` and `:297`, which nats.go
  error comes back is not probed.

- `natsclient/request_integration_test.go:128-136` — a client built by `NewClient("nats://unused")` and never
  connected returns `ErrNotConnected` from `Request`. It needs no broker.

- Real triggers (HIGH): a broker reconnect (P15), and the circuit opening after request failures (`request.go:300-304`
  records a failure).

- MEDIUM (#185: needs a caller bug or an unencodable value). These errors are made before sending and returned
  unwrapped, so callers read them as commit-unknown:
  - `client.go:136-137` — `if c == nil || c.requester == nil {` /
  `return errors.New("graph mutation client is not initialized")`
  - `client.go:139-140` — `if ctx == nil {` / `return errors.New("graph mutation context is required")`
  - `client.go:145-148` — `subject, err := ResolveSubject(SubjectFamily, operation)` … `return err`
  - `client.go:149-151` — `payload, err := json.Marshal(request)` …
  `return fmt.Errorf("marshal graph mutation %s: %w", operation, err)`

  A `message.Triple.Object` holding a NaN float is an outside-input trigger for the marshal case: `encoding/json`
  refuses NaN, and the standard library documents this.

### #137: a family parameter with one admitted value; graph-ingest derives it twice

- `internal/graphmutation/protocol.go:36-38` —
  `func ResolveSubject(family string, operation Operation) (string, error) {` / `if family != SubjectFamily {`

- Callers. The production ones: `client.go:145` (passes `SubjectFamily`) and
  `processor/graph-ingest/canonical_mutations.go:145` —
  `subject, resolveErr := graphmutation.ResolveSubject(provider, operation.operation)`.
  `provider` is the string from `:158-197`, which succeeds only when
  `:183` — `len(subjects) != 1 || subjects[0] != graphmutation.SubjectFamily || !hasContract ||` is false, so it always
  equals `SubjectFamily`. The test callers: `internal/graphmutation/protocol_test.go:13`, `:30`, `:38`;
  `processor/graph-ingest/mutation_port_contract_test.go:31`, `:40`;
  `pkg/lifecycle/manager_integration_test.go:101`, `:155`, `:178`.

- The table is built twice: `canonical_mutations.go:134-142` (an anonymous struct slice), then `:143-154`
  (`[]canonicalMutationRoute`).

- The second caller of the provider check: `processor/graph-ingest/component.go:312` —
  `if _, err := canonicalMutationProvider(c.Ports); err != nil {`
  The returned string is discarded there.

- Item a (#137 comment, 2026-10-09): `client.go:87-88` —
  `if response.Outcome != graph.MutationApplied || request.Entity == nil || response.Entity == nil ||`
  This checks `request.Entity` only after the request has been sent.

- CONFIRMED, all three.

## 2. Every spelling of "was the request sent?"

| Site | What it decides |
| --- | --- |
| `internal/graphmutation/client.go:142-143` | context done before the call → `TransportDeadline` (classified at the point it is made) |
| `client.go:159-160` | no responders → `TransportUnavailable` |
| `client.go:162`, `:170-171` | everything else, and invalid replies → `TransportCommitUnknown` |
| `client.go:55-61` | `IsDefinitelyNotCommitted`: unavailable or deadline |
| `pkg/projection/mutation_client.go:444`, `:514` | reads `IsDefinitelyNotCommitted` or `natsclient.IsNoResponders` (raw) |
| `pkg/projection/mutation_client.go:397-402` | projection's own pre-send refusals: `MutationInvalid`, not committed, no code |
| `pkg/lifecycle/graph_emit.go:63-75` | `emitError` → `projection.ClassifyMutationError` (no rule of its own) |
| `natsclient/request.go:258-264`; `natsclient/client.go:53-57` | the producer, and the sentinels (transient, T7) |
| `openspec/specs/projection-mutation/spec.md:129-131` | "including natsclient's refusals before sending until #268 is fixed" |
| `pkg/projection/mutation_client.go:419-422` | doc comment: refusals reported as commit-unknown (#268) |
| `pkg/projection/classify_mutation_error_test.go:74-78` | rows that build `TransportCommitUnknown` wrapping either sentinel by hand |
| `docs/admission-ledger.yaml:2101-2102` | projection `known_risks`: "reads commit-unknown (#268)" |
| `docs/contract.md:63-67` | consumer doc: outcomes; no-responders is not committed |

The spellings of the subject family: `internal/graphmutation/protocol.go:16`, `:42`
(`strings.TrimSuffix(family, ">")`); `processor/graph-ingest/canonical_mutations.go:174`, `:183`;
`processor/graph-ingest/component.go:348`; `internal/harness/contract/reservedsubjects_test.go:35`.

## 3. Adjacent claims

- Open PRs (`gh pr list --state open --json number,title,changedFiles,files`; none over 100 files):
  - #277 (this claim, 0 files).
  - #275 (draft, PR 3) edits `processor/graph-ingest/canonical_mutations.go:218` (`requireRegisteredMessageType`)
    and `component.go:674`, `:2060`, `:2068`. Other hunks than this change's `:128-197`.
  - #273 (draft, PR 7) edits `processor/graph-ingest/component.go:814` (`Register`) and a
    `component-registration` spec delta. That delta does not touch the import-ban lines naming
    `internal/graphmutation`.
  - No open PR touches `internal/graphmutation`, `pkg/projection`, `pkg/lifecycle`, `natsclient`, or the
    `projection-mutation` spec. The rest are Dependabot bumps (#242–#248), which do not overlap.

- Specs: `projection-mutation` (requirement "One mapping…", `:119-154`; it holds the #268 clause);
  `graph-transport-boundary` `:17` (one declaration of the family; this change keeps it); `transport-client`
  `:417-430` (sentinel classes; unchanged).

- Ledger: `docs/admission-ledger.yaml:1986-2005` (`internal/graphmutation`, `adapt`). Its contract says
  "transport-error classification as at the pin", which this change alters. Also `:2099-2103` (projection
  `known_risks`).

- Coverage floor: `scripts/cover-check.sh:37` — `"internal/graphmutation unit"` (80%).
- Archived context: `openspec/changes/archive/2026-10-15-simplify-04a-06-projection/design.md:34` (D1 row
  "fatal (unchanged until #268)") and `:136-140`;
  `openspec/changes/archive/2026-10-16-simplify-04a-08-lifecycle/design.md:151`.

### Open issues naming the package

`gh issue list --state open --search "graphmutation" --limit 200` and `--search "\"graph.mutation\""`:

| Issue | Names it in | Disposition |
| --- | --- | --- |
| #268 | title, body | retired by this PR |
| #137 | title, body, comment | retired by this PR |
| #106 | body (`graph.mutation.>` keeps its one home, `protocol.go:16`) | stays: a constraint this change keeps; its work is `graph.query.*` |
| #132 | body item d cites `graphmutation.NewClient` as the good example | names the word, not a defect in this package |
| #223 | epic lines | `Addresses #223` |
| #139 | body: `processor/graph-ingest/canonical_mutations.go:31-126` (section B, the mutation lane's metering), `:303-308` (the pre-write sequence), `:413-430` (the append no-op re-fetch) | stays: its section B covers the metering at `:31-126`; this change edits `:128-197`, none of its cited lines |
| #103 | comment: `canonical_mutations.go:348` (the reconcile fence `Update(..., request.ExpectedRevision)`) | stays: names a line this change does not edit |

The owner's 8a ruling (#223 comment 6216623583) replaces #137's pairing ("Lands in: with #139's mutation-lane item
(section B …)"): #137 is retired here, and #139 stays open.

## 4. Readers of the surface touched

- `ResolveSubject`: see § 1. After the change, `Subject` has one production reader,
  `processor/graph-ingest/canonical_mutations.go` (plumbing).

- `TransportUnavailable`, `TransportError`, `IsDefinitelyNotCommitted`. Production:
  `pkg/projection/mutation_client.go:444`, `:514`.
  Tests: `pkg/projection/classify_mutation_error_test.go:34`, `:65-81`;
  `pkg/lifecycle/graph_emit_contract_test.go:123-125`.

- Production requesters are `*natsclient.Client` only: `pkg/projection/mutation_types.go:15` —
  `NATS      *natsclient.Client`;
  `pkg/lifecycle/manager.go:97` — `mutationClient, err := graphmutation.NewClient(client, 5*time.Second)`.

- Consumers (`internal/` cannot be imported; they see the effect through `projection.MutationError`). Measured in
  `archive/2026-10-15-simplify-04a-06-projection/inventory.md:137-139` and not re-measured: semboids
  `internal/boidgraph/publisher.go:222` and `internal/sim/lifecycle.go:182`, `:202` treat `MutationUnavailable` as a
  retry signal; the four consumers read `.Commit` only on `*MutationError` (`:41-43`).

## 5. Problem shape

Classify a refusal at the seam where it is made: one typed outcome, read by `errors.As`. The closest instance is in
the same function: `client.go:142-143` — `if err := ctx.Err(); err != nil {` /
`return &TransportError{… Outcome: TransportDeadline …}`.
For client-made invalid refusals, the closest instance is `pkg/projection/mutation_client.go:397-402`
(`invalidMutationError`: invalid, not committed). No new primitive is established, so no adoption sweep is owed.
The collision table is not triggered: there is no new durable, communication or coordination primitive. The intent
check is not triggered: no boundary moves.

## 6. Pin decision records

Every pin ADR was fetched at `8b99efe9` and searched for `graphmutation`, `ResolveSubject`,
`TransportCommitUnknown`, `IsDefinitelyNotCommitted`, `graph.mutation`, `commit.unknown` and `commit_unknown`.

- **ADR-091** (Graph Mutation Authority Without Semantic Ownership): `:51` — four typed operations under family
  `graph.mutation.>`; `:69` — "A lost reply after possible delivery is `commit_unknown`"; `:100` — "`commit_unknown`
  without automatic retry". It governs the package.

- **ADR-060** (Unified RPC Error Contract), Status `:6`: "`commit_unknown` is a typed mutation-client transport
  outcome". It governs `TransportError`.

- **ADR-103** `:29` cites `internal/graphmutation/client.go:89`: the framework's writers call the client directly, so
  the registered-type gate is ingest-side.

- ADR-032 (`component.ResolveSubject`, another function), ADR-054, ADR-055, ADR-056 and ADR-068 name the old
  mutation subjects only. None of them decides anything here.

- In this repo: ADR-111 Decision 3 (`docs/adr/111-durable-execution-primitive.md:86-91`: a commit-unknown is never
  retried) and ADR-112 §3 and §5.

## 7. Shape sweep (current tree) and defects for filing

1. A second spelling: the family check is made twice, and the `graph.mutation.` prefix is derived three times
   (§ 2). #137 covers the check. The prefix derivations are optional; they stay.
2. A copied shell: none.
3. An interface with one production implementer: `client.go:15-17` `requester`, satisfied by `*natsclient.Client`
   and the test fakes, with a twin at `pkg/projection/mutation_client.go:18-20`. Optional; it stays as the narrow
   dependency.
4. Exported with test-only readers outside the package: `TransportCommitUnknown`, `TransportDeadline`, and
   `TransportError` construction (projection and lifecycle tests). Optional; it stays, and no owner has scheduled it.
5. Classification by error text: none (`errors.As`/`errors.Is`; `natsclient/errors.go:312-313`).

Defects outside #268 and #137: none found. Searches: the package read in full; the readers in § 4.

## Adopter seams

1. What a caller of `projection.MutationClient` or the lifecycle manager must know: nothing new. A write refused
   because the broker is reconnecting, or the breaker is open, reports `MutationUnavailable`, not committed,
   transient. `errors.Is(err, natsclient.ErrNotConnected)` still reaches the cause.
2. If they do nothing: today such a write is commit-unknown and fatal (it is quarantined under ADR-111 Decision 3,
   with a re-read first). After this change, a retry loop keyed on `MutationUnavailable` or `errs.IsTransient`
   retries it, which is safe because nothing was sent.
3. Where they find out: a typed runtime error (`Kind`, `Commit`, `Class`).
4. The gap between what they must know and what they should: none. #137 is internal to this module.
