# SemSource truncation count

These numbers were measured at `0c3f00ea`, before each cut entity was counted once: `provider` then also counted
entities the cap had already cut, so the two series below overlap and do not add up. They were not measured again.

How many of SemSource's entities graph-embedding embeds from only the opening of their text, read from the new
`semengine_graph_embedding_truncated_total{by}` counter on one full index of the SemSource repository. Run on
2026-11-23 on an arm64 Mac (Apple silicon, Docker Desktop).

## Result

| What | Count |
| --- | ---: |
| Entities in the graph (`ENTITY_STATES`, `total_entities`) | 9,546 |
| Keys in `EMBEDDING_INDEX`, all `status: generated` | 9,546 |
| Texts sent to semembed (`semembed_requests_total`, one text per request) | 8,916 |
| Distinct texts in `EMBEDDING_DEDUP` | 8,911 |
| `truncated_total{by="provider"}`: semembed cut the text to fit 512 tokens | 890 |
| `truncated_total{by="text_cap"}`: graph-embedding cut the text at `max_text_len` | 137 |
| `semembed_inputs_truncated_total`, semembed's own count | 890 |

With the default model (arctic-embed-s, 512 tokens) about 9.3% of all entities (890 of 9,546), or 10.0% of the texts
semembed actually embedded (890 of 8,916), are embedded from only their opening. The 4,000-character cap cut 137
entities, 1.4%.

Settings in force: `max_text_len` was graph-embedding's default, 4,000 characters (SemSource does not set it); semembed
reported `max_tokens: 512` on `/health` and `/models`.

## Cross-check

semembed's own counter, `semembed_inputs_truncated_total`, read 890, the same as `by="provider"`. They count
different things, so equality is what this run happened to show, not a promise:

- graph-embedding counts a provider cut once per entity whose vector came back marked from a fresh request. An entity
  that took its vector from the dedup bucket is not counted again, because the dedup record keeps no mark. 630 of the
  9,546 entities (9,546 minus 8,916 requests) got their vector that way, so a deployment with many identical long
  texts would read lower on graph-embedding's counter than on semembed's.
- Entities that share one in-flight request each count their own cut, which can push graph-embedding's count above
  semembed's. This run shows no such surplus.
- semembed counts every text it cuts, query texts included; graph-embedding counts only entity texts. No queries were
  sent in this run.

The two labels overlap in this run. A text cut at 4,000 characters is almost always still longer than 512 tokens, so
most or all of the 137 `text_cap` entities are also among the 890 `provider` ones; the run does not show how many.
That overlap is why the counter now counts each entity once: a text the cap cut is counted under `text_cap` only, and
`provider` counts the texts only semembed cut, so the two series add up to the entities embedded from their opening.
Under that rule this run would read 137 `text_cap` and between 753 and 890 `provider`, and `provider` would no longer
be expected to match semembed's own counter.

8,916 requests against 8,911 distinct texts means five texts were embedded twice; why was not checked.

## Timings

- SemSource started 12:36:17 UTC; all three sources finished their initial seed at 12:36:22 (9,546 entities).
- Embedding was ready (`embedding.ready: true`, lag 0, every key `generated`) at the 12:42:00 poll: under six minutes
  for 8,916 texts on one semembed container with no CPU limit.
- semembed reported 298.5 s of inference across 4,154 sub-batches, 1,740,041 tokens. Its memory was 616 MiB at the end.

Progress, polled every 45 seconds, is in `semsource-truncation-poll.log`: each line is the status endpoint's phase and
embedding block, the `EMBEDDING_INDEX` key count by status, the two `truncated_total` series and semembed's counters.

## What was run

- SemEngine: branch `claude/truncation-count-667` at `0c3f00ea`.
- SemSource: `33b86a71f43f044ff5b908957e6621da987b3bdc`, copied with `git archive HEAD` to a scratch folder (SemSource's
  own checkout was not changed), its `github.com/c360studio/semengine` requirement replaced with the SemEngine branch.
- One edit in the scratch copy: the line `natsclient.WithHealthInterval(30 * time.Second),` deleted from
  `cmd/semsource/run.go`, because that option no longer exists in SemEngine and SemSource did not compile with it. It
  sets a health poll on the NATS connection and has nothing to do with embedding.
- What was indexed: SemSource's checkout at `/Users/coby/Code/c360/semsource`, that same commit with one untracked
  folder, `skills/`, which was indexed too.
- NATS: `nats:2.14.10-alpine@sha256:311da47e2f31c26e99c623cd78d486a1e8ab039c49b32652770bb91a4f70512e`, run with `-js`.
- semembed: `ghcr.io/c360studio/semembed:0.2.1@sha256:22ea57c60e33bcb8ae9bfd4f08888340824ed48dbc0c85b981b19a69557a292e`,
  default model. `/health` answered
  `{"status":"healthy","model":"Snowflake/snowflake-arctic-embed-s","max_tokens":512}`.
- SemSource ran natively, not in Docker. The config is SemSource's `configs/mvp.json` (ast, docs and config sources)
  with the tier 1 pieces of `configs/tiers/tier1-semantic.json` added, host paths in place of `/workspace`, and
  loopback ports:

```json
{
  "namespace": "c360",
  "sources": [
    { "type": "ast", "path": "/Users/coby/Code/c360/semsource", "languages": ["go", "typescript", "javascript", "java", "python", "svelte"], "watch": false },
    { "type": "docs", "paths": ["/Users/coby/Code/c360/semsource"], "watch": false },
    { "type": "config", "paths": ["/Users/coby/Code/c360/semsource"], "watch": false }
  ],
  "source_roots": ["/Users/coby/Code/c360/semsource"],
  "http_port": 63102,
  "websocket_bind": "127.0.0.1:63104",
  "metrics": { "port": 63103 },
  "graph": { "semantic_search": true, "index_workers": 4, "coalesce_ms": 200 },
  "model_registry": {
    "endpoints": {
      "semembed": { "provider": "openai", "url": "http://127.0.0.1:63101/v1", "model": "Snowflake/snowflake-arctic-embed-s" }
    },
    "capabilities": { "embedding": { "preferred": ["semembed"] } }
  }
}
```

Commands, with `$WT` the SemEngine worktree and the ports free loopback ports picked at the time:

```bash
mkdir /tmp/semsource-667
git -C ~/Code/c360/semsource archive HEAD | tar -x -C /tmp/semsource-667
cd /tmp/semsource-667
go mod edit -replace github.com/c360studio/semengine="$WT"
go mod tidy
sed -i '' '/natsclient.WithHealthInterval(30 \* time.Second),/d' cmd/semsource/run.go
go build -o /tmp/semsource-667-bin/semsource ./cmd/semsource

docker run -d --name semengine-nats-667 -p 127.0.0.1:63100:4222 \
  nats:2.14.10-alpine@sha256:311da47e2f31c26e99c623cd78d486a1e8ab039c49b32652770bb91a4f70512e -js
docker run -d --name semengine-semembed-667 -p 127.0.0.1:63101:8081 \
  ghcr.io/c360studio/semembed:0.2.1@sha256:22ea57c60e33bcb8ae9bfd4f08888340824ed48dbc0c85b981b19a69557a292e
curl -s http://127.0.0.1:63101/health

semsource validate --config semsource.json
semsource run --config semsource.json --nats-url nats://127.0.0.1:63100

# every 45 s until embedding.ready and no pending keys
curl -s http://127.0.0.1:63102/source-manifest/status
kvcount nats://127.0.0.1:63100 EMBEDDING_INDEX
curl -s http://127.0.0.1:63103/metrics | grep '^semengine_graph_embedding_truncated_total'
curl -s http://127.0.0.1:63101/metrics | grep -E '^semembed_'
```

`kvcount` (source in `kvcount.go.txt`, a stand-alone module on `nats.go` v1.55.0) lists a bucket's keys and tallies
each record's `status` field. The `EMBEDDING_INDEX` key count is the total embedded.

SemSource was stopped with an interrupt signal after the last scrape. It logged `semsource stopped` and no error
lines, but its exit status was 1; why was not checked.

## Not checked

- A long-window model. A run with `Snowflake/snowflake-arctic-embed-m-long` at `SEMEMBED_MAX_TOKENS=2048` was
  skipped. This model is larger, and semembed's own measurement puts it at about 1.4 s for each 2,048-token
  text, so the run was expected to take well over 15 minutes. It also could not have shown much: SemSource does
  not set `max_text_len`, so graph-embedding's 4,000-character cap would still cut first. Raising the cap needs a
  second edit to SemSource.
- Which entities were cut, and how long their texts are. Only the counts were read.
- How many of the 137 `text_cap` entities are also among the 890 `provider` ones.
- How many of the 630 entities that reused a stored vector had a text semembed would cut.
- Any other corpus than SemSource's own repository, and any SemSource config other than the one above.
- A second run, to see whether the counts repeat.
