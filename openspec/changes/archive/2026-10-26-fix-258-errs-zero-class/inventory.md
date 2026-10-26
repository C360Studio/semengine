# Inventory: an unset error class reads as retryable (#258)

base: 7615030 (worktree HEAD 4f98d7a is the empty claim commit; tree identical)
Change: fix-258-errs-zero-class, claim draft PR #410. Carries no package from the pin; edits ported `pkg/errs`
(and, by literal, `pkg/projection`).

Question: where does `errs.ErrorClass`'s zero value (`ErrorTransient`) get read, built, or encoded, and which
in-scope consumers build a classified error without a class?
Repositories read (docs/inventory-scope.md:17-19): semengine; semstreams pin `8b99efe9` (code facts and ADRs, via
`gh api ?ref=`); semteams, semsource, semconnect, semboids at symbol level (local checkouts, read only; no git
commands, rule 4 at :41-44, so no version is measured).

## 1. The claimed gap

- `pkg/errs/errs.go:17` — `ErrorTransient ErrorClass = iota` (zero value is transient). Same at the pin:
  `pkg/errs/errs.go:21` @8b99efe9.
- `pkg/errs/errs.go:13` — `type ErrorClass int`; no unset or unknown constant exists (constants :15-22). `String()`
  already answers `"unknown"` for any value outside the three (`pkg/errs/errs.go:33-34`; proven by
  `pkg/errs/errs_test.go:20` — `{ErrorClass(999), "unknown"},`).
- Types carrying an `ErrorClass` field (gopls references on `errs.go:13:6`; every other hit is a parameter, result or
  local): `pkg/errs/errs.go:90` — `Class     ErrorClass` (`ClassifiedError`); `pkg/projection/mutation_types.go:138`
  — `Class     errs.ErrorClass` (`MutationError`). No third.
- **`ClassifiedError` half: confirmed.** A literal without `Class` reads transient via `pkg/errs/errs.go:159-160` —
  `if errors.As(err, &ce) { return ce.Class == ErrorTransient`. Because `errors.As` stops at the first
  `*ClassifiedError`, an unset class also hides any wrapped sentinel: `&ClassifiedError{Err: context.DeadlineExceeded}`
  reads transient through its class, not its sentinel.
- **`MutationError` half: no outside reader.**
  - `MutationError` is not a `*errs.ClassifiedError`. It has only `Error()` and `Unwrap()` returning `e.Err`
    (`pkg/projection/mutation_types.go:143-155`). So `errs.IsTransient`/`IsFatal`/`IsInvalid`/`Classify`
    (`errs.go:159-160`, `:181-182`, `:202-203`) never read `MutationError.Class`; they unwrap to `Err`.
  - gopls references on `mutation_types.go:138:2`, production only:
    - writes at `pkg/projection/mutation_client.go:399`, `:437`, `:441`, `:444`, `:457`;
    - one read at `:446`, which hands `notCommittedKind` the class the same function set at `:437`, `:441` or
      `:444`.
  - Every other reference is in a test reading a value `pkg/projection` built.
  - **Nothing reads the `Class` of a `MutationError` built outside `pkg/projection`.** What a classless
    `MutationError` reads as is decided by its `Err`, not by its `Class` field.

## 2. Every current spelling: readers, switches, literals, encodings

### 2a. Production readers of the class

Found with gopls references on `errs.go:90:2`, `mutation_types.go:138:2`, `IsTransient`, `IsFatal`, `IsInvalid` and
`Classify`.

| Site | Text | What an unset (zero) class does today |
| --- | --- | --- |
| `pkg/errs/errs.go:160` | `return ce.Class == ErrorTransient` | true |
| `pkg/errs/errs.go:182` / `:203` | `return ce.Class == ErrorFatal` / `ErrorInvalid` | false |
| `pkg/errs/errs.go:221-236` | `Classify`: transient→fatal→invalid, else `return ErrorFatal` | `ErrorTransient` |
| `pkg/errs/errs.go:222-223` | `return ErrorTransient // Default for nil` | (nil input; a named constant, not the zero read) |
| `natsclient/errors.go:349-357` | `switch errs.Classify(err) { … default: return ErrorClassFatal` | header `"transient"` |
| `natsclient/errors.go:393-402` | `var ec errs.ErrorClass`; every `switch class` branch assigns (default `ErrorInvalid`) | zero never escapes |
| `pkg/projection/mutation_client.go:437` | `Class: errs.Classify(err)` (write) | — |
| `pkg/projection/mutation_client.go:441` | `mapped.Class = classified.Class` (write, from a reply) | copies a zero reply class |
| `pkg/projection/mutation_client.go:444` | `mapped.Class = errs.ErrorTransient` (write) | — |
| `pkg/projection/mutation_client.go:446` | `notCommittedKind(mapped.Code, mapped.Class)` (read of :437/:441/:444) | — |
| `pkg/projection/mutation_client.go:483-489` | `switch class { … default: return MutationUnavailable` | Kind unavailable |
| `pkg/projection/mutation_client.go:518-525` | `classifiedAppendFailure`: string → class, else `ErrorFatal` | n/a (string input) |
| `processor/graph-ingest/canonical_mutations.go:500` | `Class: typed.Class.String()` | `"transient"` on the wire |
| `processor/graph-ingest/canonical_mutations.go:502` | non-classified → `ErrorTransient.String()`, `ErrorCodeInternal` | (not a zero read; see §8) |
| `processor/graph-ingest/keyed_ingest.go:314-322` | `IsInvalid`/`IsFatal` → Terminate; else `DeliveryDecisionRetry` | Retry |
| `processor/graph-ingest/mutation_runtime.go:198` | `if errs.IsInvalid(err)` else `rejectInternal` | internal |

The `:441` row is reached only through a `*errs.ClassifiedError` in a reply chain. A zero there comes from a
classless `ClassifiedError`, which is the `ClassifiedError` half of the defect.

### 2b. Struct literals

Found with `git grep -n -e 'ClassifiedError{' -e 'MutationError{' -- '*.go'`, cross-checked against gopls
keyed-field references.

| Site | Sets `Class`? |
| --- | --- |
| `pkg/errs/errs.go:242` (`newClassified`) | yes, `Class: class` (:243) |
| `pkg/errs/errs.go:352` (`ErrRevisionMismatch`) | yes, `ErrorInvalid` (:353) |
| `pkg/projection/mutation_client.go:398` (`invalidMutationError`) | yes, `ErrorInvalid` |
| `pkg/projection/mutation_client.go:437` (`notCommittedError`) | yes, `errs.Classify(err)` |
| `pkg/projection/mutation_client.go:456` (`commitUnknownError`) | yes, `errs.ErrorFatal` |
| `pkg/errs/classified_is_test.go:96` — `(&ClassifiedError{}).Error()` | **no** (asserts `Error()` text only) |
| `pkg/lifecycle/manager_test.go:1404-1409` — commit-unknown `projection.MutationError` | **no** (asserts Kind/Commit only, :1412-1416) |
| `pkg/errs/errs_test.go:51,52,83,84,109,110,136,142`, `pkg/errs/classified_is_test.go:43` | yes |

**Production literals that omit `Class`: none.** Test literals that omit it: two, listed above. Integration-tagged
files hold no such literal (`grep` of the three `go:build integration` files that mention these names; a
`GOFLAGS=-tags=integration` gopls run adds only test references).

### 2c. Encodings

- **Numeric value is never encoded.** In production code nothing converts `ErrorClass` to or from a number (the only
  `ErrorClass(` conversion is the `ErrorClass(999)` test row at `pkg/errs/errs_test.go:20`). Neither struct has JSON
  tags (`pkg/errs/errs.go:89-97`, `pkg/projection/mutation_types.go:134-142`). The NATS error body type
  `natsclient/errors.go:112-115` — `type errorBody struct { Message … ; Detail … }` — carries no class. It is filled
  at `:120-129` (`marshalErrorBody`).
- The class goes on the wire **as a string** in two places. The `X-Error-Class` header uses
  `natsclient/errors.go:103-105` (`"transient"`, `"invalid"`, `"fatal"`). The append-result body uses
  `graph/mutation_responses.go:146` — ``Class string `json:"class"` `` — through `ErrorClass.String()`
  (`canonical_mutations.go:500`). Its reader `internal/graphmutation/client.go:207` checks only `Class == ""`.
- Log field: `processor/graph-ingest/keyed_ingest.go:319` — `slog.String("class", errs.ErrorFatal.String())` (a named
  constant). No metric label carries a class (`git grep -n -i '"class"'` in production: the two sites above only).

## 3. Adjacent claims

- **Spec** `openspec/specs/error-classification/spec.md:11-15` — `Classify` SHALL return fatal for an unclassified
  error (no `*ClassifiedError` and no listed sentinel); `:27-30` the predicates report only a marked class; `:37-49`
  a marked error keeps its class. No requirement covers a `*ClassifiedError` whose class is unset.
  `openspec/specs/projection-mutation/spec.md:123,128` (commit-unknown is fatal), `:161` (not-committed unavailable
  is transient); `openspec/specs/transport-client/spec.md:681,686` (`ErrNotConnected` is transient).
- **Archived #167 change** `openspec/changes/archive/2026-10-15-simplify-04a-04-errs-classify/`. design.md:15-24
  weighed three options. It rejected a new `ErrorUnknown` class (:19-22) because of pin ADR-060's fixed set, and
  because a consumer switch with `default:` → Retry would retry it (pin `agentic/agentrun/milestone_settlement.go:197`).
  Its inventory.md:38 already listed `errs.go:17` ("a literal without `Class` reads transient"). Its design.md:109
  deferred the zero value. Owner ruling 3 (#167 comment 6191970843, design.md:121) files it as #258.
- **ADRs (pin, not ported to `docs/adr/`):**
  - ADR-060 `docs/adr/060-unified-rpc-error-contract.md` @8b99efe9, `:135` "transient | invalid | fatal (REUSE; no
    new class)" and `:394` "**No new error class.** The set stays {transient, invalid, fatal}".
  - ADR-060's summary `docs/adr/060-pr-d-breaking-change-summary.md` @8b99efe9, `:127` — "The error class set
    `{transient, invalid, fatal}` and the `X-Error-Class` / `X-Error-Code` headers."
  - ADR-091 (`:99-100`, per the archived inventory.md:149-150): no automatic retry of `commit_unknown`.
- **ADRs (in repo):** `docs/adr/111-durable-execution-primitive.md:97-101` names #167 as the home for the transient
  default. `docs/adr/112-natsclient-contract.md:38-41` states the unknown-error default.
- **Admission ledger** `docs/admission-ledger.yaml:442-466`: `pkg/errs` is `disposition: adapt` (:466), **not
  `carry`**. Its `contract` (:448-455) and `known_risks` (:457-461) list each behavioural change from the pin.
  `task ledger:check` fails only `carry` rows. `adapt` rows are printed by `task ledger:diff` and never failed
  (`docs/provenance.md:24-32`, `:51`). So a change to `pkg/errs` needs no pin match: it needs a `contract` and
  `known_risks` line in this row. Rows of the other touched packages: `pkg/projection` `repair-before-port`,
  `natsclient` `adapt`, `processor/graph-ingest` `repair-before-port`, `pkg/lifecycle` `adapt`.
- **Docs that state the class set:** `pkg/errs/doc.go:6`, `:17`, `:21`; `docs/contract.md:66-68`.
- **Open issues** (`gh issue list --state open --search` for `errs`, `ErrorClass`, `MutationError`,
  `ClassifiedError`, `Classify`; hits read; #161 added on review):
  - #258 is this issue.
  - #162 (harness check against classifying by error text) names `errs.IsTransient`/`IsFatal`. It does not overlap:
    its subject is text matching, which #146 already removed.
  - #161 (open; "harness: an exported identifier in a production package that nothing reads fails a check") bears on
    any shape that adds an exported constant (§7 A): such a constant needs a reader or a contract line.
  - #20 (needs-decision) covers graph-ingest's transient `internal` and is adjacent to `canonical_mutations.go:502`.
    It concerns a classification choice, not the zero value.
  - #300 names `errs.Wrap*` op names in graph-ingest only and does not overlap.
  - #96, #95, #94, #32, #105 and epic #9 use the word, not the package.
  - #257 (the lifecycle `ErrEmitFailed` path) is CLOSED.
- **Open PRs** (`gh pr list --state open --json number,title,isDraft,changedFiles,files`): #410 (this claim, 0
  files) and #409 (draft, 6 files, all under
  `openspec/changes/setup-04a-04-graph-roots-processor-shell/`). #409's design and tasks (fetched at
  `ref=claude/processor-shell-109`) plan Go edits under `component/`, `service/`, `internal/cache`,
  `internal/lifecycleguard`, `internal/harness/` and `processor/graph-ingest/component.go`, and none under
  `pkg/errs`. Its only `errs` mention is the sentinel `errs.ErrAlreadyStarted` (unaffected). **No file overlap.**

## 4. Who it is for

Every option touches an existing exported surface (`ErrorClass`, `ClassifiedError`, `MutationError`). Any new
exported constant or constructor would need a contract line: the `pkg/errs` ledger `contract` field or the
`error-classification` spec. Open #161 would also hold it to having a reader.

## 5. Problem shape: a zero value that must not read as a valid choice

Closest instances in the tree:

- `natsclient/delivery_settlement.go:20-21` — `// DeliveryDecisionInvalid is the zero and invalid decision.` /
  `DeliveryDecisionInvalid DeliveryDecision = iota`. A named invalid zero; the caller refuses it (:316-318).
- `config/key_family.go:37` — `KeyFamilyPut KeyFamilyOperation = iota + 1`. The zero is left unnamed.
- `pkg/projection/mutation_types.go:75` — `type CommitState string`. The empty zero matches no named state.

## 6. Pin decision records

No package is carried from the pin. The governing records for `pkg/errs`:

- ADR-060 (`docs/adr/060-unified-rpc-error-contract.md` `:135`, `:394`, `:443-444`).
- Its PR-D summary (`docs/adr/060-pr-d-breaking-change-summary.md` `:127`).
- ADR-091, which amends ADR-060.

All were read with `gh api …/docs/adr/<file>?ref=8b99efe9`. None is ported to `docs/adr/`.

## 7. Candidate shapes and measured blast radius (no recommendation)

"Unset" below means a `*errs.ClassifiedError` built without `Class`. No shape changes what a classless
`MutationError` reads as: nothing outside `pkg/projection` reads `MutationError.Class` (§1), and `IsTransient` and
`Classify` read its `Err`. A shape that wants to cover that half needs a reader of `MutationError.Class` that does
not exist today.

| Shape | New exported symbol | What an unset class reads as (from §2a) | Unchanged | Consumer case |
| --- | --- | --- | --- | --- |
| A. Named unset zero (`ErrorUnknown`/`ErrorUnset = iota`; the three shift by one) | yes, a fourth constant. Conflicts with pin ADR-060 `:394` and the PR-D summary `:127`; #167 design.md:19-22 rejected the same constant; #161 asks for a reader | `IsTransient`/`IsFatal`/`IsInvalid` false; `Classify` → fatal (`errs.go:236`); header `"fatal"` (`natsclient/errors.go:357`); `notCommittedKind` → `MutationUnavailable` (`mutation_client.go:488`); append wire `String()` needs a case or reads `"unknown"`, which the reader treats as fatal (`mutation_client.go:518-525`); `keyed_ingest.go:322` still Retry | no numeric encoding (§2c); every named-constant site | none measured: no in-scope consumer builds a classless `ClassifiedError` (adopter seam). semteams `:160` is unaffected (its `Class` is unread) |
| A'. Unnamed zero (`ErrorTransient ErrorClass = iota + 1`, precedent `config/key_family.go:37`) | none | same as A; `String()` already returns `"unknown"` (`errs.go:33-34`) | same as A | same as A |
| B. Keep `ErrorTransient = 0` and treat zero specially | none | **not expressible**: an explicit `ErrorTransient` (`newClassified` :243 via `WrapTransient`, `Classified(ErrorTransient,…)`, `natsclient/errors.go:398`) and an unset class are the same bit. Only works with a second field (e.g. a set flag), which is new representation | — | — |
| C. Constructors only, literals refused by a check | a constructor per type, and a new verify guard (`harness-boundaries`) | in-repo production literals: none to fix (§2b). A check here cannot reach consumer code | everything | none: consumer literals are outside this repo |

Behaviour that changes under A/A' and is pinned by no spec: an unset-class `ClassifiedError` wrapping a transient
sentinel (`context.DeadlineExceeded`) reads transient today and not transient afterwards (`errs.go:159-160`). Tests
in the repo that rely on the zero being transient: none found (the only numeric use is `errs_test.go:20`). The two
classless test literals (`classified_is_test.go:96`, `manager_test.go:1404`) assert no class.

## 8. Observations outside #258's scope (report, not fold in)

- `processor/graph-ingest/canonical_mutations.go:502` encodes a failure that is not a `ClassifiedError` as
  `"transient"` with `ErrorCodeInternal`. This follows pin ADR-060 `:443-444` and sits under #20. It is not a
  zero-value read.
- `processor/graph-ingest/keyed_ingest.go:322` retries any error that is neither invalid nor fatal, so under A/A' an
  unset class still retries there. #167's inventory.md:39 records that ADR-111 Decision 3 allows this.
- A commit-unknown `MutationError` built outside `pkg/projection` takes its retryability from its `Err`, not from
  `Class` or `Commit`.
  - semteams `cmd/semteams/approvalpause/projection.go:160` wraps the incoming error unchanged as `Err`.
  - Callers at `:122`, `:126`, `:134` pass a client error. When that error already carries a transient class
    (graph-ingest's `internal` reply, ADR-060 `:443-444`), `errs.IsTransient` on the commit-unknown result reads
    true.
  - Callers at `:131`, `:137` pass `errors.New(...)`, which `Classify` reads fatal.
  - This needs no zero value: it holds whatever `Class` the literal sets. `pkg/projection` avoids it by wrapping
    `Err` in a fatal `ClassifiedError` (`mutation_client.go:457-458`); a consumer literal does not.
  - Not measured further: whether semteams code retries on `IsTransient` for that result.

## Adopter seam inventory

Surface reached from outside: `errs.ErrorClass`, the `ClassifiedError` and `projection.MutationError` struct
fields, and `IsTransient`/`Classify`.

Measured consumers: all four import `github.com/c360studio/semstreams`, not SemEngine. semteams/semboids/semconnect
use `v1.0.0-beta.162.0.20260930150212-8b99efe9c66a` (`go.mod:6`); semsource uses `v1.0.0-beta.160` (`go.mod:8`).
No consumer compiles against `semengine/pkg/errs` today, so a SemEngine change reaches them only when they migrate.

Builders of a classless literal:

- semteams `cmd/semteams/approvalpause/projection.go:160` — `return &projection.MutationError{Operation: …,
  Kind: projection.MutationCommitUnknown, Commit: projection.CommitUnknown, Err: err}` (no `Class`; re-measured, text
  matches the issue; version unmeasured, no git in sister checkouts).
- semboids `internal/boidgraph/publisher_test.go:105`, `internal/sim/lifecycle_test.go:221` —
  `&projection.MutationError{Kind: kind, Err: …}` (tests, no `Class`).
- No in-scope consumer builds a classless `*errs.ClassifiedError`. semsource's test literals
  (`processor/supersession/prefixquery_test.go:17,28,31`) all set `Class`; semteams, semconnect and semboids build
  none (`grep -rn 'ClassifiedError{'`).

Readers:

- `MutationError.Class`: none in any in-scope consumer. They read `Kind` or `Commit`:
  - semboids `internal/boidgraph/publisher.go:217-219` — `if errors.As(err, &me) { switch me.Kind {`;
  - semboids `internal/sim/lifecycle.go:229-231` — `if errors.As(err, &me) { return me.Kind, true`;
  - semteams `approvalpause/projection_test.go:120-121` — `me.Commit != projection.CommitUnknown`.
- semteams `:164` (`Class: errs.Classify(err)`) and `:167-168` read the class of the incoming error inside
  `refusedProjection`, not the `:160` literal.
- `ClassifiedError.Class`: semsource `processor/code-context/http_error.go:70,79` (`classified.Class ==`); semteams
  `approvalpause/projection.go:167-168` (on an incoming error).
- `errs.IsTransient`: semconnect `gateway/cs-api/systems.go:871,928,986,1053`, `systems_post.go:475`.

1. **What must they know?** That a `ClassifiedError` literal without `Class` is retryable, and so they must set
   `Class`. That is one item, and it is the defect itself. For `MutationError`, `Class` is read by nobody outside
   `pkg/projection`, so what a consumer must know there is how its `Err` is classified (§8).
2. **If they do nothing:** a classless `ClassifiedError` reads transient through `IsTransient`, `Classify`, the
   `X-Error-Class` header and the append wire (§2a). No in-scope consumer builds one today (measured above), so
   this default path is reached by no measured caller. A classless `MutationError` reads whatever its `Err` reads;
   its `Class` field changes nothing a consumer observes.
3. **Where they find out:** nowhere. Nothing in the compiler, at boot or at runtime says so, and no doc states it.
4. **What should they have to know:** nothing. The gap between 1 and 4 is that a classless `ClassifiedError` has to
   stop reading retryable. Whether the `MutationError` half is in scope is for the design to decide: by measurement
   it is a field no outside code reads, not a retry path.

## Searches that closed categories

- gopls `references` on `pkg/errs/errs.go:13:6`, `:90:2`, `:152:6`, `:174:6`, `:195:6`, `:221:6`;
  `pkg/projection/mutation_types.go:138:2`; `graph/mutation_responses.go:146:2`.
- `git grep -n -e 'ClassifiedError{' -e 'MutationError{'`; `-e 'ErrorClass(' -e 'int(.*Class'`; `-i '"class"'`
  (production); `-E 'iota( \+ 1)?$'`.
- Consumers: `grep -rn --include='*.go'` for `ClassifiedError{`, `MutationError{`, `IsTransient`, `errs.Classify`,
  `.Class ==`, `switch .*\.Class` over semteams, semsource, semconnect, semboids (`.worktrees` excluded).
- `gh issue list --state open --search` for errs, ErrorClass, MutationError, ClassifiedError and Classify;
  `gh issue view 161`; `gh pr list --state open --json …files`.
