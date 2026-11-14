# Design: fix-578-prefix-damaged-entities

The premises are pinned in `inventory.md` (base `88c7deec`). This design starts from the recommendation on PR #655
(comment 6877450943). I re-checked every premise.

## Premises

1. Fusion uses only the page's IDs. Seeds come from `resp.Entities[i].ID` (`client.go:140`), and `fuseSeeds` reads
   them again through batch (`engine_lens.go:135`). Holds.
2. graph-ingest has the keys, total and page before any decode (`query.go:237-323`). The first decode is at `:330`.
   Holds, with one correction: an exact six-part prefix reads its stored value at `:286` to see whether the key
   exists. That read decodes nothing and records nothing.
3. A damaged seed reaches fusion's answer as unhydrated `error` (`client.go:265-270`, `:292-334`). Holds.
4. Not in the recommendation: fusion sends up to 40 seeds (`engine_lens.go:31`), and the batch reply has no byte fit
   (`query.go:150`). See "What changes for a caller".

## Options

- **A. A verb, `prefixIDs` (chosen).** It costs two table rows, one forward entry, one reply type, and a row added to
  five tests that list the table (inventory § 3). Each of those tests fails loudly until its row is added.
- **B. A flag on `PrefixQueryRequest`.** It is smaller: one field and one branch, no rows. But the spec says "A caller
  SHALL decode by verb; nothing SHALL decide a reply's shape from its content" (graph-transport-boundary `:433-437`),
  and a row declares one `Reply`. A flag gives one verb two reply shapes, and the table cannot say which.
  `TestGraphQueryRepliesDecodeByTheTable` refuses the extra fields. Measured, and rejected. (A secondary point:
  graph-ingest decodes without refusing unknown fields, `query.go:237`, so a responder without the flag would
  ignore it and answer with full entities.)
- **C. Do nothing.** Fusion's prefix search keeps failing on one damaged record, and #578 stays open.

## D1. graph-ingest's `prefixIDs`

- The prefix and `prefixIDs` handlers share one function for the part they have in common: decoding and checking
  the request, listing, sorting, dropping repeats, the total, the cursor and the limit. The prefix verb's
  behaviour and tests do not change.
- Reply: ``graph.PrefixIDsResponse{IDs []string `json:"ids"`; Total int `json:"total"`; NextCursor string
  `json:"next_cursor,omitempty"`}``. `ids` is never `null`: an empty page is `[]`.
- The page is the longest leading run of the selected keys whose whole encoded reply fits the server's maximum
  payload. `next_cursor` is the page's last ID when more keys follow it. When the first key cannot fit, the reply is
  refused with class invalid, code `response_too_large`, and the detail `response_bytes` and `max_payload`, as on
  the prefix verb.
- It reads no stored value except the existence check at `:286`. So it makes no damaged-entity record, does not
  change health, and touches no cache.
- A cursor is promised only on the verb that gave it.

## D2. Forward and table

- Rows: `prefixIDs` on `graph.ingest.query.prefixIDs` (graph-ingest) and on `graph.query.prefixIDs` (graph-query).
  Both are bare, with `Reply` `PrefixIDsResponse`. graph-query's forward map gains
  `"prefixIDs": {graph.ResponderGraphIngest, "prefixIDs"}`.
- **graph-query's counts and summary stay on `prefix`.** They read only IDs (graph-query spec `:333`), so they look
  like adopters. But ruling 1 has them refuse on damage, and moving them to `prefixIDs` would stop that.

## D3. Fusion

- `resolvePrefix` requests `prefixIDs` with `Prefix` and the resolve limit. It refuses a listed ID that
  `graph.ValidateDecodedEntityIDs` refuses, as `resolveSemantic` does, and returns one seed per ID in listed order.
- The `verbs` lookup replaces `prefix` with `prefixIDs`: nothing else in the client reads the `prefix` row.
- `Entities` does not change. Its re-read of a refused batch goes with #656.

## What changes for a caller

- Fusion's prefix search answers when an entity under the prefix is damaged, and lists it as unhydrated `error`. A
  key deleted between the listing and the read is listed `not_found` instead of silently dropping out.
- The prefix mode now sends up to 40 seeds, as `nl` and symbol do. Forty entities that together exceed the server's
  maximum payload are refused `response_too_large`. Before, the prefix page's byte fit held fewer seeds. Filed as #659,
  which stays open; this change does not fix it.

## Proving tests (failing first)

- **graph-ingest, damaged entity listed.** Use `poisonScopingTestComponent` with A good, B damaged and C good.
  `prefixIDs` gives `{"ids":[A,B,C],"total":3}`, and `poisonInventoryEntry(B)` is false afterwards. The prefix
  verb on the same data still refuses fatal `graph_state_reset_required`. An exact six-part prefix naming B gives
  `[B]`, and still no record.
- **graph-ingest, first key too large.** A maximum payload too small for the first key gives `response_too_large`.
- **graph-ingest, refusal codes.** Add `prefixIDs` cases to `TestQueryRefusalsCarryACode`: a bad request, a bad
  cursor, a bad pattern, a failed listing, and too large.
- **Generated check (rapid; record the seed and completed runs).** Draw:
  - keys, some under the prefix and some not, some with damaged values, and repeats where the test bucket can list
    a key twice;
  - a limit from −1 to above 1000;
  - a maximum payload from one-key size upwards.

  The reference model restates the spec's rules itself, never calling the handler's code: the limit clamp (zero or
  less, or over 1000, means 1000; `query.go:255-259`), the `prefix.` dot rule, and the exact-key fallback that
  applies only when nothing is listed under the prefix (`:276-291`).

  Walk `prefixIDs` from no cursor until `next_cursor` is empty. Against the sorted, distinct reference keys, every
  run must hold:
  - The pages joined together equal the reference, with each key once.
  - Every `total` is the reference's length.
  - Every encoded reply fits the maximum payload.
  - Each page is maximal: it holds min(limit, remaining) keys, or `json.Marshal` of the same `PrefixIDsResponse`
    with the next key added (and its cursor) exceeds the maximum payload. This oracle is the real encoder, so a size
    summed by hand that drifts from field order or `omitempty` fails.
  - No damaged-entity record exists.

  The named cases above run as their own tests, before the generated check.
- **Table.** `TestQueryVerbsDeclareEachRespondersVerbs` with the two rows. The graph-ingest and graph-query tests
  that list rows gain them (inventory § 3). `TestGraphQueryRepliesDecodeByTheTable` gains a `prefixIDs` request.
- **fusionnats unit.** A recording requester shows `Resolve` (prefix) sending to `graph.query.prefixIDs`, with seeds
  in listed order, and an invalid listed ID refused. The fakes in `client_test.go` and `poison_test.go` answer
  `prefixIDs`.
- **Fusion over real responders** (`TestDamagedEntityUnderPrefixOverRealResponders`, modelled on
  `TestPoisonedSeedOverRealResponders`). Store alpha and beta, and put damaged bytes under `poison`. As a
  precondition, `graph.query.prefix` on `c360.test.fusion.alpha` refuses `graph_state_reset_required`. Then
  `fusion.Engine.Fuse` with a prefix lens answers alpha and beta, with `unhydrated` `[{poison error}]`. On `main` the
  same test fails with `graph_state_reset_required`, which shows it can fail.

## Merge order

PR #655 and #654 overlap only in `processor/graph-query` code and tests, not in spec requirements. Whichever is
approved first merges first, and the other merges `origin/main` and re-runs graph-query's tests. I recommend #655
first because it is smaller.
