# Fable brief: #740 warm-restart replay

One judgment, read-only. Owner opened this session after stopping implementation on 2026-11-24.

## The question

What is the smallest change that stops a warm restart over a large store from failing `Start`, on a NATS server with
default settings, without changing how components own NATS connections? Is the approved design's answer (one NATS
connection per whole-bucket reader, through a new exported `natsclient.Client.NewSibling`) justified, or is there a
narrower fix? Rule on which shape #747 should carry, and what of the work done so far stays.

## The problem, measured

- Issue #740. Consumer: C360Studio/semsource#351 (works around it with `max_pending: 536870912`).
- On boot every whole-bucket reader replays its bucket at once, all on the process's one connection. The server lets
  each push watch run about 32 MiB ahead (flow-control window) and closes a connection with more than 64 MiB
  (`max_pending`) waiting. Any component `Start` that runs during the drop fails. Library facts are probe results at
  nats-server 2.15.0 (the pinned image) and nats.go v1.55.0; source reads are marked as such in `design.md`.
- Probes (scripts and every output: PR #747 comments 7137819740 and 7137820382; summary in `design.md` § Context):
  - P1: five watches on one connection at 230 MB, 7–10 server closes. One watch alone: none.
  - P2: with five watches, failures begin between 17 and 33 MB.
  - P3: after a close, each watch resumes where it stopped; the issue's "replays start over" is false.
  - P5: one watch per connection, five or ten of them, no close. Five run one after another on one connection, no close.
  - P6: five **header-only** (`MetaOnly`) watches at 230 MB: no close, 2.3 s.
- Failing-first test `TestSmoke_WarmRestartOverALargeStore` (132 MB, default `max_pending`): red on Linux CI at
  2b4cf3af, <https://github.com/C360Studio/semengine/actions/runs/45443875456/job/138622164026>.
- Separate, filed as #750: graph-index's follower also overflows nats.go's own 64 MiB per-subscription buffer
  (client-side drops, then the consumer resets; no lost update). The cause is that graph-index's reader is slow. One
  connection per watch does not change this. A header-only watch would.

## Who replays a whole bucket at boot (inventory § 1.1)

| Reader | Uses the delivered value? |
| --- | --- |
| R1 graph-index follower | no, re-reads the entity (`processor/graph-index/index.go:211`) |
| R2 graph-lexical follower | yes |
| R3 graph-clustering follower | yes |
| R4 graph-embedding follower (tier 1) | no, reads current truth |
| R5 websocket view | yes, keeps a copy |
| R6 graph-ingest boot sweep, inside `Start` | yes |
| graph-query COMMUNITY_INDEX view; graph-embedding's three EMBEDDING_INDEX watches (tier 1) | yes / mixed |

## Options to weigh

1. **Approved design (D1 B):** one connection per reader via `NewSibling`. It touches natsclient, graph/follower,
   websocket, graph-query, graph-ingest and graph-embedding, and adds about 5 to 8 connections per process.
2. **Header-only watches for R1 and R4** (`jetstream.MetaOnly`). This needs a `follower.Config` field and changes
   `Entry.Bytes` (the entity-state-follower spec `:193`). P6 says header-only watches cost almost nothing. R2, R3, R5
   and R6 still replay full values, and P1/P2 say four full replays on one connection can still close it. Measure
   whether this alone passes the 132 MB test. It also covers #750.
3. **Replays one after another on the shared connection** (P5: no close). The design rejected this because the
   embedding Worker's snapshot waits on the provider (`graph/embedding/worker.go:347-371`), so tier 0 readiness would
   wait on tier 1. Is that hazard real, or can tier 1 readers be left out?
4. **One shared second connection for all boot replays.** This was not weighed. Two connections instead of one per
   reader. Does it still sum past 64 MiB with four or more full replays on it? (P1 says yes for five.)
5. **A combination**, e.g. option 2 plus option 1 only for the full-value readers.

## Work on the branch (claude/warm-restart-740, draft PR #747)

- Pushed (head 236f68c2): inventory, design, owner rulings
  (#740 comments 7136495793, 7137820958), the failing test (883196d3), `NewSibling` (e1e6da25).
- Local only, not pushed: 710dde98 (followers), eaad5c2a (views), 21f3a5f6 (boot sweep), 85f5e153 (review fixes,
  shared test helpers in `internal/harness/natsfixture/connections.go`). After 710dde98 alone, the 132 MB test passed
  locally (macOS, not Linux CI). Code diff 60fa24bd..85f5e153: 19 files, +1819/−62, mostly tests.
- Uncommitted: graph-embedding (task 4.4), half done, in `processor/graph-embedding/component.go` and a new test file.
- Notes per task: `notes/2.1.md`, `notes/3.1.md`, `notes/4.1.md` to `notes/4.3.md`.

## Governing text

ADR-108 D4/D7 (one follower per producer), ADR-112 (natsclient passes nats.go through, adds no mechanism), ADR-113
(Start bounds Start; the boot sweep is startup work, `:118`), ADR-081 (graphview, one watch per view). Owner rules:
smallest fix that mirrors what the code already accepts; natsclient adds no mechanism; an operator must not have to
predict a buffer the framework could observe.

## Wanted back

A ruling of a page or less: which option, what the minimum proving test is, what to keep or drop from the branch (the
commits above), and whether the approved proposal must be re-approved.
