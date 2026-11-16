# Proposal: fix-674-lexical-body-revisions

Approved-by:

Track: `quick`. Capability modified: `graph-lexical` (two existing requirements; no requirement added).

## Why

An entity whose body is kept in an object store and whose other values change often (telemetry) drops out of
searches for its body's words (issue #674). graph-ingest keeps the entity's `StorageRef` when other triples merge in
(`processor/graph-ingest/entity_writes.go:244-247`), but graph-lexical treats every revision as new text: it puts the
inline text alone (`processor/graph-lexical/apply.go:150-151`), which replaces the whole document
(`graph/lexical/index.go:138`), queues a body read (`apply.go:160`), and drops any read whose revision a newer one
overtook (`apply.go:170`, `:185`). While updates arrive faster than a read returns, the body is never indexed, and
each update costs a store read. `bodies_not_indexed` counts it as `queued`.

## What Changes

- **Unchanged text, unchanged reference: nothing to re-index.** A revision whose `StorageRef` equals the one held
  for the key and whose inline text equals the held inline text leaves the index as it is, asks for no body read,
  and completes as soon as no read for that reference is out. To compare, the applier keeps each offloaded entity's
  inline document and reference after the read (today `pending` holds them only while a read waits, `apply.go:96-99`,
  and drops them at `:191`). Memory: the inline text of each entity with an offloaded body; the body is not kept.
- **A read serves its reference, not its revision.** A body read is indexed while the key's latest revision still
  names the same reference and its body is not yet indexed, joined to that latest revision's inline text. A newer
  revision that keeps a reference whose read is queued or out asks for no read of its own.

| Case | What a search sees | Store reads |
| --- | --- | --- |
| Non-text values change, same reference | Inline and body words throughout | None |
| Inline text changes, same reference | New inline words at once; body words return when the read does | One |
| Reference changes | New inline words; old body words gone at once; new body words when read | One |
| Reference removed | Inline words only; a read still out is dropped | None |
| A read fails | As today, by cause; a same-text revision changes nothing; repair passes retry `unread`, `no store` | Per pass |
| Restart | Every offloaded body read once at start (owner ruling on #10) | One per body |

A body whose store content changes behind an unchanged reference is read again only when the inline text or the
reference changes, or at start; today any revision re-reads it. SemEngine's object store writes a new key on every
store (`storage/objectstore/content.go:115-118`), so a new body comes with a new reference. A `missing` body is
likewise read again only then, not on every revision.

## Options not chosen

- **Keep the body text and reuse its words** (the issue's second option, in full): covers inline edits with no read,
  but holds up to `max_text_len` characters (16,000 by default) per offloaded entity beside the index's own words.
- **Keep the old document until the new read returns** (the issue's first option): the index shows stale inline
  text while a read is out, against the requirement that it holds the latest inline text, and still starves while
  each read is dropped by the next revision.

## Tests

Failing first, recorded in `notes/2.1.md`: `TestPropApplyMatchesModel` gains a "keeps its reference" choice on
deliveries (body words per reference, not per revision) and witnesses: a non-text revision after the read keeps
the body's word and asks for no read; a run of same-reference revisions while the read is out ends with the body
indexed after one read; a reference change drops the old body's word. `TestStaleBodyReadDropped` gives R+1 a new
reference, so its case stays a read the key no longer names.

## Impact

No exported surface changes. PR #675 carries it (`Closes #674`); no other open PR touches `processor/graph-lexical`.
