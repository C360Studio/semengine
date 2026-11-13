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
  `ids` meets `id` and `prs` meets `pr`. All-caps forms then fold like lower-case ones: `IDS` gives `id`, `PRS` `pr`;
  `URLS` and `APIS` already give `url` and `api` today.
- A new test, failing on today's code, shows `ID`, `id`, `IDs` and `ids` find one another, and the same for URL,
  API, PR, ADR and UUID. `tokens.go`'s comments, `doc.go` if needed, the README's "How text becomes words" and the
  two spec requirements state the rules.

## Cost of rule F, measured

56 distinct 3-byte words newly fold, 2,663 times in the corpus (`acronym-plural-probe.log`, rule F). `its` is
2,199 of them: it now meets `it`. 35 are durations (`10s`, `60s`), 157 times, which now meet the bare number, as
`100s` already does today. The rest are acronyms and short words: `ops` meets `op`, `kvs` `kv`; `yes`, `tls`,
`sys`, `aws`, `his` fold to stems no other corpus word has. `ins` and `tos` fold to stopwords and are dropped (3
times). The fold is the same on both sides, so `tls` still finds `TLS`; the cost is only words that now meet.
Scorecard, today against S + F: R@1 24.6% / 24.6%, R@10 56.0% / 56.0%, MRR 0.356 / 0.358; every behaviour
question passes (`notes/probes/scorecard-probe.log`).

## Non-goals

A digit before a capital (`base64Encode`), issue #651. Mixed-case names (`IPv4`, `iOS`, `gRPC`). Plurals inside a
run (`EntityIDsByType`). A list of known acronyms or any dictionary.

## Impact

No exported symbol, signature or config changes. Every stored word list is rebuilt from text, so nothing migrates.
Overlap: PR #648 edits the purpose line of the same spec and the README's first section, not the lines this change
edits; #648 merges first and this branch merges `main` after.

## Rulings

Open for the owner:

1. **Lower-case 3-byte plurals.** (a) Adopt rule F, the full Done-when: `ids` and `prs` meet `id` and `pr`, at the
   cost above, mostly `its` meeting `it`. (b) Narrow the Done-when to rule S only: `IDs`, `ID` and `id` meet, but
   `ids` and `prs` meet only themselves, so a typed query `ids` misses the 44 standalone `IDs` in the corpus and
   finds only its 11 `ids`. Recommended: (a). The merges it adds are plural-like and the scorecard does not drop.
2. `Approved-by:` stamp.
