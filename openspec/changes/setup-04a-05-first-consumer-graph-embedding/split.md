# Split: change 5, `setup-04a-05-first-consumer`

Four pull requests, each its own OpenSpec change under the Slice 04A milestone. Pin lines from `inventory.md` § 1;
"port" figures are hand estimates of what each PR holds, production plus tests, not measurements. Every PR passes
`task verify`; a verb or package not yet landed reports no responders or does not exist, never a wrong answer.

| PR | Package (lifecycle owner) | Delivers | Pin lines (prod / test) | Port estimate | Depends on | Retires |
| --- | --- | --- | --- | --- | --- | --- |
| A | `processor/graph-embedding`, step 1 (`Component`) | Embeddings made at tier 0: shell, follower plug-in, Worker under the run context, failed map, repair lane, StorageRef text lane, config, Health, readiness on the `graph-embedding` key. No query verbs | 3,253 / 5,701 for the package | 1,500 + 2,300 | #548 | none |
| B | `processor/graph-embedding`, step 2 | `search` and `similar` verbs in the tree's wire shape; vector cache onto `pkg/graphview` (#477 ruling) in `graph/embedding`; ADR-071 scope; `searchGraph` decision; package whole | (same package) | 500 + 900 | A | none (advances #477) |
| C | `pkg/fusion` lens path, with `fusionvocab` | Engine, facets, hydrate, budget, rank signals; envelope copy gone (#110 item 2); no `vocabulary/cco` | 2,245 / 3,306; vocab 51 / 52 | 2,000 + 2,800 | none (parallel with A, B) | none (advances #110 item 2) |
| D | `pkg/fusion/fusionnats` (`Client`) | Retrieval client over `graph.QueryVerbs()`; readiness from the reply; the no-embedder composition proof | 658 / 1,403 | 500 + 1,100 | B, C | none |

Not carried: `fusion.Fuse`/`SubQuery`/`GraphQueryClient`/`Evidence` (03B D5, defer), `lensregistry` (dead: no pin
caller outside tests), the HTTP embedder path (#489), `graph.embedding.query.status` (ADR-083).

Why this cut: graph-embedding has one lifecycle owner but 8,954 pin lines; its write path (A) and its read path (B)
each stand alone and test alone, as graph-query's steps did. `fusionvocab` (103 lines, no owner, one interface
implementation) rides with C under #159's "split as suits the task": it only implements `fusion.RankSignals`, and
C's tests need a real signal. It carries its own open question (`cco.Parents` has no tree equivalent, inventory § 2);
if the owner's answer needs new `vocabulary` surface, it splits out as its own PR after C. fusionnats is its own owner
and is where the no-embedder proof lands, because it is the first caller that crosses graph-query to the embedding
verbs.

Order: #548 merges first (it changes `Watermark.Observe` and the follower A plugs into). A, then B. C may run beside
A and B in a second lane. D last. Each later PR merges `origin/main` before review.

Outside this split, for the owner: `output/websocket` and `pkg/buffer` (04A D2 change 5) are not in the 2026-11-08
direction.
