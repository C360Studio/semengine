# Design: setup-04b-02-lexical-processor

## Context

Facts are in `inventory.md` (INVENTORY PASS at `5582eb9e`, one pin fixed in `f7a1b001`), cited by section. Nothing
feeds `graph/lexical` (§1). Four rules decide what an entity's text is, and none gives the Name/Body split
`lexical.Document` takes (§2a). Two mechanisms keep in-memory state current from a KV watch: `graph/follower`, with
three plug-ins, and `pkg/graphview`, used by `output/websocket` and `VectorCache` (§2b). The only text verb is the
embedding `search`, whose reply carries a cosine `Similarity` and no pre-limit count (§2c, §3). SemSource offloads
every doc passage body and every code body (§9). The index costs about 60 bytes per held word (#623, §2c). No pin
package is carried (§8). Terms: *apply Value* and *offloaded body* as in the inventory's header.

## Goals / Non-Goals

Goals: a tier 0 component that keeps a word index current from `ENTITY_STATES`, rebuilt on every start, gated until
the rebuild has caught up, answering one verb whose reply is what change 3 needs; one text rule shared with
graph-embedding; measured memory and rebuild cost; the smoke check #628 asks of this change.

Non-goals: change 3 and 4 work (proposal); a stored index; the per-entity word record (D6 shows where it goes).

## Decisions

**D1. Placement: its own component, `graph-lexical`.** (a) Own component, fourth follower plug-in, key and responder
`graph-lexical` (chosen). Cost: a product registers one more factory and config block (§9 item 1a); one key SemSource
does not read yet (§9). Gain: its memory, rebuild time and faults are its own; a word-index fault withholds only its
verb; it runs at every tier while graph-embedding becomes tier 1 only (plan item 4). (b) Inside graph-index: its
apply style is re-read through a keyed pool into KV buckets (§2b), so the index would be a second apply path in one
component; a word-index fault would withhold all eight structural verbs (§4); it has no store registry (§2a).
(c) Inside graph-embedding: the plan turns it off at tier 0 and allows no buckets or worker pool there; keeping it on
in a "lexical mode" is the configuration flag standing in for a type that ADR-108 D6 rejects. (d) Do nothing: tier 0
stays on hashed vectors (§3). Forecloses: nothing; a later merge into another component is a composition change.

**D2. Mechanism: a follower plug-in that applies Value inline.** (a) `graph/follower`, applying `Entry.Value` on the
follower's goroutine with a per-key revision guard, graph-clustering's shape (§2b) (chosen). It gives the readiness
key, the floor watermark that makes `indexed_revision` true, the reset latch, lost-feed, order and stream-generation
checks, and `QueryReady` (§2b table), and holds nothing beside the index and one revision per key. Because `Complete`
is called only after `Put` returns, `Indexed()` is a lower bound on what a search sees. (b) `pkg/graphview`: no
readiness key, reset, order or generation check (§2b table), and it holds a decoded copy of every entity beside the
index (its size not measured). (c) Both, as graph-embedding does: two gates for one answer (§7). (d) Re-read
through a pool, as graph-index: one `Get` per entity on every rebuild, and a second apply path. A repair pass for
unread bodies (D5) reads current truth and applies only above the key's revision, so ADR-108's warning against two
paths applying out of order does not arise: the guard decides, not arrival order.

**D3. One text rule: `graph/entitytext`.** (a) A new pure package (it imports `graph`, `vocabulary`, `storage`; no
NATS client: `go list -deps ./storage | grep -c nats-io` is 0) holding graph-embedding's suffix rule, its defaults,
its cap and its body read, moved, not copied (chosen). Selection is today's: string values whose predicate ends in a
configured suffix, relationships skipped (§2a). Each selected value, in triple order, is marked Name when its
predicate is a vocabulary label predicate (`vocabulary.DiscoverLabelPredicates`, §2a) or ends `.name` or `.title`,
and Body otherwise; the offloaded body is Body, after them. graph-embedding joins the selected values in triple
order with one space, as today, so its bytes, dedup keys and vectors do not change; graph-lexical builds
`lexical.Document` from the marks. Config is one type,
`entitytext.Config` (`text_suffixes`, `max_text_len`, default 4,000 characters), decoded by both components with the
keys graph-embedding has now. (b) Put it in `graph/lexical`: graph-embedding, the tier 1 home, would import a word
index to learn what text is. (c) Write a second extractor in the processor: interpreter number five (§2a). The other
rules, graph-clustering's every-string-value and graph-query's predicate-plus-value, are adopters for changes 4 and 3
(adoption list below). Adopter seam: a product that customises `text_suffixes` sets it on each component it runs;
SemSource builds both configs from one list already (§9).

**D4. The verb: a new subject, `graph.lexical.query.search`.** (a) New row, responder `graph-lexical` (chosen).
Request `graph.LexicalSearchRequest{Query, Limit, Scope, MinRevisionField}`, decoded strictly; limit 0 means 10, 1 to
100 accepted, others refused (the existing `DefaultSimilarityLimit`/`MaxSimilarityLimit`, one spelling of the bound);
the query goes through `lexical.ParseQuery`, whose refusals answer `invalid_request` naming the rule (#624 stays with
change 3). Reply, enveloped: `graph.LexicalSearchData{Results []LexicalHit, Matched int, UnreadBodies int}`,
`LexicalHit{EntityID, Score}`. `Matched` is every match in scope before the limit, which change 3 maps to
`EntitiesTruncated`; `Scope` is prefixes, which is local search; empty scope is global search (§3). The envelope
carries `indexed_revision` = the follower's floor, and `min_revision` is honoured as graph-index's are (§2c), so a
caller can wait for its own write. (b) Serve `graph.embedding.query.search`: two responders on one subject at tier 1,
and a reply type whose `Similarity` would carry an unbounded BM25F score. (c) Make `semantic` forward here when
embeddings are off: one verb with two reply meanings chosen by config, and every SemSource NL answer changes with no
code change (§9 item 2). Fusion: its doc (`pkg/fusion/retrieval.go:57`, "cosine similarity") stays true in this
change, because no BM25F score reaches `Similarity`. When fusion resolves through the word index (open question 2),
the doc becomes "the resolve mode's own score, on that mode's scale", and the node names the mode. Adopter seam: a
caller that wants word search names the new verb; one that does nothing keeps today's answers.

**D5. Readiness, restart, offloaded bodies.** On start the follower replays every entity and the verb answers
`index_not_ready` until the replay is applied, `graph_state_reset_required` after an undecodable entity, and
`index_not_ready` after a lost feed (§4). Each entity is applied inline: decode, select text (D3), read the offloaded
body when its store is registered, `Put`, `Complete`. A body read runs under a 10-second deadline, below the follower's
30-second degraded rule (§7). A store that honours its context cannot stall the replay past it; whether the NATS object
store's read honours it at the pinned nats.go is not promised until task 2.1's probe says so. A body that cannot be read
(no store registered, read error, deadline) is left out: the entity is indexed on its inline text, counted in
`UnreadBodies` and in the envelope's failure reasons as `body_unread`, and does not withhold the verb. An entity the
index refuses (`lexical.ErrInvalidEntityID`) is counted `entity_refused` and does not withhold either, as
graph-clustering's refusals do not (`processor/graph-clustering/component.go:201-203`). A repair pass every 30 seconds
reads those entities again until the body reads or the entity changes. Rebuild cost: decode and `Put` at about 16,500
entities a second on one goroutine (#623), plus one body read per offloaded entity, unmeasured (§10); task 2.6 measures
it. The 10-second deadline lives in the shared reader, so graph-embedding's Worker gains it too, which retires #551.

**D6. The per-entity word record: an extension point, not built.** Where it goes: the apply step's body branch.
What it needs: `lexical.Index.Words(id)` and `PutWords(id, name, body)` (change 1 D7), a tokenizer version constant,
and a bucket `LEXICAL_WORDS` keyed by entity ID holding `{source revision, tokenizer version, name words, body
words}`, written after `Put`. On a start, an entity whose record matches its revision and the tokenizer version is
restored with `PutWords` and its body is not read. Nothing in this change's surface blocks it: the verb, reply, key
and config do not change. When: task 2.6's measurement against the budget in open question 1.

**D7. Memory (#623): measure, then change the layout.** The index holds about 60 bytes per word, about 9.5 GB at a
million entities of 157 words (§2c). A SemSource passage near its 2,000-byte ceiling would be about 18 KB (§10,
arithmetic). (a) Before the processor loads real data, re-run #623's harness, change the private layout to word
numbers (one table from word to number; sequences and postings by number, the layout change 1's `sequence-probe`
measured at 472 B per 20-word entity), and re-run it (chosen; task 2.2). (b) Accept 60 B: a 32k-entity SemSource
corpus near 0.6 GB on an edge machine. No caller, test or scorecard answer changes (change 1 D7). Memory is reported
as a gauge of held entities and held words, so an operator sees growth.

**D8. Smoke (#628, first item).** `stackConfig` gains `graph-lexical`; `componentFactories` and `indexKeys` gain it
(§4). A new check asks the verb for a word only one fixed entity's name holds, a word every name holds, and a deleted
entity's word; it waits on the reply's `indexed_revision` reaching the `ENTITY_STATES` revision (the ordering event),
then requires the live holders only. The restart tests compare the whole reply before and after restart, rank and
score, which BM25F over the held set makes exact (R1). The graph-only and no-embedder compositions are unchanged.

**D9. Docs.** The processor README says word search helps entities with text only; sensor and drone telemetry and
other numeric entities gain nothing and are reached by prefix, predicate, relationships, name and alias, and group
(the list in `graph/lexical/README.md`, "Who word search helps"); it states the verb, the body rule (D5), memory
(D7) and what a restart costs.

Adoption list (D3 establishes a primitive): `graph/clustering/terms.go:57` (change 4),
`processor/graph-query/localsearch.go:95` (change 3, replaced by the index), `processor/graph-embedding/hop1.go:228`
and `graph/embedding/worker.go:744` (this change). Filed as one tracking issue at landing.

## Risks / Trade-offs

- Rebuild time grows with offloaded entities; unmeasured until task 2.6 (open question 1).
- An unread body lowers that entity's rank until the repair pass reads it; counted, not hidden.
- Search holds the read lock for its whole scoring, so a common-word query on a large index delays writes (§2c).
- Each process holds its own index; two instances answer from two copies built from one bucket.
- graph-embedding is edited (D3); its tests and a byte-equality test guard its output. #606 batch 6 is not open.

## Migration Plan

None for data: nothing is stored. A product adds `graph-lexical` to its composition and registers its factory.
graph-embedding's config keys and output are unchanged.

## Open Questions (for the design read; the owner rules)

1. Offloaded bodies on every start. Recommended: read them every start (SemSource's passages are all offloaded, so
   without them its doc search sees titles only, §9), with the 10-second deadline and the repair pass; set a rebuild
   budget now (suggested: 60 s for 32k offloaded entities on one machine) and build the word record (D6) as the next
   change if task 2.6 exceeds it. Alternative: inline text only until the record exists.
2. SemSource's natural-language path. Recommended: no silent switch. `semantic` keeps meaning embeddings; word
   search is the new verb, which SemSource's runner can call now (#620); fusion resolves natural language through
   the word index at tier 0 from change 4, when the embedder leaves tier 0, with the score doc of D4.
3. `docs/setup-plan.md:429-430` says SemSource's mainline stays on SemStreams until tier 1; SemSource `main` builds
   on SemEngine (§9). Recommended: correct the sentence in this change's docs task, on the owner's word, since it is
   the approved plan.
