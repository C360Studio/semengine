# natsclient-contract

Status: revision 1, draft for pre-owner review. Issue #170; claim PR #177. Rests on `inventory.md` revision 2 and
`design.md` revision 1.

## Why

`transport-client` says `Close` returns within the caller's deadline. It does not: nats.go v1.54.0 takes one lock for
every write on a connection, and a write the server is not reading keeps that lock until the write completes or
nats.go's one-minute write timeout ends it. A `Close` with a 50 ms context was held for 3 m 1.6 s, and past 200 s in
another run (P14). Nobody measured nats.go when the promise was written. The owner ruled on 2026-10-10 (epic #9
comment 6098388541) for a record of what natsclient promises over nats.go, each line measured, and for a rule that a
library claim cites a probe (#170). On 2026-10-11 he added that the record only takes promises away.

## What Changes

- **ADR-112.** natsclient passes nats.go's behaviour through. The record states what natsclient adds on top, in time,
  nil returns, error classes, ownership and reconnect. Each line names its probe or test, and anything not measured
  is listed as not promised.
- **`transport-client` cut to match.** Four requirements change:
  - Close's context ends its wait for the drain, not a write the server is not reading.
  - Each Close's wait ends with its own context.
  - A consumer's setup context is used for setup, rather than bounding it.
  - Subscribe's limit sentence names ADR-112 instead of #168: nats.go's write timeout ends each held write, and
    nothing bounds the wait as a whole.

  The Purpose line is edited by hand at the archive.
- **The rule.** One `AGENTS.md` row, and one bullet each in the architect and reviewer contracts.
- No code, exported name, option or test is added, and natsclient's behaviour does not change.

## Capabilities

### Modified Capabilities

- `transport-client`: four modified requirements; none added or removed.

## Impact

- Documents only: no `.go` or YAML file, so Codex's review of record is not needed (`AGENTS.md`, "Merge").
- #168 narrows to correcting natsclient's doc comments and README (16 places, `design.md` D1), in a later
  comment-only code pull request.
- A caller gets no bound on a call while a server stops reading. Change 3's shutdown design states this limit.
- Overlaps: #93 shares no file or heading, so either can merge first. #176 edits the same `docs/repository-map.md`
  row (`design.md`, "Overlaps and order").
