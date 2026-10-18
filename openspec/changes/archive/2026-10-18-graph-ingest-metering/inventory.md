# Inventory: graph-ingest-metering

base: `bccd8b8ef4c1b35ba88699a6fb37e27b0f68c00a` (origin/main `bd11ab0` plus the empty claim commit; draft PR #307)

Status: `INVENTORY PASS` on round 2 (semengine-reviewer, 2026-10-18; round 1 `CHANGES REQUESTED` with one BLOCKING,
who reads `DataFlow` at the pin, corrected). Every pin is at the base commit and under `processor/graph-ingest/`
unless another path is given.

Question: what #302, #303 and #300 section B find on today's tree; who reads each surface they touch; and which
other open issues name the package. Scope: #302, #303, #300 section B (#139 section B text; ruling L, #91 comment
6062681355), and the #190 export listing (#223 comment 6214503370). Not in scope (PR #307 body): #300 sections C, D,
F, G, H; #301; #306; #291; #216.

This change brings in no package from the pin. `processor/graph-ingest` is already ported (ledger row
`docs/admission-ledger.yaml:2280`, `disposition: repair-before-port` at `:2326`), so no pin probe runs are owed. Pin
reads are from the `8b99efe9` tarball (`gh api repos/C360Studio/semstreams/tarball/8b99efe9`, local only).

Repositories read (`docs/inventory-scope.md`): this tree; the pin; working trees of semsource `4093d3ce`, semconnect
`dff12657`, semteams `30ce863a`, semboids `37dbdb08`, semvocab `c8a07be9` and semsparql `51ac5357`, read without git
commands (HEAD taken from `.git/HEAD`).

## 1. The claimed gaps (re-measured)

### #303: two WARN lines for one revision mismatch

The call path for a reconcile or delete refused with `revision_mismatch`:

1. `mutation_runtime.go:26` — `sub, subscribeErr := c.subscribeForRequests(ctx, route.subject, c.meteredCanonicalMutation(route))`
2. `canonical_mutations.go:32` — `metered := c.meteredMutation(route.subject, route.handler)` (outer wrapper `:31-54`).
3. The handler returns `rejectRevisionMismatch` (`mutation_runtime.go:239-241`). Reconcile does this at
   `canonical_mutations.go:296` (read revision) and `:315` (CAS lost); delete at `:449` and `:455`.
4. The inner wrapper, `mutation_runtime.go:41-63`: `recordPredicateContractRejections` (`:47`, no-op here). Not an
   authority error (`:52`), so the reason is the code (`:56-60`). `recordMutationRejection` (`:61`) then increments
   `mutation_rejections_total` (`:157`) and logs **WARN 1** at `:158` — `c.logger.Warn("graph mutation rejected",`.
   Its attributes are subject, reason and error text.
5. Back in the outer wrapper: `recordCanonicalOutcome(route.operation, canonicalErrorOutcome(err))` (`:36-37`). Then,
   for `ErrorCodeRevisionMismatch` only (`:39`), **WARN 2** at `:40` — `c.logger.Warn("graph mutation revision mismatch",`.
   Its attributes are operation, entity_id and expected_revision, taken from the error's Detail.

Every other refusal logs once. Authority refusals go through `recordAuthorityRejection` (`authority_gate.go:79-99`, one
WARN) and return at `mutation_runtime.go:54`; the outer wrapper adds no WARN for them. natsclient adds no WARN for a
handler error: it logs only when it fails to publish the reply (`natsclient/request.go:368-372`). A request refused
before `Start` completes is answered inside `subscribeForRequests` (`component.go:1048-1050`), never reaches either
wrapper, and is not metered. The same path exists at the pin: `canonical_mutations.go:40`, `mutation_runtime.go:165`.

### #300 section B: two wrappers and the re-decode

- `canonical_mutations.go:46-53` re-decodes the handler's own success reply. On a decode failure it counts
  `internal` and logs a third WARN text: `:49` — `c.logger.Warn("canonical mutation response could not be metered",`.
- `recordCanonicalResponse` (`:67-100`) decodes per operation. Create and reconcile take the reply's `Outcome` (`:74`).
  Append takes each subject's `Outcome`, or `result.Error.Code` when the outcome is `failed`, and refuses a `failed`
  result that has no error (`:80-88`). Delete takes `Outcome` (`:95`).
- `recordCanonicalOutcome` (`:102-107`): an empty outcome becomes `internal`, then
  `c.mutationOutcomes.WithLabelValues(string(operation), outcome).Inc()` (`:106`).
- `canonicalErrorOutcome` (`:56-65`): the classified code, or `internal` when there is none.
- Every reply the handlers marshal: create `:254`; reconcile `:302` (unchanged) and `:327` (applied); append `:396`
  (results built at `:368-394`); delete `:465`.
- The metric: `component.go:94-102` (`Name: "graph_mutation_outcomes_total"` at `:98`; help text "Canonical graph
  mutation outcomes by bounded operation and outcome" at `:99`); field at `:526`; built at `:722`; registered at `:757`.

**Ruling L at base.** The label already is the classified code. It landed in PR #286 (`git log -S'or internal when it
carries none (ruling L' -- processor/graph-ingest/canonical_mutations.go` gives `2d9ce7c`). At the pin,
`canonicalErrorOutcome` mapped five codes to four outcomes (`entity_not_found`, `entity_already_exists`,
`revision_mismatch`, and `invalid` for both `invalid_request` and `structural_invalid`) and every other code to
`internal` (pin `canonical_mutations.go:58-75`; outcome values at pin `graph/mutation_responses.go:109-115`).
`isServerMutationOutcome` is gone (`git grep -n isServerMutationOutcome`: none). Still present: `canonicalErrorOutcome`
and the re-decode layer, both of which ruling L says go, and the second wrapper (#139 B).

### Outcome label values the metric takes today

`graph_mutation_outcomes_total{operation,outcome}`:

| operation | success (from the re-decoded reply) | refusal (classified code of the error) |
| --- | --- | --- |
| `entity.create` | `applied` | `invalid_request` (`:190-242`), `message_type_unregistered` (`:180`), `entity_id_authority_invalid` (`:205`), `entity_id_invalid` (malformed ID through `authorizeSubject`'s parse, `pkg/types/entity_id_authority.go:36-38`; no test), `entity_already_exists` (`:247`), `rejectFromError` (`:250`) → `invalid_request` or `internal` |
| `entity.reconcile` | `applied`, `unchanged` | `invalid_request`, `entity_id_authority_invalid`, `entity_not_found` (`:290`, `:320`), `revision_mismatch`, `graph_state_reset_required` (`readEntity`, passed by `rejectFromError` `mutation_runtime.go:230`), `internal` |
| `triple.append` | per subject: `applied`, `unchanged`, `entity_not_found`, or the failure code `graph_state_reset_required` / `invalid_request` / `internal` (`canonicalAppendFailure` `:495-502`) | `invalid_request`, `entity_id_authority_invalid`, `internal` (`:361`, `:388`); `:359` wraps `batchErr` with `%w`, so its code, or `internal` when it has none |
| `entity.delete` | `applied` | `invalid_request`, `entity_id_authority_invalid`, `entity_not_found`, `revision_mismatch`, `internal` (`:463`) |
| any | `internal` when the re-decode fails (`:48`) | — |

`structural_invalid` is not reachable on this metric. Its only producer, `validateTriplePredicates`, has one production
caller, `component.go:1713` (fact lane); the doc comment `graph/mutation_responses.go:11` agrees. The reply-outcome
spelling `entity_not_found` (`graph/mutation_responses.go:117`) and the error code (`:29`) are the same string.

Spec: `openspec/specs/graph-transport-boundary/spec.md:155-159` says to "count each canonical mutation … with
`outcome` set to the reply's outcome, or to the error's classified code, or to `internal`". Append counts once per
subject, not once per mutation (`canonical_mutations.go:80-88`).

### Query error-code spellings (`query.go`)

| Verb | Return | Spelling |
| --- | --- | --- |
| entity | `:72`, `:79` | `ClassifiedCode(Invalid, invalid_request)` |
| entity | `:90`; `:93-94`; `:96` | `entity_not_found`; state error passed through; `ClassifiedCode(Transient, internal)` |
| batch | `:114` | `errs.Classified(errs.ErrorInvalid, …)`, no code |
| batch | `:118` | hand-written `` []byte(`{"entities":[]}`) `` when `ids` is empty; when every requested ID is the empty string, each is collected into `invalid` (`:481`) and returned as missing (`:495`), and the reply built at `:148` is `{"entities":null,"missing":[{"id":"","reason":"error"},…]}` (`batchMissingReason` `:161-163`; `graph.MissingError`, `graph/query_batch_types.go:34`) |
| batch, prefix | `:139` (batch), `:329` (prefix fetch) → `classifyEntityQueryError` `:443-450` | state error passed, else `ClassifiedCode(Transient, internal)` |
| prefix | `:223` | `Classified(Transient)`, no code (max payload) |
| prefix | `:240` / `:243` | `Classified(Invalid)` no code (decode) / `ClassifiedCode(Invalid, invalid_request)` (both set) |
| prefix | `:248`, `:253` | validator errors returned raw (`entity_id_prefix_invalid`, `entity_id_pattern_invalid`; spec `:117-119`) |
| prefix | `:270`, `:282` / `:291` | `Classified(Transient)` no code / `ClassifiedCode(Transient, internal)` |
| prefix | `:305` | `Classified(Invalid)`, no code (cursor) |
| prefix | `:363`, `:389`, `:435`, `:438` (and `:406` passing them on) | unclassified `fmt.Errorf` |
| prefix | `:418-420` | `ClassifiedCodeDetail(Invalid, "response_too_large", …)`, a literal |
| batch/prefix | `:626` | `ClassifiedCode(Fatal, graph_state_reset_required)` |

The helpers exist but `query.go` does not call them: `rejectInvalid` (`mutation_runtime.go:214`),
`rejectInvalidDetail` (`:218`) and `rejectInternal` (`:222`, class transient, code `internal`). The second spelling of
`response_too_large` is natsclient's unexported `natsclient/request.go:20` — `const errorCodeResponseTooLarge =
"response_too_large"`, used at `:383`. On the wire an uncoded error carries no `X-Error-Code` header
(`natsclient/errors.go:174-175`).

### #302: what `messagesProcessed` counts

| Lane | Site | Counts |
| --- | --- | --- |
| fact (stream) | `component.go:2160` in `mergeEntityOnLane` (`:2073`) | +1 per input whose replace succeeded, including `revision == 0`, which wrote nothing (`:2163-2167`); bytes `:2161` |
| fact, hierarchy | `createEntityWithReceipt` `:2276` via `hierarchyStore.CreateEntity` (`:394-395`) → `CreateEntity` `:2041` | +1 per container born |
| in-process `CreateEntity` | `:2276` | +1 |
| `entity.create` | `canonical_mutations.go:253` → `recordCanonicalEntityCommit` `:673-678` (`:674`) | +1 when applied |
| `entity.reconcile` | `:326` | +1 when applied; 0 when unchanged |
| `triple.append` | `addTriplesLane` `component.go:2432` | 0 (errors only, `:2515`, `:2521`) |
| `entity.delete` | `deleteEntityAtRevision` `:2314` | +1, no bytes |
| any refusal or failure; queries | — | 0 |

Search: `grep -n 'messagesProcessed' *.go | grep -v _test` finds `canonical_mutations.go:674` and `component.go:516`,
`:885`, `:2160`, `:2276`, `:2314`. `keyed_ingest.go` has none. No site counts input messages.

- Reader: `DataFlow` `component.go:883-920`. `MessagesPerSecond = messages/uptime` (`:900`).
  `ErrorRate = errors/messages` (`:902-903`). The `c.errors` writers count per input message (`:1527`, `:1558`) and
  per write (`:2156`, `:2259`, `:2270`, `:2310`, `:2515`, `:2521`).
- Per-message observations that already exist: `keyed_ingest.go:294` (`processingDuration.Observe`, every input
  that reaches apply), `:338` (`recordApplied`, applied inputs only), `component.go:1536` (`ingestLag`), and
  `keyed_ingest.go:286` (`redeliveriesDropped`).
- Readers of `component.FlowMetrics` in this tree: none in production. `git grep -n 'FlowMetrics\|DataFlow()'` finds
  the interface `component/discovery.go:28`, the type `:83`, the graph-ingest implementation, and test fakes. At the
  pin: `service/component_manager.go:1374` (`status.DataFlow = managed.Component.DataFlow()`) fills
  `ComponentStatus.DataFlow` (tag `"data_flow"`, `:1388`), and nothing reads that field. The only callers of
  `GetComponentStatus`, `service/component_manager_http.go:337` (`list`) and `:460` (`status/`), build their own maps
  (`name`, `state`, `type`, `enabled`, `healthy`, `last_error`, `error_count`, `uptime_seconds`, …) without it;
  `grep -n -i 'dataflow\|data_flow\|messages_per'` over that file finds nothing, and `grep -rn '\.DataFlow()'` over
  the pin's Go files finds only `:1374` outside doc comments. No route serves `DataFlow` at the pin. `service` is
  admitted and arrives in change 3. Consumers: semsource's eight components return `MessagesPerSecond: 0` (e.g.
  `processor/git-source/component.go:583`). `grep -rn messages_per_second` over the six trees finds only semteams UI
  tests (`ui/src/lib/types/chat.phase6.test.ts:32`, `ui/src/lib/server/ai/toolExecutors.phase6.test.ts:154`,
  `ui/src/lib/components/chat/HealthStatusCard.test.ts:229`); production `ui/src/lib/server/ai/toolExecutors.ts:300-315`
  passes a generic `metrics` map from `GET /health/<name>` (`:274`) through and reads no field by name. `data_flow`
  matches only an unrelated design-document field (`ui/src/lib/components/board/ArtifactCard.svelte:263`).
- Test: `component_test.go:614` `TestComponent_DataFlow_ReturnsMetrics` asserts only that the rate moves
  (`:625-631`, comment "The count behind the rate is writes, not messages (#302)").

## 2. Every spelling of the facts being modeled

- **Outcome of one mutation.** The reply's `Outcome` (`graph/mutation_responses.go:109-120`); the re-decoded copy
  (`canonical_mutations.go:67-100`); the error code (`canonicalErrorOutcome` `:59`, and again in `meteredMutation`
  `mutation_runtime.go:56-60`); the commit bookkeeping (`recordCanonicalEntityCommit` `:673`). The pin has the same
  homes.
- **One refusal metered and logged.** `meteredMutation` (`mutation_runtime.go:41`); the outer wrapper
  (`canonical_mutations.go:34-45`); `recordAuthorityRejection` (`authority_gate.go:79`), shared by the RPC lane, the
  direct lane (`recordDirectAuthorityRejection` `:119`) and the fact lane (`keyed_ingest.go:262-263`).
- **Error code to metric label.** Rejection reason (`mutation_runtime.go:56-60`): code, else `internal`, with authority
  reasons replaced (`authority_gate.go:58-72`). Outcome label: code, else `internal` (`canonical_mutations.go:59-65`).
  The two series differ only for authority refusals.
- **"Messages processed."** `messagesProcessed` (six sites above); the `processing_duration_seconds` histogram count
  (`component.go:133`, observed per message); `entities_updated_total` (`:48`, incremented at `:2170`, `:2279`,
  `canonical_mutations.go:677`).
- **Query invalid/internal.** The table above, plus `rejectInvalid`/`rejectInternal`; `response_too_large` in
  `query.go:420` and `natsclient/request.go:20`.

## 3. Adjacent claims

### Open pull requests

`gh pr list --state open --json number,title,isDraft,changedFiles,files`: only #307 (this claim, 0 files). No overlap.
No other change is open under `openspec/changes/` (`ls`: `archive` only).

### Open issues naming the package

Queries: `gh issue list --state open --search "graph-ingest" --limit 100` (29 hits); `--search "graphingest"` (0).
Each hit's body and comments were grepped for package paths.

| Issue | Names the package by | Disposition here |
| --- | --- | --- |
| #302, #303 | title | retired (`Closes`, PR body) |
| #300 | title | section B is this change; C, D, F, G, H stay (owner ruling, #300 comment 6250544834) |
| #306 | `component.go`, `query.go:484` | stays (PR body: not in scope; the cache handle, #300 C) |
| #301 | `component.go` fields | stays (attached to the #282 lifecycle PR 2, comment 6250545567) |
| #291 | `Start` reconnect | stays (out of scope) |
| #216 | lost consumer | stays (out of scope) |
| #270 | `TestIntegration_TypedClientPreservesCommitAmbiguity` | stays (unexplained single failure; no `class:flake`) |
| #20 | `canonical_mutations.go:360` → `rejectInternal` (pin) | stays: an owner question (comment 6152619320). The change touches `rejectInternal` call sites. |
| #109 | `DataFlow` shell; #300 G and D's file seams | stays; ADR-108 (c) moves the counters into a shell (`docs/adr/108-processor-shell.md:80-81`) |
| #106 | `query.go:44` verb table | stays |
| #111 | hierarchy at birth (`component.go`) | stays |
| #33 | boot refusal text (`component.go:739`, old line) | stays |
| #134 | `component.go` hierarchy config | stays |
| #141, #195, #196 | adoption issues; graph-ingest's sites already done | stay |
| #103 | `graph.query.*` envelope; graph-ingest's replies are bare | stays (change 4) |

The rest of the hits name the word, not the package: #107, #15, #198, #110, #16, #94, #95, #96, #7, #9, #24 (no
package path in body or comments).

### Specs, ADRs, ledger

- `graph-transport-boundary` spec `:155-169` (outcome metric, ruling L); `:113-139` (prefix codes `invalid_request`,
  `entity_id_pattern_invalid`); `:151` (`{"entities":null}` for a pattern with no match).
- `component-registration` spec `:82`, `:97` (`DataFlow` is part of `Discoverable`); `component/README.md:138`.
- ADR-108 (c): the shell computes `DataFlow` from messages, bytes, errors and last activity. ADR-109: no text on
  `DataFlow`, `ErrorRate`, outcomes or the metric (`grep -n -i 'DataFlow\|ErrorRate\|messagesProcessed\|outcome'
  docs/adr/109-ready-after-boot.md`: none).
- README metric table: `processor/graph-ingest/README.md:161-162`.
- Ledger: `docs/admission-ledger.yaml:2280-2327`; coverage floor `scripts/cover-check.sh:31`
  (`"processor/graph-ingest merged"`).

## 4. Readers of the surface (#190)

No exported symbol of `processor/graph-ingest` is removed by section B's shape. Every function in the wrapper,
re-decode and query helpers is unexported (`grep -n '^func [A-Z]\|^func (c \*Component) [A-Z]'` over the touched
files lists only `component.go` symbols). Exported surface touched:

| Symbol | Reader | Kind |
| --- | --- | --- |
| `Component.DataFlow` / `component.FlowMetrics` | interface `component/discovery.go:28`; no production reader in this tree; at the pin only `service/component_manager.go:1374` calls it, filling `ComponentStatus.DataFlow`, which no handler or other code reads (`service` admitted, change 3) | API: `component-registration` spec `:82`; meaning not specified anywhere |
| `graph.MutationOutcome` and its four values | `internal/graphmutation/client.go:94-220`; `pkg/projection/mutation_client.go:222-224`; semconnect `gateway/cs-api/graph_mutations.go` | API: spec `:155`; ruling L keeps it |
| `graph.ErrorCode*` used as labels | handlers above; consumers classify on them (semconnect `systems_post.go:463`) | API: `graph/mutation_responses.go:8-16` |
| metric `semengine_graph_ingest_graph_mutation_outcomes_total` | none in this tree's production code or docs except README `:162` and spec `:158`; none in the six consumer trees (`grep -rln graph_mutation_outcomes`: none) | contract: spec `:155-169` |
| metric `…_mutation_rejections_total` | semsource `internal/governance/live_graph_integration_test.go:188`, `:263` (asserts zero, `semstreams_` prefix); semconnect qualification evidence script | contract: README `:161` |
| batch/prefix error codes on `graph.ingest.query.*` | semsparql `reader/nats.go:72` (subjects from `graph.QueryVerbs()`), `:120` (`entity_id_pattern_invalid`), `:176` and `server/server.go:768-786` (code, or a fallback when empty); semsource `processor/supersession/prefixquery.go:40` (`response_too_large`, via `graph.query.prefix`) | API: spec `:113-151`; ADR-060 at the pin |
| `natsclient.errorCodeResponseTooLarge` | unexported; `natsclient/request.go:383` | — |

Unexported code whose only callers are tests (#186): `recordCanonicalResponse` (production caller `:47`; test callers
`canonical_mutations_test.go:879`, `:895`). `meteredMutation`'s only production caller is `:32`; tests call it on 10
lines (`mutation_rejection_metric_test.go:32`, `:54`, `:81`, `:98`, `:118`; `structural_predicate_gate_test.go:129`,
`:174`, `:213`, `:254`, `:321`).

## 5. Problem shape

- **Classified refusal plus observed signal, in one place.** The closest instance is `recordAuthorityRejection`
  (`authority_gate.go:79`), "one home" for three lanes, whose doc says that is what makes "metered exactly once"
  true (`:105-117`).
- **Record the outcome where it is decided, not by re-reading the reply.** Instance in this package:
  `recordCanonicalEntityCommit` (`canonical_mutations.go:673`), called at each handler's success return (`:253`, `:326`).
- **Count each input once.** Instances: `processingDuration.Observe` (`keyed_ingest.go:294`) and `recordApplied`
  (`:338`) in `applyIngest`, one per message. At the pin, graph-index spells `messagesProcessed` at six sites
  (`processor/graph-index/component.go:1597-2178`), the shape ADR-108 consolidates.

## 6. Pin decision records

No package is carried from the pin. Pin ADRs that govern what this touches (searched for `meteredMutation`,
`mutation_rejections`, `graph_mutation_outcomes`, `DataFlow`, `messagesProcessed`, `response_too_large` across
`docs/adr` at `8b99efe9`):

- ADR-055 §3: the rejection metric comes from the `meteredMutation` wrapper around every mutation handler (`:47-48`,
  `:526-527`).
- ADR-056: `mutation_rejections_total` is the live signal (`:71-74`).
- ADR-060: unified RPC error contract; class plus non-empty code, with the code on `X-Error-Code` (`:145`, `:245-254`).

None is ported here. In this repository: ADR-108 (c) and ADR-112, the library-statement rule; no nats.go behaviour
is stated above beyond the code read.

## 7. Tests that pin today's layers or labels

- The re-decode layer: `canonical_mutations_test.go:867` `TestCanonicalAppendOutcomeMetricCountsEachSubjectResult`
  and `:890` `…RejectsFailureWithoutDetail` feed hand-built reply JSON into `recordCanonicalResponse`.
- WARN 2: `:788` `TestCanonicalMutationOutcomeMetricAndRevisionMismatchLog` asserts the text "graph mutation revision
  mismatch" (`:810`), and nothing about a WARN count.
- WARN 1: `structural_predicate_gate_test.go:157` asserts "graph mutation rejected".
- No test counts mutation WARN lines. `grep -n strings.Count *_test.go` finds counts in `config_fields_test.go`,
  `guard_generation_test.go`, `guard_record_test.go`, `keyed_ingest_test.go`, `poison_scoping_test.go` and
  `query_contract_guard_test.go`. The closest precedent is `keyed_ingest_test.go:270`, which counts fact-lane WARN
  lines (`strings.Count(logLine, "level=WARN")`, one per rejected message).
- Labels through the production chain: `:821` `TestCanonicalMutationOutcomeLabelIsTheClassifiedCode`
  (`message_type_unregistered`, `invalid_request`; checks that `internal` and `invalid` do not move).
- Inner wrapper only: `mutation_rejection_metric_test.go:26`, `:50`, `:76`, `:93`, `:110`;
  `structural_predicate_gate_test.go:129`, `:174`, `:213`, `:254`, `:321` (#139 B: these miss WARN 2).
  `structural_gate_wire_integration_test.go:26` names the chain in a comment only.
- No series before `Start`: `failed_start_callbacks_integration_test.go:125`.
- Query codes: `query_prefix_unit_test.go:34`, `:231`, `:350`, `:387`; `query_prefix_integration_test.go:116`
  (`IsInvalid` only, cursor), `:166`; `query_prefix_regression_test.go:170`; `indexing_profile_test.go:124`;
  `query_prefix_unit_test.go:492` (`{"entities":null}`). The `response_too_large` literal also appears in
  `commit_ambiguity_integration_test.go:267` (natsclient's code on a mutation reply).

## Adopter seams

- **Operators reading `graph_mutation_outcomes_total`.** The label values are those in the table in section 1. A
  successful reply whose re-decode fails is the only way a success counts as `internal` today. No reader of the
  metric was found in this tree or in the six consumer trees.
- **Log readers.** A revision mismatch logs two lines with different attribute sets (subject/reason/error versus
  operation/entity_id/expected_revision). Only the second carries the expected revision as a field.
- **`DataFlow` readers.** None today: no production reader in this tree, and at the pin `service` fills
  `ComponentStatus.DataFlow` but no route or other code reads it. Whoever adds the first reader (change 3's
  `service`, or the #109 shell) inherits these units.
  `MessagesPerSecond` is per write, append is uncounted, and `ErrorRate` mixes the two units. What they should have
  to know: nothing. Today they cannot learn the unit from any doc (no spec line states it).
- **Query callers (semsparql, semsource; graph-query in change 4).** They branch on `ce.Code`. Batch and prefix
  invalid and transient errors carry no code today, so semsparql reports its fallback HTTP code
  (`query_execution_error`, `upstream_unavailable` or `internal_error`). Any code added to those paths changes that
  field.
- No new exported symbol, config key, subject or bucket is named by the scope text.
