# graph/lexical

Word search over the text of graph entities. An `Index` holds, for each entity, the words of a short name and a
longer body, and ranks the entities that match a query with BM25: a standard scoring formula that rewards a word
that is frequent in the entity and rare across all entities, and that discounts long texts. Words in the name count
twice (`NameWeight`). The index is in memory and needs no external provider. The package imports no NATS client and
reads no storage: the caller decides what text each entity has and writes it with `Put`.

The `graph-lexical` component (`processor/graph-lexical`) fills an index from `ENTITY_STATES` and answers
`graph.lexical.query.search` with it; its README covers configuration, memory and restart cost. A product that wants
something else builds an `Index` and keeps it current itself. Nothing persists: after a restart the caller writes the
entities again, as the component does by reading the bucket.

## Who word search helps

Word search helps only entities that have text: documents, notes, descriptions, names. An entity whose facts are
numbers, such as sensor or drone telemetry, gains nothing from it: there are no words to match, and a reading such
as `41.7` is one more word, not a quantity.

Such entities are reached through the structure of the graph instead, by the read requests graph-ingest,
graph-index and graph-query answer today:

- **By ID prefix**: every entity whose ID starts with given parts, for example every entity of one source
  (`prefix`).
- **By predicate**: every entity holding a predicate, such as `sensor.reading.temperature`, with counts per
  predicate and AND or OR over several predicates (`predicate`, `predicateList`, `predicateStats`,
  `predicateCompound`).
- **By relationship**: the entities one entity points to, and those pointing to it (`outgoing`, `incoming`,
  `relationships`), and a walk outward from one entity (`pathSearch`).
- **By name and by alias**: an entity's name or one of its aliases (`byName`, `alias`, `entityByAlias`).
- **By group**: `graph/clustering` groups entities by the first parts of their IDs; the grouping needs no text.

Two limits apply to numeric data today:

- The `predicate` request can filter on a value, but it matches the value written as a string, exactly: a stored
  `85` matches `"85"` and nothing else. Nothing offers a numeric range, such as "temperature above 80".
- The time-range request (`graph.temporal.query.range`) and the bounding-box request (`graph.spatial.query.bounds`)
  are declared, but nothing in this repository answers them, so graph-query's `temporal` and `spatial` requests,
  which forward to them, cannot be answered either.

## Basic use

```go
ix := lexical.NewIndex()
err := ix.Put("acme.ops.docs.runbook.page.restart", lexical.Document{
    Name: "Restart the cooling pump",
    Body: "Stop it with docker compose down, then call withRetry until the pump reports ready.",
})
// ...
q, err := lexical.ParseQuery(`pump -telemetry`)
// ...
res, err := ix.Search(q, lexical.Options{Limit: 10})
for _, hit := range res.Hits {
    fmt.Println(hit.EntityID, hit.Score)
}
```

`ExampleIndex` in `example_test.go` writes two entities and runs five queries; `go test` checks its output:

```text
pump retry: 2 matched acme.ops.docs.runbook.page.restart acme.ops.docs.runbook.page.readings
pump -telemetry: 1 matched acme.ops.docs.runbook.page.restart
"compose down": 1 matched acme.ops.docs.runbook.page.restart
withretry: 0 matched
pump*: reserved syntax
```

## What is here

| Name | What it does |
| --- | --- |
| `NewIndex`, `Index` | An empty index; safe for concurrent use, and a search sees each write whole or not at all |
| `Document` | The text an entity is found by: `Name` and `Body` |
| `Index.Put` | Replaces everything an entity held with the words of a document; a document with no words removes the entity. Refuses an ID that is not a canonical six-part entity ID (`ErrInvalidEntityID`) |
| `Index.Delete`, `Index.Len` | Removes an entity; the number of entities held |
| `ParseQuery`, `Query` | Reads a query (rules below); the zero `Query` matches nothing |
| `Query.Excluded` | The excluded terms, each as its words, so a caller can say which words removed entities |
| `Index.Search`, `Options` | The best `Limit` hits (at least 1, else `ErrInvalidLimit`) among entities under one of the `Scope` ID prefixes (all entities when `Scope` is empty; a malformed prefix is `ErrInvalidScope`) |
| `Result`, `Hit` | Hits ordered by score, then by entity ID; `Matched` counts every match in scope before the limit |
| `Tokens` | The one tokenizer, used for documents and queries alike |
| `NameWeight` | How many times a name word counts against a body word: 2 |

## How text becomes words

`Tokens` splits text at everything that is not a letter or a digit, so `graph.query.status` gives `graph`,
`query` and `status`, and `pump-7` gives `pump` and `7`. A run also splits where an identifier changes case:
`withRetry` splits into `with` and `retry`, `HTTPServer` into `http` and `server`. The joined form is not kept, so
`withretry` finds nothing; search for `retry` or `withRetry`. An acronym in the plural keeps its capitals together:
a run that ends in a lower-case `s` right after two or more capitals drops the `s`, so `IDs` gives `id`, `URLs` gives
`url` and `UUIDs` gives `uuid`. This applies only at the end of a run; `EntityIDsByType` gives `entity`, `i`, `ds`
and `type`, because inside a run the same letters are as often an acronym followed by a word, as in
`EntityIDIsTerminated`.

Words are lower-cased and plurals folded to the singular (`services` to `service`, `ids` to `id`, `prs` to `pr`).
The plain rule drops a final `s`, but not `us` or `ss`, from a word of 3 bytes or more, so `os` and `ms` keep theirs
and `bus` and `class` stay as written. Because the rule cannot tell a plural from a word that only ends in `s`, it
also joins some words that are not plurals:

- `its` meets `it`;
- a three-letter acronym ending in `s` meets its first two letters: `TLS` gives `tl`, `CAS` (compare-and-swap)
  meets `CA` (certificate authority), and `ios` meets Go's `io`;
- a duration meets its bare number (`10s` and `10`);
- a two-letter stop word with an `s` is dropped like the stop word: `ins` goes with `in`, and `DOS` gives no word,
  so a query of only `DOS` is refused with `ErrEmptyQuery`.

A short list of stop words (`the`, `and`, `with` and the like) is dropped, and so is any word longer than 32 bytes
after folding, which in practice is hex such as a commit hash.

## Query rules

- **Plain words** rank the results. An entity matches when it holds at least one of them (or a phrase, below);
  entities holding more of them, and rarer ones, rank higher.
- **A phrase** is text in double quotes. Its words must stand next to each other, in order, within the name or
  within the body. Quoting makes any text literal: only tokenizing applies inside quotes, so `"name:pump"` or
  `"-race"` is searched as words. Because stop words are dropped first, `"out of memory"` also matches "out the
  memory". A quote left open ends at the end of the query.
- **An exclusion** is a term outside quotes starting with a single `-` followed by a letter, a digit or a quote.
  It removes every entity holding it, and adds nothing to a score. So `go test -race` excludes `race`, while
  `--force` is the plain word `force`. To search for a flag such as `-race`, quote it.
- **Reserved syntax** is refused with `ErrReservedSyntax`, so a later query form can give it a meaning without
  changing an answer given today: a term outside quotes that starts with `name:` or `body:` followed by more text,
  or that ends in a letter or digit followed by `*` (`pump*`). The error names the term and says to quote it, which
  searches it as text. A term that also starts with `*` is emphasis, so `*bold*` is the word `bold`. Every other mark
  separates words: `HEAD~` is the word `head`.
- A query with no plain word and no phrase left after tokenizing (only stop words, punctuation or exclusions) is
  refused with `ErrEmptyQuery`. A query with more than `graph.MaxSearchTerms` (32) distinct words, plain, phrase and
  excluded counted together, is refused with `ErrQueryTooLong`.

## How good the ranking is

`TestScorecardHoldsItsFloor` asks 500 known-item questions (each has one right section as its answer) of 2,000
sections of this repository's own Markdown, frozen under `testdata/scorecard/`. The index puts the right answer in
the first ten hits for 56.0% of them, and the test fails below 55.0%. A plain BM25 ranker over the body alone,
using the tier-0 embedder's tokenizer, scored 52.6% on the same questions. That ranker is not part of the
package or its tests; its source and how to rerun it are in
[`2.4-reference.go.txt`][refsrc] and [`2.4.md`][refrun]. The figures say how the index does on this repository's
prose, not on any other text.

[refsrc]: ../../openspec/changes/archive/2026-11-12-setup-04b-01-lexical-index/notes/2.4-reference.go.txt
[refrun]: ../../openspec/changes/archive/2026-11-12-setup-04b-01-lexical-index/notes/2.4.md
