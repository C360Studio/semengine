# Proposal: design-rule-core-split-plan

Approved-by:

Track: `design` (the owner's choice for this lane). No spec Requirement changes here; each porting PR is its own
OpenSpec change with its own design.

## Why

Milestone `v0.1.0-alpha.9` ports the rule engine: `processor/rule` and `processor/rule/expression`, without the
agentic `publish_agent` action and the governance `deny`/`approve` actions (owner, #8, Q15; SETUP 03B design D12).
At SemStreams `8b99efe9` the two packages are 163 Go files and 51,467 lines, two thirds of them tests. A porting
PR carries one package or one lifecycle owner, about 3,000 pin lines as a guide (#500). This change is the plan for
how the port is cut into PRs, in what order, which issue each retires, and which designs Fable reads first.

## What changes

- `design.md` splits the port into 12 PRs, one OpenSpec change each, listed with what each carries, its size, the
  issues it closes or leaves, what it waits for, and whether its design goes to Fable before code.
- The rule engine is runnable by a product from PR 8 (the processor), answers semboids' message rules from PR 9,
  and is whole at PR 12. Before PR 8 the code builds and is tested but nothing starts it.
- Two follow-ons in the same milestone: the loop-supervision skill (#714, PR 13) and the deadline timer (#703,
  PR 14, its own change after the port, as the issue says).
- No code, no spec, no ledger row in this change. Ledger rows and ADR ports come with the PR that carries the code.

## Decisions you are making by approving

- **The order and the cut.** Twelve PRs, built from the bottom up: expressions, definitions, state and substitution,
  actions, rules, then the processor and its three inputs, cron and hot reload. Changes for you: semboids can run
  on SemEngine's rule engine after PR 9; semteams after PR 12, and only once it registers its own `publish_agent`.
- **Fable reads nine designs before their code** (PRs 2, 4, 5, 7, 8, 9, 10, 11, 12), because each touches the boot
  path, connection ownership, durability or acknowledgement, cross-plane writes, or new exported surface (#740).
  PR 2 carries #25's registry. PRs 1, 3, 6 and 13 do not go to Fable.
- **What is not carried.** `publish_agent`, `deny`, `approve`, the verdict auditor and their tests (already ruled);
  also the pin's hand-written "owner proof" tests (1,184 lines), which SemEngine's lifecycle suite replaces.
- **Four config keys are dropped and then refused at boot:** `message_cache`, `buffer_window_size`,
  `alert_cooldown_period` and `consumer`. At the pin they are read and then ignored. None of semboids', semteams',
  semsource's or semconnect's checked-in rule configs sets them (measured).
- **Some PRs run over the 3,000-line guide.** PRs 3, 7, 8, 10 and 11 are 4,100 to 5,400 pin lines, mostly tests that
  cannot be split from the one owner they test. Each design states its count; a developer may split further.
- **The deadline-timer probe (#703) may start at any time.** It reads only nats-server and nats.go, not rule code.
  The timer itself merges after PR 12.

## Impact

Docs only. Twelve later PRs, each with its own approval. Consumers: semteams' five `replace_owned` files are
refused at load until migrated (owner, #8, task 4.9).
