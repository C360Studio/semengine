# Proposal: design-rule-core-split-plan

Approved-by: cglusky (2026-11-25) <https://github.com/C360Studio/semengine/issues/9#issuecomment-7160955176> sha256:224c6b834b23

Track: `design` (the owner's choice for this lane). No spec Requirement changes; each porting PR is its own change.

## Why

Milestone `v0.1.0-alpha.9` ports the rule engine, `processor/rule` and `processor/rule/expression`, without the
agentic `publish_agent` action and the governance `deny`/`approve` actions (owner, #8, Q15; SETUP 03B design D12).
At SemStreams `8b99efe9` they are 163 Go files and 51,467 lines, two thirds tests. A porting PR carries one package
or one lifecycle owner, about 3,000 pin lines as a guide (#500). This change plans the cut, the order, which issue
each PR retires, and which designs Fable reads first.

## What changes

- `design.md` holds the decisions and the PR list; `split.md` holds each PR's contents, size, issues and required
  corrections. Twelve port PRs, then the deadline timer (#703, PR 14) and the loop-supervision skill (#714, PR 13).
- A product can start the rule engine from PR 8; it answers semboids' message rules from PR 9; it is whole at PR 12.
- No code, spec or ledger row here. Ledger rows and ADR ports come with the PR that carries the code.

## Decisions you are making by approving

- **The order and the cut:** twelve PRs from the bottom up. semboids can run on SemEngine's rule engine after PR 9;
  semteams after PR 12, once it registers its own `publish_agent`.
- **Rule firings stop sending graph events** (owner, 2026-11-25). The old engine sends a "rule triggered" update to
  `graph.events.*`, where nothing listens, here or in our four products. Dropping it removes about 1,000 lines and
  the trigger-identity rules of ADR-076 (its pack-ID rule is already kept). Two counters change: `total_triggered`
  counts firings that ran their actions, and `events_published` stops counting graph events. The gate below does
  not need the event: at the pin the event is what the gate throttles, not how it counts.
- **`fire_every_n_events` works as documented** (owner, 2026-11-25). It says it fires a rule's actions every Nth
  match, but on message and entity rules it never gated actions, only the graph event and a counter. PRs 9 and 10
  put the gate in front of the actions on both inputs; PR 7's design says whether the count is per rule or per rule
  and entity. On cron rules it already works and stays (PR 11).
- **Three accepted proofs change.** #25 closes at PR 7, when semboids' pack can actually fire. #26's check becomes
  "the `tool_choice` and `response_format` bytes survive unchanged", since SemEngine has no agentic types. #27's
  "semteams' 40 `publish_agent` actions dispatch" becomes semteams' own check at its switch; here, an import guard and
  a test family.
- **Fable reads nine designs before their code** (PRs 2, 4, 5, 7, 8, 9, 10, 11, 12): boot path, connection
  ownership, durability or acknowledgement, cross-plane writes, or new exported surface (#740).
- **What is not carried:** `publish_agent`, `deny`, `approve`, the verdict auditor and their tests (already ruled),
  and the pin's "owner proof" test files (1,156 lines). The two shutdown orderings they check that the lifecycle suite
  does not are re-proved through real `Start` and `Stop` in PRs 8, 9 and 11.
- **Four config keys are dropped and refused at boot:** `message_cache`, `buffer_window_size`,
  `alert_cooldown_period`, `consumer`. They are read and then ignored; no consumer config sets them (measured).
- **Nine of the twelve port PRs run over the 3,000-line guide** (3,038 to 5,363 lines), mostly tests of one owner.
- **The deadline-timer probe (#703) may start at any time;** the timer merges after PR 12, and the skill after it.

## Impact

Docs only. Twelve later PRs, each with its own approval. New modules later: `robfig/cron/v3` (PR 11), and
`gojsonschema` for one test (PR 2) unless that test is rewritten. semteams' five `replace_owned` files are refused at
load until migrated (owner, #8, task 4.9).
