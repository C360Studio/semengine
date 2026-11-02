# Design: `processor/graph-query` step 1, direct lookups

Base `4368d0b`; pin `8b99efe9`, probes P1-P6 in `inventory.md` § 1. Binding: PR #504 comments 6582751320, 6585009466.
Inventory round 3 unreviewed; cites used here re-read. **Goal.** Direct lookups: never a wrong success, always the
responder's refusal. **Non-goals:** steps 2 to 5 (`split.md`); metrics (all 15 pin collectors serve later steps).

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

Ingest rows are bare; index rows enveloped. The pin has no `graph.query.predicate*` verb, so ruling 1's "later" adds
no row. Consumer verbs stay outside the table. *Proof:* `TestQueryVerbsDeclareEachRespondersVerbs`;
`TestGraphQueryRepliesDecodeByTheTable` decodes a real reply of each verb by its marks, unknown fields refused, and
fails if `entity` is enveloped or `entityByAlias` bare. *New exports' readers:* graph-query the responder name,
`RelationshipEdge`, `MinRevisionField`; graph-index the field and code; products the marks (contract line:
`graph-transport-boundary`); `composition` the finding.

## D2. Forwards are forwards

Owner ruling (Q3): `entity`, `batch`, `prefix`, `byName` pass request and reply bytes unchanged, so `min_revision`
reaches the index untouched (ADR-071's reasoning). A refusal returns unwrapped, so class, code and detail survive
(ADR-060). When a request reports no responders, whose class is not promised (`natsclient/errors.go:217-218`,
ADR-112), graph-query refuses transient `internal`, naming the subject; a server that reports nothing is a plain
timeout, also transient (`:334-339`). *Proof, real broker:* `TestForwardPassesBytesBothWays` (all four verbs, unknown
field, odd spacing, each direction), `TestForwardedRefusalKeepsClassCodeDetail`, `TestNoRespondersIsTransient`.

`relationships` takes `entity_id` and `direction` (`pin:query.go:359-360`) and forwards the request bytes unchanged.
`direction` is `outgoing`, empty (outgoing) or `incoming`; anything else is `invalid_request`: the pin read `both` and
any typo as outgoing (`:371`). Rows keep the pin's keys; no rows is `[]`; an index reply that does not decode or names
no `producer` (`{}` decodes; `NewQueryResponse` refuses an empty one) is transient `internal`. *Proof:*
`TestRelationshipsEnvelopeKeepsIndexRevision`, `TestRelationshipsRefusesUnknownDirection`,
`TestRelationshipsRefusesBrokenIndexReply`.

## D3. `min_revision` lands here, in graph-index

A separate graph-index PR would have no reader; leaving it out makes four verbs ignore a field they accept. **In step
1.** One embedded `graph.MinRevisionField` (json `min_revision`, 0 means none; D18). `queryRevision` takes it and all
eight handlers pass it (`processor/graph-index/query.go:134-614`); the exported `PredicateListQuery` (strict decoder,
`:398-399`) and `CompoundPredicateQuery` embed it too. Order: request validation, `index_not_ready` (ADR-109), the
comparison. Refusal: transient, `revision_not_reached`, detail `{"indexed_revision": R}`. Bare verbs forward the field
unread, met by construction. A recreated bucket: #15.

*Invariant:* a success to `min_revision` M has `indexed_revision` >= M (`graph-index`, "An unmet `min_revision` is
refused"). Examples suffice: boundaries 0, R, R+1. *Proof:* `TestMinRevisionBoundary` on all eight verbs;
`TestMinRevisionRefusalThroughGraphQuery` on `byName`, `relationships` both ways and `entityByAlias`, M far above R,
class, code, detail intact; `TestReadYourWrites` on real ingest and index: never a success below the written revision.

## D4. Alias refusals (ADR-114)

Request `{"aliasOrID": ..., "min_revision": ...}` (`pin:query.go:146`). The alias verb's one non-refusal miss is a
success with `canonical_id` null (`graph-index/query.go:272`). So: holder, read that ID; no holder, read the input as
an ID; **any** refusal reaches the caller unchanged. An alias reply that does not decode or names no `producer` is
transient `internal`, not "no holder" as at `pin:query.go:175`. The reply is `graph.QueryResponse` with the entity
verb's bytes unchanged as `data` (Q3) and the alias reply's revision. Bare at the pin, enveloped now. The pin
swallowed all refusals (P5, P5b).

Inventory correction: P5's "caller sees not-found" came from a fake. The real entity verb refuses a non-ID as
`invalid_request` (`graph-ingest/query.go:79-82`, pin `:82`), so an unknown alias `sn-1` would read as malformed.
**With no holder and an input that fails `ValidateEntityID`, refuse `entity_not_found`, no second request.** That
narrows ADR-114's "Not found keeps falling through" (`docs/adr/114`:113): valid IDs still fall through. The ADR
sentence is amended here. *Proof:* `TestEntityByAlias`. Real ingest and index: holder, ID input, unknown non-ID,
`alias_ambiguous`, `revision_not_reached`. Stub only for: `index_not_ready`, `internal`, undecodable reply, `{}`, no
responders, holder whose entity is gone. A later write to another entity, once a direct alias read shows it, proves
the envelope's revision is the alias reply's and not `kv_revision`.

## D5. `kv_revision` against SemConnect's `kvRevision` (left to this design)

This tree writes `kv_revision` (`graph/exact_entity.go:19`; spec "Authority reads are not enveloped"; every other key
is snake case). SemConnect `gateway/cs-api/systems.go:808` at `d0d06e00` decodes `kvRevision` in a private struct.
Code search finds no other production decoder of the key (case-insensitive, default branches); graph-gateway's schema
names it (below). **Propose SemConnect changes one struct tag at its cutover.** Refused: re-spelling (breaks D2) or
writing both keys (a permanent second spelling). It fails loudly: `systems.go:814` turns revision 0 into fatal
`internal`. SemConnect is read-only from here; the owner carries it across.

**graph-gateway.** SemSource composes it (`cmd/semsource/run.go:867`) and documents `/graphql`
(`docs/integration/m5-consumer-integration.md:178`). At the pin it unwraps by detection
(`gateway/graph-gateway/component.go:2059`, not ported), places reply bytes under a field (`:1527`), and its schema
names `kvRevision` (`:1869`). The pin's unwrap accepts only `data` and `timestamp`
(`pin:graph/query_contracts.go:89`), so a copy nests every enveloped answer one level deeper, by step 5 `summary` too.
Its `alias` argument reaches the handler as `alias`, which reads `aliasOrID` and refuses as empty (read). The ledger:
consumer-owned, "not exercised", drags agentic and `graph/inference`; no owner named.

## D6. Second responder on a declared verb (#106)

Options: (a) a `composition.Analyze` finding, boot refused at `service/component_manager.go:328`; (b) graph-query
probes each subject before subscribing; (c) `natsclient` tracks subjects, against "natsclient passes nats.go through".
**(a).** A NATS-request input port overlapping a table subject is `request_subject_conflict` (error) when its
`FactoryIdentity` is not the verb's responder, or on the later instance when two are. **Plain limit:** it sees
declared ports only. SemSource's verbs subscribe with no port (`processor/source-manifest/component.go:176` at
`e4febc0d`), so the one real collision on record, its old `graph.query.summary` (`:34-41`), would pass. *Proof:*
`TestAnalyzeReportsSecondResponder`, `TestBootRefusesSecondResponder`.

## D7. Shell and configuration

`component.Lifecycle`; handlers walk the table, and a declared verb with no handler fails `Start`. Nothing outlives a
handler, so no run context yet (ADR-113, step 3). Input port as at the pin (P3). Decoding is strict (P2;
`graph-index/component.go:107`). Keys: `ports`, and `query_timeout_ms` replacing `query_timeout`, whose advertised
`"5s"` the pin refused (P1); default 15000, above ingest's 10 s budget on `batch` and `prefix`
(`graph-ingest/query.go:109,:234`; gh#830), inside natsclient's 30 s handler budget (`entityByAlias`'s two hops can
reach it, still transient). This cures one hop: SemConnect's 5 s default (`gateway/cs-api/config.go:157`) and
SemSource's 10 s still time out a slow `batch`, loudly. No consumer sets the key. Notice for SemConnect:
`request_timeout`, `max_traversal_depth`, `max_traversal_nodes` (`deploy/semstreams.json`, read by nothing at the pin)
refuse at boot (`AGENTS.md` § 1). No test hook, no requester interface (S5, S21). *Proof:* lifecycle suite;
`TestGraphQueryServesExactlyTheDeclaredVerbs`; `TestUnknownConfigKeyIsRefused`; `TestQueryTimeoutIsRead`.

## Scope and questions

`split.md`, "Step 1 dispositions", holds every shape finding and open issue; this retires no issue. ADR-060 ported;
ADR-114 amended (D4). Draft PR `#505` merges first. Six owner questions: `owner-questions.md`.
