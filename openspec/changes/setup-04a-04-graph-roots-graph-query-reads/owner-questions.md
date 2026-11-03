# Owner questions: setup-04a-04-graph-roots-graph-query-reads

## Should the graph overview and the hierarchy counts be exact?

**Recommendation: yes, add one small request to graph-ingest that counts IDs from a single key listing. By my estimate
it adds about 250 lines to this PR. If you say no, the design as written stands and is honest, but it answers with a
sample on almost every real graph.**

**What you accepted.** Read one page of entities and say when it is partial. That page is the alphabetically first
1,000 entities, fewer when they are large (about 450 at 2 KB). So on any graph past that size, the overview's type
list and the hierarchy's children come from the first few systems only. The answer says so, but it cannot tell the
caller what else is there. Getting the true numbers means walking every page, and each page repeats the whole listing:
36 to 53 seconds for 100,000 entities in one group, in the earlier measurements.

**What was not on the table then.** Both reads need only entity IDs, never their contents. graph-ingest already lists
every key under the prefix on each request, before it picks a page (`processor/graph-ingest/query.go:280-298`). A
request that counts those keys by next segment and by type answers exactly, for the price of the listing alone: about
155 ms for a 100,000-entity group, 1.4 s for a million-entity bucket. Those figures are for one page including the
listing, so the count should cost no more; I did not measure it separately.

**What yes changes.** `summary` and `hierarchyStats` read the new verb, and `entities_scanned`, `truncated` and
`next_cursor` go, replaced by a true `total_entities`. A child list too long for one reply still says so. The two
counting tasks change and one is added for graph-ingest, a merged package, with its spec. The later plan to answer
counts from community records would no longer be needed.

**What no changes.** Nothing. This can also land later as its own PR, and consumers would change shape once more.
