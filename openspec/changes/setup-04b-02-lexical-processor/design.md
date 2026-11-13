# Design: setup-04b-02-lexical-processor

## Context

Facts are in `inventory.md` (INVENTORY PASS at `5582eb9e`, one pin fixed in `f7a1b001`), cited by section, and in
two probes run for this revision under `notes/probes/`: `objectstore-watch-probe` and `body-size-probe`, each with
its source (`.go.txt`) and log. Nothing feeds `graph/lexical` (§1). Four rules decide what an entity's text is, and
none gives the Name/Body split `lexical.Document` takes (§2a). Two mechanisms keep in-memory state current from a KV
watch: `graph/follower` and `pkg/graphview` (§2b). The only text verb is the embedding `search`, whose reply carries
a cosine `Similarity` and no pre-limit count (§2c, §3). SemSource offloads every doc passage body and every code
body (§9). The index costs about 60 bytes per held word (#623, §2c). No pin package is carried (§8). Terms: *apply
Value* and *offloaded body* as in the inventory's header; a *body read* is one read of an offloaded body.

Revision 2 answers the design read on PR #629 (head `564ec192`): findings 1 to 8 and the owner's rulings there.

## Goals / Non-Goals

Goals: a tier 0 component that keeps a word index current from `ENTITY_STATES`, rebuilt on every start, gated until
the rebuild has caught up, answering one verb whose reply is what change 3 needs; one text rule shared with
graph-embedding; a first build whose time under a blocking store does not grow with the entity count; measured
memory and rebuild cost; the smoke check #628 asks of this change.

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

**D2. Mechanism: a follower plug-in that applies Value, with one body path.** (a) `graph/follower`, applying
`Entry.Value` with a per-key revision guard, graph-clustering's shape (§2b) (chosen). It gives the readiness key, the
floor watermark that makes `indexed_revision` true, the reset latch, lost-feed, order and stream-generation checks,
and `QueryReady` (§2b table). `Deliver` runs on the follower's goroutine (`graph/follower/follower.go:75-76`) and
does no I/O: it decodes, selects text (D3), `Put`s the inline text, and either completes the revision (no offloaded
body) or hands the entity to the body path (D5), which completes it. (b) `pkg/graphview`: no readiness key, reset,
order or generation check, and a decoded copy of every entity beside the index (§2b table). (c) Both, as
graph-embedding does: two gates for one answer (§7). (d) Re-read through a pool, as graph-index: one `Get` per
entity on every rebuild, and a second apply path.

Two writers touch an entity, `Deliver` and the body path, so one rule orders them (finding 1). A delivery at or below
the revision the index holds for its key is ignored. A body read for revision R is applied only if the key is still
at R when the read returns; otherwise it is dropped and R completes, because a newer delivery has queued its own
read, and a delete has removed the key. This is an order-dependent history over two paths, so a generated check
proves it (task 2.3): random interleavings of deliveries, deletes, body outcomes and late read returns, against a
reference model in which each live key holds its latest revision's inline text plus that revision's body when that
read succeeded. Named witness first: a read in flight while a newer delivery of the key is applied does not put the
older body back.

**D3. One text rule: `graph/entitytext`.** (a) A new pure package (it imports `graph`, `vocabulary`, `storage`; no
NATS client: `go list -deps ./storage | grep -c nats-io` is 0) holding graph-embedding's suffix rule, its defaults
and its body read, moved, not copied (chosen). It offers two views of one rule:

- `Values`: string values whose predicate ends in a configured suffix, relationships skipped, in triple order
  (§2a). graph-embedding joins them with one space, as today, so its bytes, dedup keys and vectors do not change.
- `Document`: Name is every value of a vocabulary label predicate (`vocabulary.DiscoverLabelPredicates`, §2a),
  whatever its suffix, and every suffix-selected value whose predicate ends `.name` or `.title`, each once; Body is
  every other suffix-selected value, then the offloaded body. graph-lexical uses it.

Label predicates are selected regardless of suffix for the word index only (finding 5): an entity graph-index finds
by name (`byName`) is then found by its name's words. graph-embedding keeps suffix-only selection, so its bytes do
not move; whether tier 1 should match is change 4's question. (b) Put the rule in `graph/lexical`: graph-embedding,
the tier 1 home, would import a word index to learn what text is. (c) A second extractor in the processor:
interpreter number five (§2a). graph-clustering's and graph-query's rules are adopters for changes 4 and 3.

The cap (finding 4). `max_text_len` keeps graph-embedding's meaning: the most characters of text taken, the tail cut
at a word. It applies to Body only; Name is never cut, and the inline Body values come before the offloaded body, so
a cut takes the body's tail. Defaults differ by component: graph-embedding keeps 4,000, its embedder's input limit;
graph-lexical takes 16,000. Measured on SemSource's source at `4ca2072c` (`body-size-probe.log`): of 4,766 Go symbol
bodies, a 4,000 cap cuts 34 (0.71%) and 2.27% of all code text; 16,000 cuts 1 and 0.04%; no doc fenced block
exceeds 1,507 characters, and passages are at most 6,000 bytes by SemSource's splitter. A cut is declared: a gauge
counts entities whose text was cut, and the component logs the first one. File paths (R3): deferred, stated. A path
is indexed when a product adds its path predicate's suffix to `text_suffixes`; no default changes, and SemSource's
`-test` question is measured with its runner (#620).

**D4. The verb: a new subject, `graph.lexical.query.search`.** (a) New row, responder `graph-lexical` (chosen).
Request `graph.LexicalSearchRequest{Query, Limit, Scope, MinRevisionField}`, decoded strictly; limit 0 means 10, 1 to
100 accepted, others refused (the existing `DefaultSimilarityLimit`/`MaxSimilarityLimit`, one spelling of the bound);
the query goes through `lexical.ParseQuery`, whose refusals answer `invalid_request` naming the rule (#624 stays with
change 3). Reply, enveloped: `graph.LexicalSearchData{Results []LexicalHit, Matched int, BodiesNotIndexed int}`,
`LexicalHit{EntityID, Score}`. `Matched` counts every match in scope before the limit, which change 3 maps to
`EntitiesTruncated`; `Scope` is prefixes, which is local search; empty scope is global search (§3).
`BodiesNotIndexed` counts, across the whole index, not the scope or the hits, the entities whose offloaded body is
not in the index now, for any reason in D5 (finding 7). The envelope carries `indexed_revision` = the follower's
floor, and `min_revision` is honoured as graph-index's are (§2c). (b) Serve `graph.embedding.query.search`: two
responders on one subject at tier 1, and `Similarity` would carry an unbounded BM25F score. (c) Make `semantic`
forward here when embeddings are off: one verb with two reply meanings chosen by config. Fusion: its doc
(`pkg/fusion/retrieval.go:57`, "cosine similarity") stays true in this change, because no BM25F score reaches
`Similarity`; when fusion resolves through the word index (change 4, owner ruling on question 2), the doc becomes
"the resolve mode's own score, on that mode's scale", and the node names the mode.

**D5. Readiness, restart and offloaded bodies.** On start the follower replays every entity, and the verb answers
`index_not_ready` until the floor reaches the replay's target, `graph_state_reset_required` after an undecodable
entity, and `index_not_ready` after a lost feed (§4). An offloaded entity's revision completes only when its body is
indexed or has reached one of the states below, so a first build that reads ready has read every body it could.

The body path is one goroutine under the run context, not a pool. It takes entities in delivery order, and each read
runs under `context.WithTimeout(runCtx, 10s)`, so `Stop` ends it (owner ruling, question 4). `Deliver` never waits
for it. Probe (`objectstore-watch-probe.log`, nats.go v1.55.0, server 2.15.0): a healthy read takes 0.42 to 0.46 ms
mean, 1.07 ms p99, at 2,000 and 6,000 bytes; with the server paused, a read returns at its deadline (2.002 s at a
2 s deadline, 2.967 s at 3 s); a plain cancel with no deadline was seen only after 4.95 s. So a `Stop` during a
read waits until that read's deadline, at most 10 s.

The bound under a blocking store (finding 2): when a read on a store reaches its deadline, the store is *stalled*
until the next repair pass, and the body path does not read from it again before then. Each queued entity of a
stalled store completes at once as unread. A first build under a store that blocks therefore takes the inline
replay plus one 10-second deadline per store, whatever the number of entities. The bound holds for a store that
honours its context, which the NATS object store does (the probe above). For one that does not, the body path waits
on that read, the key reads degraded after 30 s without a completion (`follower.go:90-95`), and `Stop` returns its
context's error and keeps the component for a later `Stop` (ADR-113); this is declared, not fixed.

Four states of a body not in the index (finding 3), each counted in `BodiesNotIndexed` and in a gauge by state:

| State | Cause | Retried | Key and `Health` |
| --- | --- | --- | --- |
| queued | read not yet made | it is the next read | floor waits, no failure, healthy |
| unread | read error, deadline, or stalled store | every repair pass | failure `body_unread`: key degraded, `Health` names it (`follower.go:382-384`, `:508-512`) |
| missing | `storage.ErrObjectNotFound` (`storage/objectstore/store.go:148-153`) | no, until the entity changes | no failure, healthy; first logged |
| no store | no store registered under the instance | each pass, a registry lookup only | no failure, healthy; first logged |

Only `unread` is a failure, because only a retry can fix it; none withholds the verb. A store registers before
graph-lexical starts, because the component manager starts store providers in a phase of their own before the rest
(`service/component_manager_lifecycle.go:37-54`, providers sorted first at `service/component_manager.go:293-301`,
registration after `Start` at `component_manager_lifecycle.go:123`). So `no store` at start means a store outside
this process or a configuration fault, not a start race. The repair pass runs 30 s after the previous one ends,
never overlapping; one pass costs the healthy reads plus at most one deadline per store.

The live watch while the body path works: `Deliver` does no I/O, so it is as fast as decode and `Put` (about 16,500
entities a second, #623). The probe also had a watch reader take 1 ms per entry while 20,000 writes landed on 5,000
stored keys: all 25,000 entries arrived in order, no revision skipped, the channel open. Behaviour past that
(nats.go's subscription limits) is not measured and not promised.

Rebuild cost at SemSource's size, arithmetic from the probes: 32k offloaded entities at 0.46 ms a read is about 15 s
of reads, plus about 2 s of `Put`. Task 2.6 measures the whole first build against the owner's budget (question 1).

**D6. The per-entity word record: an extension point, not built.** Where it goes: the body path, before a read.
What it needs: `lexical.Index.Words(id)` and `PutWords(id, name, body)` (change 1 D7), a tokenizer version constant,
and a bucket `LEXICAL_WORDS` keyed by entity ID holding `{source revision, tokenizer version, name words, body
words}`, written after `Put`. On a start, an entity whose record matches its revision and the tokenizer version is
restored with `PutWords` and its body is not read. Nothing in this change's surface blocks it: the verb, reply, key
and config do not change. When: task 2.6's measurement against the budget.

**D7. Memory (#623): measure, then change the layout.** The index holds about 60 bytes per word, about 9.5 GB at a
million entities of 157 words (§2c). (a) Before the processor loads real data, re-run #623's harness, change the
private layout to word numbers (one table from word to number; sequences and postings by number, the layout change
1's `sequence-probe` measured at 472 B per 20-word entity), and re-run it (chosen; task 2.2). (b) Accept 60 B: with
the 16,000 cap (D3), SemSource's code bodies average about 600 characters, so its corpus fits, but a million entities
does not. No caller, test or scorecard answer changes (change 1 D7). Gauges report held entities and words.

**D8. Smoke (#628, first item).** `stackConfig` gains `graph-lexical`; `componentFactories` and `indexKeys` gain it
(§4). A new check asks the verb for a word only one fixed entity's name holds, a word every base's name holds, and a
deleted entity's word; it waits on the reply's `indexed_revision` reaching the `ENTITY_STATES` revision, then
requires the live holders only. The restart tests compare the whole reply before and after restart, rank and score,
which BM25F over the held set makes exact (R1). The fixture has no offloaded body (§4), so the body path is proven
in the component's integration tests (task 2.4), not here.

**D9. Docs.** The processor README says word search helps entities with text only; sensor and drone telemetry and
other numeric entities gain nothing and are reached by prefix, predicate, relationships, name and alias, and group
(the list in `graph/lexical/README.md`, "Who word search helps"); it states the verb, the four body states, the cap
and its cut gauge, file paths, memory, and what a restart costs.

**D10. Three pull requests, one change (finding 8).** PR A: task 2.1, the text rule and graph-embedding onto it,
`Closes #551`. PR B: task 2.2, the index layout, `Closes #623`. PR C: tasks 2.3 to 2.7 and 3.x, the component, the
verb, smoke and docs, `Addresses #628`, with the archive. A and B touch nothing of the component and each has its own
proof, so they land first. Cost: the `entity-text` delta is archived with PR C, so `openspec/specs` lags PR A's code
by the time C takes; the tasks name the PR each lands in.

Adoption list (D3 establishes a primitive): `graph/clustering/terms.go:57` (change 4),
`processor/graph-query/localsearch.go:95` (change 3, replaced by the index), `processor/graph-embedding/hop1.go:228`
and `graph/embedding/worker.go:744` (this change). Filed as one tracking issue at landing.

## Risks / Trade-offs

- The first build waits for every reachable body; at SemSource's size about 15 s of reads by the probe's figure, on
  one machine class (Docker Desktop on Apple silicon). Other hosts are unmeasured until task 2.6.
- A transient store fault reads degraded and unhealthy until a repair pass reads the bodies; that is the honest state.
- Search holds the read lock for its whole scoring, so a common-word query on a large index delays writes (§2c).
- Each process holds its own index; two instances answer from two copies built from one bucket.
- graph-embedding is edited (D3); its tests and a byte-equality test guard its output. #606 batch 6 is not open.

## Migration Plan

None for data: nothing is stored. A product adds `graph-lexical` to its composition and registers its factory.
graph-embedding's config keys and output are unchanged.

## Open Questions

Ruled by the owner on PR #629's design read: bodies are read on every start (after findings 1 to 3, as above); no
silent switch of SemSource's natural-language path; `docs/setup-plan.md:429-430` is corrected (task 3.1); the
10-second read deadline stands, derived from the run context.

1. Owner: the first-build budget, set from the probe's number as the ruling asks. Recommended: 60 s for 32k
   offloaded entities on one machine, about four times the arithmetic above; if task 2.6 exceeds it, the word
   record (D6) is the next change.
2. Owner: graph-lexical's default cap of 16,000 characters (D3). Recommended as measured: it cuts 1 of 4,766
   SemSource symbol bodies, against 34 at 4,000; set lower only if task 2.2's memory figures say so.
