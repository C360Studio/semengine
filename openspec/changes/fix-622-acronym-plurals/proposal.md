# Proposal: fix-622-acronym-plurals

Approved-by:

Track: `quick`. Capability modified: `lexical-search` (two existing requirements; no requirement added).

## Why

`lexical.Tokens` splits an acronym written in the plural inside the acronym: `IDs` gives `i ds`, `URLs` gives
`ur ls`, `APIs` gives `ap` (issue #622). A search for `id`, `url` or `api` never finds that text. The scorecard
corpus holds 23 such forms, 150 times (`notes/probes/acronym-plural-probe.log`, rule S). Fixing the split is not
enough on its own: lower-case `ids` and `prs` are 3 bytes, and the plural rule only drops an `s` from words over 3
bytes, so `ids` would still not meet `id`.

## What Changes

- **Splitter (rule S).** A run that ends in a lower-case `s` right after two or more capitals drops that `s`, and
  the capitals stay one part: `IDs` gives `id`, `URLs` `url`, `UUIDs` `uuid`, `CPUs` `cpu`. Only at the end of a run:
  inside one the same shape is just as often a capital run followed by the word `Is` (`EntityIDIsTerminated`, 3
  times in the corpus, against 3 for `EntityIDsByType`), so `EntityIDsByType` keeps today's words.
- **Plural rule 5 (rule F).** Drop a final `s` (not `us` or `ss`) from words over 2 bytes instead of over 3, so
  `ids` meets `id` and `prs` meets `pr`. The limit is in bytes: `µs` folds to `µ`; `ms` and `ns` keep their `s`.
  All-caps forms fold too: `IDS` gives `id`, `PRS` `pr`; `URLS` and `APIS` give `url` and `api` already.
- A test failing today shows `ID`, `id`, `IDs`, `ids` meet, and likewise URL, API, PR, ADR, UUID. `tokens.go`,
  `doc.go`'s sentence on when words agree, the README and the two spec requirements state the rules.

## Cost of rule F, measured

`notes/probes/acronym-plural-probe.log`: 58 written forms newly fold, 2,663 times. `its`/`Its` is 2,199: it meets
`it`. Durations (`10s`) meet the bare number, as `100s` does today. Not all merges are plurals:

- Every 3-letter acronym ending in `S` folds to two letters: `TLS`/`tls` to `tl` (68), `CAS`/`cas` to `ca` (16),
  `SYS`, `GPS`, and `DNS`, `AES` (none here). Unrelated terms then meet: `CAS` (compare-and-swap) and `ca`
  (certificate authority), `gcs` (ground control stations) and `gc` (`runtime.GC`), `ios` and Go's `io` (16 `io`).
- Any 2-byte stopword plus `s` is dropped: `add-ons plug-ins` gives `add plug`; `DOS` and `dos` give no word, so a
  query `DOS` is refused with `ErrEmptyQuery` where today it finds matches.

Scorecard, today against S + F: R@1 24.6% both, R@10 56.0% both, MRR 0.356 / 0.358; every behaviour question
passes (`notes/probes/scorecard-probe.log`).

## Non-goals

A digit before a capital (`base64Encode`), issue #651. Mixed-case names (`IPv4`, `iOS`, `gRPC`). Plurals inside a
run (`EntityIDsByType`). A list of known acronyms or any dictionary.

## Impact

No exported surface changes; word lists are rebuilt from text, so nothing migrates. PR #648 edits other lines of
the same spec and README; it merges first, then this branch merges `main`.

## Rulings

Open for the owner:

1. **Lower-case 3-byte plurals.** (a) Rule F, the full Done-when, at the cost above. (b) Rule S only: `ids` and
   `prs` meet only themselves, so a query `ids` misses the 44 standalone `IDs` and finds 11 `ids`. (c) Rule F
   skipped for an all-caps part: same scorecard (MRR 0.358), keeps `TLS`, `CAS`, `DOS`, but `TLS` text then gives
   `tls` and a `tls` query `tl`, so they stop meeting (today they meet), and the fuzz test breaks the round-trip
   law at once (`Tokens("חS")`); rejected. Recommended: (a), naming its merges in the README.
2. `Approved-by:` stamp.
