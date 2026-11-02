# Design: `processor/graph-query` step 1, direct lookups

Base `4368d0b`; pin `8b99efe9`, probes P1-P6 in `inventory.md` § 1. Binding: PR #504 comments 6582751320, 6585009466.
The inventory's third round was not re-reviewed; cites used here were re-read. Steps: `split.md`.
**Goal.** Direct lookups on `graph.query.*`: never a wrong success, always the responder's refusal. **Non-goals:**
steps 2 to 5; metrics (all 15 pin collectors read community or search paths, `pin:metrics.go:62-161`).

## D1. One verb table, marked

The pin kept an unexported table beside a router (`pin:query.go:49-66`, `router.go:16-43`). Options: (a) extend
`graph.QueryVerbs()`; (b) a table in graph-query: a second declaration, which the reserved-subject guard cannot read
and which D20 rules out; (c) a doc comment: no test can pin it. **(a).** New fields `Enveloped bool` and `Reply
reflect.Type` (the reply body, or `Data`'s type when enveloped, so neither restates the other; a string would drift).

| Verb on `graph.query.` | Answers from | Enveloped | `Reply` | Producer |
| --- | --- | --- | --- | --- |
| `entity`, `batch`, `prefix` | `graph.ingest.query.` same name | no | `ExactEntity`, `EntityBatchResponse`, `PrefixQueryResponse` | none |
| `entityByAlias` | `alias`, then ingest `entity` | yes | `ExactEntity` | `graph-query` |
| `relationships` | `outgoing` or `incoming` | yes | `[]RelationshipEdge` | `graph-query` |
| `byName` | `graph.index.query.byName` | yes | `NameData` | `graph-index` |

Ingest rows are bare; index rows enveloped. The pin has no `graph.query.predicate*` verb, so ruling 1's "later" adds no
row. Consumer verbs stay outside the table. *Proof:* `TestQueryVerbsDeclareEachRespondersVerbs`;
`TestGraphQueryRepliesDecodeByTheTable` decodes a real reply of each verb by its marks, unknown fields refused, and
fails if `entity` is enveloped or `entityByAlias` bare. *Adopter:* decodes by verb; SemSource can drop the list its
collision test pins by hand. *New exports' readers:* graph-query reads the responder name, `RelationshipEdge`,
`MinRevisionField`; graph-index the field and the code; a product decodes by the marks (contract line:
`graph-transport-boundary`); `composition` the finding.

## D2. Forwards are forwards

Owner ruling (Q3): `entity`, `batch`, `prefix`, `byName` pass request and reply bytes unchanged. Accepted cost: a
broken authority reply reaches the caller. A refusal returns unwrapped, so class, code and detail survive (ADR-060). No
responders is not left to the library, whose class is not promised (`natsclient/errors.go:217-218`, ADR-112):
graph-query refuses transient `internal`, naming the subject. *Proof, real broker:*
`TestForwardReturnsReplyBytesUnchanged`, `TestForwardedRefusalKeepsClassCodeDetail`, `TestNoResponderIsTransient`.

`relationships` must pick a subject, so it decodes. `direction` is `outgoing`, empty (outgoing) or `incoming`; anything
else is `invalid_request`: the pin read `both` and any typo as outgoing (`pin:query.go:371`). Rows keep the pin's keys
(`pin:pkg/fusion/fusionnats/client.go:508-512`, fusion reads them); no rows is `[]`. *Proof:*
`TestRelationshipsEnvelopeKeepsIndexRevision`, `TestRelationshipsRefusesUnknownDirection`.

## D3. `min_revision` lands here, in graph-index

A separate graph-index PR would have no reader; leaving it out makes four verbs ignore a field they accept. **In step
1.** One embedded `graph.MinRevisionField` (json `min_revision`, 0 means none; D18). `queryRevision` takes it and all
eight handlers pass it (`processor/graph-index/query.go:134-614`); `graph.PredicateListQuery` must embed it because its
decoder refuses unknown fields (`:398-399`). Order: request validation, `index_not_ready` (ADR-109), the comparison.
Refusal: transient, `revision_not_reached`, detail `{"indexed_revision": R}`. Bare verbs forward the field unread, met
by construction. A recreated bucket restarts revisions: #15.

*Invariant:* a success to `min_revision` M has `indexed_revision` >= M (`graph-index`, "An unmet `min_revision` is
refused"). Examples suffice over generated checks: one comparison, boundaries 0, R, R+1. *Proof:*
`TestMinRevisionBoundary` on all eight verbs; `TestMinRevisionRefusalThroughGraphQuery`; `TestReadYourWrites` on real
ingest and index: never a success below the written revision, and the first success shows the write.

## D4. Alias refusals (ADR-114)

The alias verb's one non-refusal miss is a success with `canonical_id` null (`graph-index/query.go:272`). So: holder,
read that ID; no holder, read the input as an ID; **any** refusal reaches the caller unchanged. `min_revision` goes on
the alias request; the envelope carries the alias reply's revision. The pin swallowed it all (P5, P5b).

Inventory correction: P5's "caller sees not-found" came from a fake. The real entity verb refuses a non-ID as
`invalid_request` (`graph-ingest/query.go:79-82`, pin `:82`), so an unknown alias `sn-1` would read as malformed.
**With no holder and an input that fails `ValidateEntityID`, refuse `entity_not_found`, no second request.** *Proof:*
`TestEntityByAlias`: holder; holder whose entity is deleted; ID input; unknown non-ID; `alias_ambiguous`,
`index_not_ready`, `internal`, `revision_not_reached`; no responder.

## D5. `kv_revision` against SemConnect's `kvRevision` (left to this design)

This tree writes `kv_revision` (`graph/exact_entity.go:19`; spec "Authority reads are not enveloped"; every other key
is snake case). SemConnect `gateway/cs-api/systems.go:808` at `d0d06e00` decodes `kvRevision` in a private struct,
though its ADR-004 § 3 says exact reads decode `graph.ExactEntity`. Code search finds no other production JSON reader
(case-insensitive, default branches). **Propose SemConnect changes one struct tag at its cutover.** Refused:
re-spelling on `graph.query.entity` (breaks D2) or writing both keys (a permanent second spelling). It fails loudly:
`systems.go:814` turns revision 0 into fatal `internal` (settles inventory § 11's "not measured"). SemConnect is
read-only from here; the owner carries it across.

## D6. Second responder on a declared verb (#106)

Options: (a) a `composition.Analyze` finding, boot refused at `service/component_manager.go:328`; (b) graph-query
probes each subject before subscribing; (c) `natsclient` tracks subjects, against "natsclient passes nats.go through".
**(a).** A NATS-request input port overlapping a table subject is `request_subject_conflict` (error) when its
`FactoryIdentity` is not the verb's responder, or on the later instance when two are. graph-query's `graph.query.*`
passes; `graph.query.sources` overlaps nothing. **Plain limit:** it sees declared ports only. SemSource's verbs
subscribe with no port (`processor/source-manifest/component.go:176` at `e4febc0d`), so the one real collision on
record, its old `graph.query.summary` (`:34-41`), would pass. See open question 1. *Proof:*
`TestAnalyzeReportsSecondResponder` (other factory, two graph-query, consumer verb allowed),
`TestBootRefusesSecondResponder`.

## D7. Shell and configuration

`component.Lifecycle`; handlers bound by walking the table, a declared verb with no handler failing `Start`
(`graph-index/query.go:53`). Nothing outlives a handler, so no run context yet (ADR-113 applies from step 3). Input
port as the pin requires (P3; both consumers send it). Decoding is strict (P2; `graph-index/component.go:107`). Keys:
`ports`, and `query_timeout_ms` replacing `query_timeout`, whose advertised `"5s"` the pin refused (P1); default 5000,
no consumer sets it. SemConnect's `request_timeout`, `max_traversal_depth`, `max_traversal_nodes`
(`deploy/semstreams.json`, read by nothing at the pin) refuse at boot, as `batch_size` did. No test hook, no requester
interface (S5, S21). *Proof:* lifecycle suite with the failed-cleanup branch;
`TestGraphQueryServesExactlyTheDeclaredVerbs`; `TestUnknownConfigKeyIsRefused`; `TestQueryTimeoutIsRead`.

## Bounded port scope, issues, ADRs, overlap

**Shape sweep** (`inventory.md` § 8). Required, done here (`AGENTS.md` rows, ADR-108, ADR-114, surface audit, build):
S4, S5, S6, S9 for reserved subjects, S10, S17, S24, S25. Optional, gone with D2 and D7 at no cost: S19, S21, S23, S26.
Code not carried here, step in `split.md`: S2, S7, S8, S11-S16, S18, S20, S22. Clean: S3, S27, S28. Dead surface
dropped: `router.go`. Pin tests rewritten on a broker (`split.md`). **Issues:** retires none. Stay, required later:
`#103` (step 5; marker `#15`), `#106` (rows, 5), `#405` (4; ADR-114 half here), `#406`, `#31`, `#477` (3), `#479` (4),
`#105` (2), `#111` (2, 4, 5; item 4 change 7). Stay, optional or not this package's: `#68` (no collector yet), `#195`,
`#196`, `#16` (the table widens it), `#15`, `#108`, `#155`, `#32`, `#485`-`#487`, `#506`, `#484`, `#9`, `#500`. Names
the word only: `#7`, `#24`, `#109`, `#300`, `#350`. **ADRs:** ADR-060 ported, with a note that ADR-091 amends it and is
not here; ADR-109, -113, -114 applied; D18 amended by ruling 1; ADR-071 step 2, ADR-087 D5 step 3. **Overlap:** draft
PR `#505` merges first (`AGENTS.md`, rules index, other rows); this step updates the reserved-subject row. The
`query-pattern` skill grows.

## Open questions (owner)

D6's limit: accept declared-only, or add (b)? **Recommend accept:** (b) misses a later squatter; no SemSource verb
collides today. D5: accept, and who tells SemConnect. D7: the three refused keys. D4's `entity_not_found`. The
proposal's new exports.
