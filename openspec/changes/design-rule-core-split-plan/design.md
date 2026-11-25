# Design: design-rule-core-split-plan

Base: `4ead6360978d008649e0eb5e869086dcac2b36f1`. Pin: SemStreams `8b99efe9`. Inventory: `inventory.md` (INVENTORY
PASS, round 2). Per-PR detail: `split.md` (what each PR carries, its lines, its issues, its required corrections,
its ADRs and guidance).

## What this plan settles, and what it leaves

It settles the cut, the order, the issue each PR retires, and which designs go to Fable first. It does not settle any
PR's shape. Each PR is its own OpenSpec change with its own inventory (shape sweep, open issues, pin probe including
the lifecycle suite for its owner, ADRs, the test-side compile drift) and a design of about one page.

## Options considered

- **A. Bottom-up layers (recommended).** Each PR adds a layer that compiles on the ones below; no earlier file is
  reopened. Cost: nothing a product can start exists until PR 8, the carried-in-tree precedent (architect contract,
  "One package per pull request").
- **B. Vertical first slice.** The processor, the message path, `publish` and `lifecycle_transition` first, so
  semboids runs about three PRs sooner. Cost: the dispatcher, the evaluator and the processor are each reopened by
  three or four later PRs, and the early processor carries temporary refusals.
- **C. One PR per lifecycle owner** (`Processor`, `CronScheduler`, `ConfigManager`): three PRs of 10,000 to 30,000
  lines, which the guide exists to avoid.
- **D. Defer.** Epic #9's order ranks this next; deferring keeps semteams and semboids on SemStreams.

## Owner decision 1: the graph-event path (ruled: drop)

The ruling on #101 did not port `graph/events.go` and left rule's use of it to this port: "moves to the
mutation-request protocol or is dropped" (#101 comment 6126673987; the change 2 design D16 leaves both open). When a
message rule fires, the pin builds an update for the rule's trigger entity and publishes it to `graph.events.*`,
which nothing subscribes to. It reaches PR 2 (`Rule.ExecuteEvents`, `interfaces.go:41`), PR 4 (`publisher.go`,
`graph_event_identity.go`), PR 6 (`expression_factory.go:329-366`) and PR 9 (`message_handler.go:207-231`).

Ruled by the owner on 2026-11-25: drop, since decision 2 does not need the event (the gate's counter is separate
from the event, which sits behind the gate at `message_handler.go:188-231`).

- **Drop (ruled).** `ExecuteEvents`, the `Event` type, the graph-event publisher and the trigger identity are
  not carried (about 200 production and 800 test lines). ADR-076's decisions 1-3 and 5 (trigger and alert identity)
  are not ported; its decision 4, the pack ID as producer identity, is already kept by `pkg/rulepack` (ledger :3044).
  What a reader sees change: `total_triggered` (`processor.go:1456`) counted only firings whose graph events
  published, so PR 9 counts each firing that runs its actions instead; `events_published` keeps counting `publish`
  actions and `rule_events` notifications, and stops counting graph events; the `rule_events` notification stays.
- **Move (not taken).** Each firing becomes an entity update through graph-ingest's mutation requests, on the ADR-076 identity,
  which PR 4 ports. Every firing is then a graph write, which nothing at the pin applied; PR 4's design names its
  write budget. PR 6 then waits for PR 4 (`expression_factory.go:349` calls `graph_event_identity.go:22`), and
  `total_triggered` counts applied mutations.

Either way the in-memory cooldown stamp in `ExecuteEvents` (`expression_factory.go:363-364`) does not survive: #702
moves the cooldown into stored state in PR 7.

## Owner decision 2: `fire_every_n_events` (ruled: gate the actions)

The key is documented to fire a rule's actions only on every Nth match (`rule_factory.go:44-56`). At the pin it gates
only the graph event, the `rule_events` notification and the counter (`message_handler.go:174-232`); `on_enter`,
`on_exit` and `while_true` actions run earlier and ungated (`:135-150`, `stateful_evaluator.go:426`). semteams hit it
(`configs/rules/ops/01-run-terminal-observe.json:59`) and sets the key in no checked-in rule. Cron rules are the
exception: there the key does skip the actions on all but every Nth tick (`cron_scheduler.go:496-504`, proved by
`TestCronScheduler_FireFireEveryNGate`), and PR 11 carries that gate either way. On message and entity rules, carrying
the key as is would port a described behaviour nothing implements (surface audit (c)), so one of these is required:

Ruled by the owner on 2026-11-25: gate the actions as documented.

- **Refuse the key on message and entity rules (not taken).** PR 2 refuses `fire_every_n_events` at load on any
  rule that is not `type: "cron"`, naming rule and file; PRs 9 and 10 carry no gate. Its counter is per rule, not per
  entity, the same flaw #702 fixes for cooldown.
- **Gate the actions as documented (ruled).** PR 9 and PR 10 apply the gate before the evaluator runs actions, and PR 7's
  design says whether the count is per rule or per rule and entity.

## The PRs (detail in `split.md`)

| PR | Change id | Retires | Waits for | Fable first |
| --- | --- | --- | --- | --- |
| 1 | `rule-core-01-expression` | none | none | no |
| 2 | `rule-core-02-definitions` | #26 | 1 | **yes**: new exported framework surface (#25) |
| 3 | `rule-core-03-context` | none | 2 | no |
| 4 | `rule-core-04-graph-actions` | #27 | 3 | **yes**: dispatch surface; graph writes |
| 5 | `rule-core-05-state-actions` | #294 | 4 | **yes**: framework-owned buckets; lifecycle state |
| 6 | `rule-core-06-expression-rules` | none | 3 | no |
| 7 | `rule-core-07-evaluator` | #25, #28, #702 | 5, 6 | **yes**: durability |
| 8 | `rule-core-08-processor` | none | 7 | **yes**: boot path; connection ownership; registration |
| 9 | `rule-core-09-message-input` | none | 8 | **yes**: acknowledgement and durability |
| 10 | `rule-core-10-entity-input` | none | 9 | **yes**: connection ownership; readiness |
| 11 | `rule-core-11-cron` | none | 8 | **yes**: durable timer; its own lifecycle owner |
| 12 | `rule-core-12-hot-reload` | none | 10 | **yes**: boot path; writer of `rules.*` |
| 13 | `skill-714-loop-supervision` | #714 | 14 | no |
| 14 | `feat-703-deadline-timer` | #703 | 12 (probe: none) | **yes**: durability; new surface; ADR-111 Decision 4 |

## Merge order

1, 2, 3; then 4 and 6 in parallel (5 follows 4); 7; 8; then 9 and 11 in parallel; 10
after 9; 12 after 10; 14 (its probe at any time); 13 last, so its worked examples are real code, the deadline timer
included. PRs from 8 on all edit `processor.go`; the later one merges `origin/main`. semboids qualifies after 9.
semteams qualifies after 12, once it registers `publish_agent` and migrates its five `replace_owned` files.

## Proofs this plan changes

- #25 retires at PR 7, not 5: its proving test loads and fires semboids' core-only pack, which needs the expression
  rule (PR 6) and the evaluator (PR 7). PRs 2, 4 and 5 name #25 without a closing keyword.
- #26's accepted property (family decode equals `agentic.ToolChoice` decode, SETUP 03B D4a) needs agentic types this
  repository does not have. PR 2 proves byte preservation of `tool_choice` and `response_format` against semteams'
  fixtures at `a331910e` instead.
- #27's "semteams 40/40 `publish_agent` actions dispatched" becomes semteams' check at its cutover; here, PR 4 proves
  the import guard and a registered fake family's dispatch.
- The pin's owner-proof test files are not carried as written. Two orderings they check are not in the
  `lifecycle-suite` spec: `Stop` drains the JetStream consumers before the run context ends
  (`lifecycle_owner_test.go:118`), and cron's `Stop` joins an admitted fire before the `Start` context ends
  (`test_owner_proof_test.go:14`). PRs 8 and 9 re-prove the first and PR 11 the second, through production seams.

## Who carries the surface (adopter seam)

semboids calls `rule.Register` and sets `Dependencies.LifecycleManager` (`cmd/semboids/main.go:200`). semteams calls
`rule.Register`, `NewConfigManager`, `ConfigManager` and `HotReloadTarget` in production. After the port they must
register any family their packs use (`publish_agent` for semteams), hand the lifecycle manager to the rule
registration, and drop the four dead keys. Doing nothing gives a boot
error naming the cause for each; none fails silently. PR 2's Fable read decides the registration surface for
families and the manager, so PR 8 implements an approved shape.
