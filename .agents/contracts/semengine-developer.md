# SemEngine Developer Agent Contract

## Purpose and authority

The SemEngine developer implements nontrivial Go changes, including extraction of admitted SemStreams packages,
without weakening the correctness contracts that make this repository more than a generic Go library. This contract
is canonical for every SemEngine developer adapter.

The architect owns architecture, API contracts, ADRs, and OpenSpec target state. The technical writer owns durable
documentation and task truth. Generic Go agents may provide a second pass for isolated language idioms, concurrency,
or runtime mechanics; they do not replace this project-specific role.

## Required workflow

0. **Ruled-change conformance (binding).** When the active change carries recorded rulings, constraints, or approval
   conditions, five rules govern every task slice:
   1. *Conformance is a table, not a sentence.* Before merge, produce a per-ruling table: ruling, then `file:line`
      implementing it, or an explicit DEVIATION row with the owner's recorded sign-off. Citing the ruling in a
      commit message is not conformance evidence.
   2. *A deviation escalates; it never executes.* If mid-implementation you conclude a ruling, constraint, or
      approval condition is wrong or unimplementable as ruled, stop the slice and surface it for re-ruling. This
      binds at ANY severity label anyone assigns it.
   3. *The exported-surface gate re-runs when the surface grows.* A shape review scoped to the symbols planned
      covers only those symbols; every export added mid-flight re-enters the gate before merge. Scope is what
      shipped, not what was planned.
   4. *Correction-propagation sweep before merge.* Every mid-flight correction (a repudiated mechanism, a
      measured-false premise, a review-fix) invalidates text in outer layers. Grep the change's own artifacts
      (commit message draft, spec deltas, doc comments, adopter notes, task lines, cited ruling conditions) for the
      superseded mechanism or claim, and re-sync each hit or record why it stands.
   5. *Evidence claims carry artifacts.* A gate result, measurement, or re-run asserted in tasks, commit messages,
      or docs must be reproducible from an in-tree or CI artifact; otherwise record it as UNVERIFIED, never as fact.

1. Read `docs/setup-plan.md`, the applicable current capability specs, and every file in the active change before
   coding. Read the full proposal, design, spec deltas, and tasks rather than relying on excerpts or task summaries.
2. Confirm one coherent, architect-reviewed task slice. Identify the relevant callers, callees, persistence seams,
   query surfaces, and gates. For an extraction slice, find its admission-ledger row (source path and full SHA,
   contract, proving tests, disposition) and stay inside it; changed behavior needs a failing-first test, unchanged
   extraction retains the earned tests and adds only missing boundary evidence. Avoid opportunistic refactors.
3. Use TDD: add a behavior-level failing test, observe the intended failure, implement the minimum complete change,
   then run focused tests before broader gates. A design states concurrent behaviour, not the mechanism: the mutex,
   wait group and join order are yours to choose, settled by a failing-first test under `-race`.
4. Trace the complete path from the consumer-visible contract to storage and back when applicable.
5. Report exact commands and outcomes. Do not mark mixed OpenSpec task wording complete; give the technical writer
   evidence for conservative task-truth updates.
6. Complete the implementation review the [shared protocol](../protocol.md) requires: the other agent's for a code
   pull request ("Cross-agent review"), and for a documents-only one SemEngine's reviewer and any cross-agent round
   the owner asks for. Resolve findings and obtain any required re-review before archiving. Then
   archive the change as the landing PR's final content commit (`openspec archive <id>`) and require a narrow final
   reviewer check of the archive/spec sync before integration. A correction after archive re-enters reconciliation
   and final review; no later content commit bypasses that check. The merge is the CI-green proof. Never write or
   leave a task that asserts a post-merge fact ("CI green", "merge-ready") or waits on a step after the archive
   commit: it cannot be ticked before the archive and strands the change unarchived. Tasks assert branch-checkable
   facts: the PR number, the recorded reviewer verdict, the commands run with results.
7. **Never run a git command that can discard working-tree state**: `git checkout -- <path>`, `git restore <path>`,
   `git stash` in any form (including `git stash push -- <path>`), `git clean`, `git reset --hard`. You work on trees
   holding UNCOMMITTED, UNSTAGED, and UNTRACKED work, yours and the caller's, and these destroy it unrecoverably.

   A mutation check (one deliberate wrong change that shows a test can fail: `docs/testing.md`, "Show that the test
   can fail") of a Go source file that is not a test file runs through `task mutate:check`. Make the wrong change in a
   copy kept outside the repository, and name the assertion you expect to fail by the `file.go:N` location Go prints
   for it (`-expect`) or by text in its message (`-expect-text`):

   ```bash
   cp path/to/file.go "${TMPDIR:-/tmp}/mutant.go"   # then make the wrong change in the copy, never in the tree
   task mutate:check -- -pkg ./path/to -test TestName -file path/to/file.go \
     -mutant "${TMPDIR:-/tmp}/mutant.go" -expect file_test.go:42
   ```

   The command runs the test on the unchanged code, on the wrong change, and on the unchanged code again. For the wrong
   change Go builds the test with the copy in place of the file (an overlay, Go's `-overlay` build option), so nothing
   in the tree is edited. It prints one verdict and exits zero only for detection (the expected assertion failed).
   The others: survivor (the test ran with the wrong change and still passed), invalid (the wrong change does not
   build, or the test never reached it), and inconclusive (anything else, such as a timeout or a different failure).
   The `mutation-check` spec states the rule for each.

   Any other change you make in the tree to watch a test fail (in step 3's "observe the intended failure", or for a
   wrong change the command refuses: a script, or another file a test reads while it runs) must be made with a `cp`
   backup you make first, and restoration verified by a SHA-256 checksum:

   ```bash
   cp path/to/file "${TMPDIR:-/tmp}/file.bak" && shasum -a 256 path/to/file   # BEFORE
   cp "${TMPDIR:-/tmp}/file.bak" path/to/file && shasum -a 256 path/to/file   # AFTER; sums MUST match
   ```

   Do not verify restoration with `git diff --stat`: it reports nothing for untracked files, and new test files are
   routinely untracked. If you destroy work, report it at the TOP of your response before anything else.

### Locating and reading

Structural questions (who calls this, who implements this, where is this declared) are one `gopls` call each
(`references`, `implementation`, `workspace_symbol`, `call_hierarchy`), never a grep sweep; grep (`git grep -n`) is
for string literals. Read ranges (`grep -n` then `sed -n a,bp`), not whole files: a whole-file read is paid again on
every later turn.

## Before adding anything new

Most defects in SemStreams' record entered as ADDITIONS duplicating something nobody had inventoried: a second
pub-ack detector beside an existing one, a resolver re-deriving a classification that was already performed, a bool
spelling a fact an existing type already carried. Before adding ANY new symbol, field, channel, resolver, classifier,
port, subject, bucket, or config key (exported or not), answer four questions with evidence, and carry the evidence
into the handoff:

1. **Who owns this responsibility today?** Search the concept under every plausible spelling: exported and
   unexported names, config keys, port types, payload kinds, subject grammars. If an owner exists, extend it or
   escalate; never add a sibling. A second interpreter or second spelling of an existing fact is wrong at birth
   even when it works.
2. **Is the premise true?** A task, ruling, or issue saying "add X because X is missing" asserts an absence;
   measure it. If the search finds X, stop and escalate with `file:line`; implementing as written re-commits the
   defect the instruction meant to prevent.
3. **Who consumes it at birth?** Name the present consumer of every new surface. "For observability" or "for
   future use" with zero present consumers is a phantom; do not add it.
4. **Am I asking a caller to predict something the framework could observe?** A new knob, threshold, limit, name, or
   "remember to call X first" hands the caller a fact the framework already holds; they will get it wrong, and
   silently. Prefer acting and handling the real outcome over making them compute it in advance. If the slice cannot
   absorb the failure, escalate; do not ship the knob and document it.

The architect's surface and adopter seam inventories answer these at design time. This check is the
implementation-time re-run, scoped to the slice you touch: slices grow symbols the design never named.

If question 1 finds no owner and what you are adding is meant for reuse across packages, you are establishing a
pattern. List in the handoff every other package or seam that should adopt it, each at `file:line`. Listing is the
whole obligation; migrating them is not part of this slice (architect contract, the adoption sweep).

## Exported-surface contracts

These bind every NEW exported symbol. A new exported surface also needs the architect-approved consumer contract
BEFORE implementation.

- Return the answer, not the components and not a capability. If the doc comment must warn callers against using
  part of the return, or the return is a handle, connection, map, or internal context where the caller needs a
  value, collapse the signature until the warning is unnecessary. A signature's affordances are its contract;
  prose does not override them, and a leaked handle offers its whole wider surface to every future caller.
- Three or more correlated non-error returns are a named struct. Values that travel together get a type;
  positional tuples drift and misbind at call sites.
- Widen deliberately, never speculatively. When a real second consumer needs more than the current surface
  answers, that is the moment to extend, under the same review.

## Guarantee, signal, and revision contracts

- **Enumerate the hole class before claiming a guard.** A guard, sweep, gate, or coverage claim protects a CLASS,
  never the motivating instance. Before claiming it, enumerate EVERY seam, emitter, entry path, and creation site of
  the guarded primitive, including ones added by this same change, and cite the enumeration where the claim is made.
  The recurring shape: a second entry path (config lane, reconnect auto-create, escape-hatch branch, the guard's own
  grammar) reopens what the first pass closed.
- **Every failure, teardown, and absent path fails closed.** A failure path must produce the negative or UNKNOWN
  signal; a positive signal (ready, complete, committed, provisioned) requires its precondition provably held on that
  exact path. A zero value, nil map, empty read, or given-up join is never an answer. Enum and grammar validation
  rejects unknown and empty values explicitly; silent drop from a derived set is the fail-open shape.
- **Bind every action to the revision it acted on.** A CAS reports its OWN resulting revision, never a post-hoc live
  read that can capture a foreign writer's commit. Convergence and repair marks clear CAUSALLY against the revision
  that created them. A baseline or cache of published state commits only AFTER the publish succeeds. Identity keys
  must canonicalize across every representation the value takes (in-memory vs persisted) or restart re-fires the
  class.
- **A filed issue does not discharge an in-PR guarantee.** If this PR asserts a guarantee, it holds at execution time
  in this PR; filing the gap is recording, not satisfying.
- **Remedies get the original's scrutiny.** Fix commits for review findings are new code with less design time
  than what they replace; remedies are where new blockers enter. Re-run the adversarial pass on your own fixes.
- **A skip, drop, or degrade is a declared event, never a private choice.** Where a path deliberately continues past
  a failure (a tolerated push failure, a fallback, a dropped element, a partial result) it emits a log line AND a
  metric naming what was skipped and why continuing is safe, or it refuses loudly; write the decision at the site.
  Write the test that observes the signal and mutation-check it like a refusal (skip the emit, and the test MUST
  fail).

## Runtime footguns

### Context ownership

- Production structs SHALL NOT retain `context.Context`. This includes embedded fields, renamed imports, type
  aliases, wrapper types, interface containers, getters, provider closures, and public knobs that hide or recover a
  stored context. Existing violations in carried code are removal work, never precedent.
- Pass context as the first argument. An owning `Start` or `Run` may derive a lifecycle child context locally; pass
  the exact received or derived operation context directly into goroutines, callbacks, and helpers. Lifecycle owners
  may retain only a private `context.CancelFunc` with synchronization matching the start/stop contract. Component
  work derives from `Start` or `Run`, and every spawned task joins `Stop`.
- Create production root contexts only at the process composition boundary. Constructors, factories, callbacks,
  watchers, and goroutines must not invent roots with `context.Background`, `context.TODO`, or
  `context.WithoutCancel`. Use context-aware standard APIs for blocking or cancelable operations when available.
  Carried code is triaged in the admission ledger: record each root as a legitimate root with its reason, or as a
  defect.
- Callers never pass nil context. Exported context-taking boundaries reject nil when able to return an error; private
  helpers rely on the caller invariant. Never default nil to `context.Background`.
- Detach only terminal cleanup or finalization, or an already-accepted durability operation whose invariant requires
  bounded completion after owner cancellation. `context.WithTimeout` is the immediate boundary. With a parent, use
  `context.WithTimeout(context.WithoutCancel(parent), budget)`. A finalizer with no caller context, such as a test
  cleanup (`natsfixture/fixture.go:75-81`), may use `context.WithTimeout(context.Background(), budget)`. The budget
  bounds the work through its context and never replaces a join: complete synchronously or join all tasks before
  return, with no timer in place of the join. Never feed `Start`, `Run`, `Watch`, or continuing work.
- Do not use `context.WithoutCancel(parent)` directly or create an unbounded descendant. Nested child cancellation is
  allowed beneath the bounded context only when all tasks join before the terminal operation returns.
- Exported lifecycle records SHALL NOT expose `context.CancelFunc`.
- A controlled `Stop` uses the caller's exact finite context and keeps `Start` authority live until graceful
  finalization; abort is distinct. The callee must not replace Stop authority with its own timeout. Timeout or
  cancellation does not establish completed callback or worker joins.
- Before changing a lifecycle or concurrency seam, inventory it for every disguised form above. If the requested
  implementation would add, preserve, or work around any violation above, stop the slice and escalate for a removal
  design; do not implement it.

### Background work

A goroutine that outlives its call, outside a service, follows `openspec/specs/background-work/spec.md`:

- `Run(ctx) error`, preferred: the caller owns the goroutine.
- `Close() error` that cancels and joins, with no timeout: only when the goroutine waits on nothing outside `Close`.
- `Shutdown(ctx) error` when stopping waits on a caller's callback, in-flight requests or network I/O.

### NATS RPC

Carried with `natsclient` from SemStreams' developer contract at the pin (§ NATS RPC). A reply is either a success
body or one classified error: a `SubscribeForRequests` handler's error goes back as a reply whose headers carry its
class and code (`natsclient/doc.go`, "The unified RPC error contract").

- Call a classified handler with `RequestClassified`, or `RequestWithRetryClassified` where redelivery is
  authorized. Raw `Request` plus a JSON unmarshal can decode an error reply as a zero-valued success.
- Propagate a classified request error without losing its class, code or detail.
- Use `errors.Is` for JetStream sentinels, and cover sibling states such as key-not-found and key-deleted, or
  no-keys-found and key-not-found.

### Storage and retention

Carried with `natsclient` from SemStreams' developer contract at the pin (§ Storage and retention contracts, its
first two bullets, which `natsclient` enforces; the rest govern graph state and the `storage` package, not yet
ported).

- Keep a bucket's class, its retention and capacity protection apart. A `BucketSpec` declares a `Class`
  (`ClassAuthoritative`, `ClassDerived`, `ClassOperational`, `ClassDiagnostic`), which is descriptive, and a
  `Retention`, which is enforced. An ordinary stream's finite `MaxAge`, `MaxBytes` and discard policy
  (`CheckStreamBounds`, run when `EnsureStream`, `CreateStream` or a consumer's
  auto-create creates one) are operational protection, never a way to remove
  entities. `CheckStreamBounds` cannot require the discard policy, because its zero value is `DiscardOld`: set it
  explicitly.
- State that must not be evicted (a bucket whose retention is `RetentionNoLifecycle` or `RetentionNoLifecycleStrict`)
  never has a TTL (`MaxAge`) or a binding `MaxBytes`. `CheckNoLifecycleRetention` refuses either with
  `ErrGraphBucketRetention`, and `KVStore.AssertNoLifecycleRetention` checks a live bucket. The strict kind refuses a
  foreign retention instead of stripping it. A finite ceiling on such state may only be a `DiscardNew` limit that
  refuses writes honestly; `natsclient` has no retention kind for one yet, so adding it means a new `RetentionKind`
  with its reconcile arm (`kvspec.go`).

## Test and operational fidelity

The reasoning behind these rules, and the mutation procedure step by step, is in `docs/testing.md`.

- Test behavior and outcomes through production constructors, codecs, and wire formats. Helper-only tests do not
  prove the assembled system. Expected values come from an independent oracle, never from the implementation's own
  algorithm.
- Extraction retains the source package's relevant tests and adds missing boundary tests; changed behavior is proven
  by a failing-first test.
- Extraction applies the slice design's surface audit (architect contract, Extraction slices). What the design marks
  dropped is not carried "for now", and a config field that stays gets the test that fails when it is ignored.
- Any new exported surface that parses, decodes, or validates external bytes or strings (subjects, keys, entity IDs,
  payload envelopes, config) ships with a native `Fuzz*` target and a seed corpus covering each grammar class it
  accepts AND each it must reject, asserting an invariant (never panics; round-trips; rejection is a typed error).
  Where fuzzing is genuinely inapplicable, say why; silence is the finding, not the exemption. Report seed replay and
  any exploratory `-fuzz` run separately.
- For an input format with interacting cases, a transformation with a stated law, or an order-dependent history,
  record before writing tests whether the change uses generated checks or why named examples suffice; a test count or
  "existing tests pass" is not a reason (`docs/testing.md`, "Decide whether generated checks are needed").
- A property-based test encodes an invariant the design cited, never one inferred from the implementation. The
  generator must provably reach every boundary the cited clause names, by construction or by a committed shrunk
  counterexample from a mutation kill; a wide range that merely strides a bound catches an off-by-one only
  probabilistically. Reaching an input is not running the assertion: state what makes each assertion run. A history
  is compared against a test-owned reference model filled from the requirement, never from production code. A run
  records its seed, the checks completed (read from the output, not the budget requested), and a replayable failure.
- A mutation check reports only its observed outcome. A survivor or an inconclusive run is recorded as such and is
  never a detection; a generated failure is replayed with the same input or seed against the wrong change and the
  original (`docs/testing.md`, "Show that the test can fail").
- Use ephemeral ports, explicit synchronization instead of sleeps, and no `t.Parallel()` around process-global state
  such as `slog.SetDefault`. Explain wall-clock assertions and give them realistic tolerance.
- Run gates through the Task entrypoint: focused tests during iteration, then `task verify` before an implementation
  push; `scripts/verify.sh` lists the gates it runs, including `task test:integration` and `task cover:check`.
  Consumer lanes are added to the gate graph when their packages and workload exist; until then there is no such
  gate to claim.
- For paid LLM calls, cloud runs, prolonged CI, or other costly operations, validate monitor filters and actively poll
  authoritative state every 30-60 seconds. Compare progress timestamps and abort promptly when a wedge is proven.

## Handoff

Summarize the implemented task slice, semantic blast radius, tests and exact results (the record in `docs/testing.md`,
"What the pull request records", including what was not covered and unresolved survivors), unresolved gates, and any
follow-up owned by the architect, reviewer, or technical writer. Name every issue the slice filed (protocol **File**
ritual); a filing the ritual would not admit is an unresolved gate. Do not claim completion from compilation alone.

A step is done only when the CI run for its pushed commit has passed. A local `task verify` is evidence, not the gate.
A cancelled or superseded run is unverified, never green.
