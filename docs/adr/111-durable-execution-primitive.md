# ADR-111: Durable Execution Generalises What the Pin Already Does

## Status

**Accepted (2026-10-10).** Ruled by the owner on #9 (comment 6097814764): Decisions 1 to 6, and all eight owner
questions as recommended (the journal's home deferred to a qualifying tier-0 consumer). Drafted by the
semengine-architect on draft PR #165 (`Addresses #24`) under epic #9's "Ruled 2" (comment 6085243841): one of four
records change 3's design cites. The record is the decision, not the implementation: mechanics go to the epic's
capability spec. It decides only what a change-3 or change-4 package needs now, generalised from what the pin has;
everything else is an owner question with options and costs. SemStreams facts are read at **the pin**, the frozen
SemStreams commit `8b99efe9`, cited `path:line`; SemEngine facts are on `main` at `522bf3a` or on PR #93's branch at
`3d66c91` (cited `#93:path:line`). The inventory behind it is the claim's inventory file on PR #165.

## Context

SemEngine has two halves (#8, 2026-10-01, comment 5930151428): the live knowledge graph, and "the **durable-execution
half** (workflows that survive restarts, replay, settlement, retries with known outcomes, timers/signals, and the rules
that drive them)". The port so far carries, for the second half, `pkg/lifecycle` (entity workflows: a graph entity's
declared phases, moved by a compare-and-set write) and `pkg/projection` (the typed write client with three outcomes:
committed, not committed, commit unknown). The owner named those two as "the primitive" and the pin's five composition
points as the input (#24, 2026-10-01; `internal/boot/run.go:184,220,299,330,339`, two of which wire the rule
processor), then asked for this record on four mechanics (#24, 2026-10-09):

- a **journal**: a durable record per **unit of work** (one delivery, one rule firing, one tool call, one loop) that
  says what was attempted and how far it got;
- a **terminal owner**: the one writer allowed to record a unit's final outcome, once;
- an **effect fence**: whatever stops an effect from being applied twice after a restart;
- **replay on boot**: what the engine does about unfinished units after a restart.

`docs/contract.md:100-106` tells consumers these are "not in this release" and points at #24. Neither package has them
(inventory, category 1). The pin has each mechanic in two to four homes (category 2): the rule processor's `RULE_STATE`
revision guard, `OnRecovery` opt-in, actions-then-persist order and `RULE_SCHEDULES` timer
(`processor/rule/stateful_evaluator.go:148-161`, `:172-181`, `:238-249`, `:466-470`; `schedule_tracker.go:1-8`);
agentic-tools' immutable outcome with "deliberately no claimed/in-progress state" and its retry-before-effect rule
(`processor/agentic-tools/outcomes.go:21-24`, `:58-62`, #759); agentic-loop's terminal owner and record
(`terminal_owner.go:118-164`, `component.go:3245-3259`); graph-ingest's applied-sequence guard (`component.go:632-639`,
ADR-072); delivery settlement (`natsclient/delivery_settlement.go:17-33`); gated-DAG's claim and markers as triples on
the unit (`gated-dag/claim.go:36-58`; ADR-046 `:349-351`, ADR-064, ADR-070; not ported). The SemStreams decisions that
bind: live in `ENTITY_STATES` by default, a private bucket needs an ADR on the rubric (ADR-049 `:141-178`);
graph-ingest is the sole physical writer, a birth is an atomic create, every other write is a compare-and-set at a read
revision, a lost reply is `commit_unknown`, no outbox or exactly-once mechanism (ADR-091 `:40`, `:58`, `:60-69`); an
effect is idempotent or carries a stable deduplication key, and no exactly-once across NATS and an outside system
(ADR-094 `:122-127`); effect precedes guard precedes ACK, and a **lane** (one durable consumer's handler path) that
cannot declare idempotency declares
at-most-once (ADR-095 `:46-48`); replay is bounded catch-up (ADR-090 D1); stranded work is "an alert only, never
auto-re-dispatch" (ADR-070 D4, `:105-110`); shared mechanics only "after at least three surviving owners need the same
behavior" (ADR-090 D9, `:59-60`). Each is being re-derived per package inside port pull requests (#9 "Ruled 2").

No tier-0 consumer of a journal is confirmed today (inventory, category 4): semsource re-publishes from its source
after a memory-stream loss (`docs/contract.md:73`) and its owner wrote that journal, replay and fences "must not be
ported" (foundation `inventory.md:541-542`); semteams' `agent-run` is the agentic tier; semboids is unmeasured.

## Decision

Decisions 1 to 6 are taken now; each generalises a shape the pin has and is what a change-3 or change-4 durable owner
(`service`, the first `ENTITY_STATES` followers, later the rule processor) needs to cite. Questions 1 to 8 are the
owner's; the journal's home and store are among them.

### Decision 1 — A phase is entity state; an attempt is not a phase

A fact a rule, a query or a dashboard reasons over — the phase, who owns the mission — is **entity state**: a fact on
the entity in `ENTITY_STATES`, changed through the **write seam** (graph-ingest's compare-and-set write, ADR-110's) and
carried by `pkg/lifecycle` as it is (ADR-047 "what this is not", `doc.go:7-20`; ADR-049 kept). The record of an
attempt, an outcome, a guard value or a parked delivery is **execution state**: it is never modelled as a phase, and a
terminal phase is never inferred by the engine from an execution outcome (ADR-053 D3 kept: the product or a rule sets
it, `pkg/lifecycle/manager.go:761,827`). `pkg/projection` is the entity-write edge: its receipt is held for one call
(`mutation_types.go:80-83`). Where execution state is stored is Question 2.

### Decision 2 — An outcome is written once, after the effect, by one owner

A unit's outcome is recorded after its effect, with the store's create-once write (`natsclient/kv.go:211-212`,
`ErrKVKeyExists` `:219`); a refused create means an outcome already exists, and the owner adopts it by the unit's
identity (`terminal_owner.go:122-126`) or republishes it without a second effect (`outcomes.go:58-62`). A record that
more than one process may advance is advanced only by a plain compare-and-set at the revision the writer read
(`kv.go:231-239`); a lost compare-and-set means another process advanced the unit, so this one re-reads and never
retries in place (`processor/agentic-loop/component.go:3250-3254`). A record only one process writes (ADR-090 D5) may be
written with `Put`, as `RULE_STATE` (`processor/rule/state_tracker.go:157`) and the ingest guard
(`processor/graph-ingest/keyed_ingest.go:295`; `#93:…/keyed_ingest.go:361`) are. Before the attempt the pin has two
shapes, and a unit kind declares which it uses: **no claim** — the outcome is the first and only record of the attempt
("There is deliberately no claimed/in-progress state", `outcomes.go:21-24`; actions first, state after,
`stateful_evaluator.go:238-249`); or **claim before dispatch** — a marker committed on the unit before the work is
published, so a crash between the two is found as a claimed unit, not re-derived as ready (ADR-046 `:349-351`, whose
`replace_owned` marker its amendment note `:14-18` retires; the live claim is `gated-dag/claim.go:36-40`, and ADR-070
`:127` keeps the claim marker). Which shape an external attempt with no fence of its own takes is Question 3.

### Decision 3 — Retry only before the effect; a blocker is a classified error

For an effect that is neither idempotent nor fenced (ADR-095 `:46-48`), a delivery may be retried only for a typed
transient result whose effect is proven not to have begun (#759, `outcomes.go:58-62`;
`agentic-dispatch/terminal_settlement.go:23-26`); a result that may have had such an effect, including every `commit
unknown`, is never retried: it is quarantined (`agentic-loop/component.go:1330-1336`), and the caller re-reads before
acting again (`docs/contract.md:61-62`). A fenced or idempotent effect may be redelivered after it ran: graph-ingest
sends a Nak after its effect when the guard stamp fails (`#93:processor/graph-ingest/keyed_ingest.go:233-245`) and on a
transient ingest error (`:227`); agentic-tools sends one to republish an outcome that is already durable
(`outcomes.go:58-62`). An effect that cannot carry a stable idempotency key is declared at-most-once on its lane
(ADR-095 `:46-48` kept); the engine claims no exactly-once across NATS and an outside system (ADR-094 `:125-126` kept).
A **blocker** — why an attempt stopped and whether it may be retried — is an `errs.ClassifiedError`: class, stable code,
detail, matched with `errors.Is`, never parsed from text (ADR-060; #162). The delivery decision and the commit state are
derived from it at their edges, not stored as a second spelling. One defect is named and given one home: `errs.Classify`
defaults an unclassified error to Transient (`pkg/errs/errs.go:279-280`), so a commit-unknown emit error is spelled
retryable (`#93:pkg/lifecycle/graph_emit.go:110`); filed as #167.

### Decision 4 — Effect, then guard or record, then acknowledgement

The order every durable owner keeps is ADR-095's (`:46`): the effect commits, the durable guard or record is written,
then the delivery is acknowledged; a crash after the guard is written and before the acknowledgement redelivers, and the
guard makes the second application the same as one (ADR-094 `:122-127`; `#93` recovery spec `:28`, `:44`); the window
between the effect and the guard is Decision 5's. The fence on a NATS write is the store's own: `Nats-Msg-Id` within a
duplicates window for a stream publish (ADR-070 D1 kept; `main:natsclient/ client.go:1488`), the revision for an
`ENTITY_STATES` write (ADR-091 §3 kept; the seam ADR-110 owns), the applied-sequence guard for a stream input (ADR-072
kept; the rule processor's `SourceRevision` guard is the same shape, `stateful_evaluator.go:148-161`). Claim leases and
timer-driven re-dispatch are rejected (ADR-070 D5 kept).

### Decision 5 — Recovery is the pin's three; a re-run is a crash window or an opt-in

After a restart: (1) the transport redelivers unacknowledged work on a file-backed stream, over the fence
(ADR-070 D1; `docs/contract.md:74`); the guard is written after the effect (Decision 4), so a crash between the two
is a window the guard does not cover: the redelivered effect runs again, and that is why the effect must be
idempotent or carry a stable key (ADR-094 `:122-127`). The rule processor says it of itself: an aborted persist
"makes bootstrap re-derive a world where the actions never happened … and OnEnter/OnExit double-fire"
(`stateful_evaluator.go:238-246`). (2) Outside that window an owner re-fires only where it explicitly opted in: the
rule processor's `OnRecovery` or `RerunOnRecovery` decide only whether a **saved** match fires again at bootstrap
(`stateful_evaluator.go:127-132`, `:172-181`), because "Rules without either are assumed to have side effects that
must not re-run on restart" (`:466-470`); (3) work that is neither redelivered nor opted in is **reported** — a stranded
unit is an alert, never an automatic re-dispatch (ADR-070 D4 kept, `:105-110`). What is unfinished is read from the
store, never from memory (`loop_presence.go:19-21`);
a record that cannot be read is unknown, never stale (`:41-45`); a lost memory stream is re-published by the source
(`docs/contract.md:73`). Replay is bounded catch-up (ADR-090 D1 kept). Automatic re-entry of live units is Question 4.
When recovery runs relative to readiness is ADR-109's; the shell the recovering owner runs in is ADR-108's.

### Decision 6 — Settlement is the delivery edge, kept as it is

`natsclient`'s delivery settlement — a decision and its cause from the owner's work, settled to the broker as ack, nak
or term, with a progress heartbeat and a panic quarantined — is the primitive's edge to a JetStream delivery
(`delivery_settlement.go:291-390`; `main:openspec/specs/transport-client/spec.md:23`; graph-ingest its first non-agentic
caller, #8 Q18). It "owns no consumer lifecycle or restart state" (`main:delivery_settlement.go:309-312`) and stays so:
nothing it holds survives a restart, so it records no attempt and no outcome. `Quarantine` stays the decision for a
result that may have had an effect (Decision 3). A delivery parked after `MaxDeliver` is recorded and visible
(`internal/maxdelivery/observer.go:1-6`; `docs/contract.md:85-89`), a blocker sink and not a journal.

### Owner questions (each with options and costs; recommendation first)

**Ruled 2026-10-10 (owner, #9 comment 6097814764): all eight as recommended.** The questions, options and costs stay
below as the record of what was weighed.

1. **The journal's home.** The 2026-10-01 ruling names `pkg/lifecycle` + `pkg/projection`. Options: (a) **defer**
   (foundation `design.md:653-654`, option 2; recommended): Decisions 1–6 bind now, both packages stay as carried, and
   the journal's home is decided when a tier-0 consumer names a workload; cost: the journal stays undesigned, and
   ADR-090 D9's three owners are not yet surviving (graph-ingest's guard on #93, the rule processor at change 6, the
   agentic owners at tier 2). (b) Extend the ruled home: cost: `pkg/lifecycle` says it is not a workflow engine
   (`doc.go:7-20`) and every consumer that wants phases would import the journal; `pkg/projection` is a write client.
   (c) A third package: cost: departs from the ruling, adds surface with no confirmed tier-0 consumer, and precedes D9's
   three owners. 2. **Where execution state lives.** (a) Triples on the unit's entity (ADR-049's default; gated-DAG's
   shape, `claim.go:36-40`): one store, rules and queries see it, no bucket ADR; cost: every attempt is a graph write
   through the one writer, `History=1` keeps no attempt history unless each attempt is its own triple on the current
   entity (the 64-occurrence window ADR-049 `:163-167` uses for transitions), and write rate is on the rubric's
   exception list. (b) A bucket per execution kind (the pin's shape for loops, tool outcomes, rule state and the guard):
   CAS over a multi-field record, retention apart from the graph; cost: an ADR per bucket on the rubric (`:175-178`), a
   catalog row, and a cleanup path. Recommended: (b) for attempt records, (a) for anything a rule must match on, decided
   per execution kind in the epic. 3. **A claim before an external attempt.** The pin has both shapes (Decision 2): no
   claim for tool calls and rule firings; claim before dispatch for gated-DAG units (ADR-046 `:349-351`,
   `gated-dag/claim.go:36`). Option: the claim shape for every external attempt makes a stranded unit visible to ADR-070
   D4's detector; cost: it reverses agentic-tools' deliberate choice and the rule processor's order, and a stranded unit
   is indistinguishable from a slow one without a wall-clock knob (ADR-070 `:111-115`). Recommended: not decided here;
   the epic may propose it per execution kind with that cost. 4. **Automatic re-entry of live units at boot.** Against
   ADR-070 D4. Option: re-dispatch every non-terminal record at boot; cost: a double-run when the unit is healthy on
   another process (ADR-090 D5 makes it rarer, not impossible) and the knob above; benefit: no operator in the loop.
   Recommended: alert only, as the pin. 5. **Unit identity.** ADR-105 mints a UUID for loops only; the guard is keyed
   `entityID/streamName` (`processor/graph-ingest/component.go:634-635`), gated-DAG units are entity IDs. Option: a
   framework-minted key for every unit kind; cost: re-keying existing guards, one more mint seam. Recommended: each kind
   keeps the pin's key; ADR-105 for loops. 6. **Timers and signals.** Named in the half (#8), ruled nowhere.
   `RULE_SCHEDULES` is a tier-0 durable timer that ports with the rule processor (change 6); the approval sweeper (#24's
   body) is tier 2. Options: inside #24; their own epic when a consumer names them (recommended); out of scope. Cost of
   "inside #24": #24 grows before it has a consumer. 7. **#138 items 1–2.** The five single-method interfaces and the
   alias package have no reader; recommended: release the hold of K1 (PR #93 design D6's "kept with a reason" item for
   the two packages' unread surface) and land them after #93, because nothing uses them (not because they are outside
   the primitive: they are the write edge Decision 1 includes). Cost of keeping: API nobody asked for. 8. **What is
   decided before a tier-0 consumer qualifies.** Recommended: Decisions 1–6 (pin shapes change 3 and 4 need); Questions
   1 and 2 wait for the consumer. Cost of deciding the journal now: invented surface (#8's rule) and a second round when
   the consumer's workload differs.

### Left to the implementation epic (#24)

The Go surface and its home (Question 1); the store per execution kind (Question 2); the record's fields; how records
of finished units are reclaimed (a TTL is already forbidden on a bucket that must not be evicted, `AGENTS.md` storage
rule, so reclaim is an explicit step, as `Despawn` is for entities); the blocker code list and the derivation tables
to the two edges; the proving tests (#24 "pass evidence": restart mid-attempt, duplicate
delivery, replay on boot); the migrations named in the inventory's adoption sweep; semsource's part, if any.

## Consequences

- A change-3 or change-4 owner cites Decisions 1–6 and re-derives none of them; a design that spells any of the six
  facts in category 2 a second time is wrong at birth.
- `pkg/lifecycle` and `pkg/projection` stay what ADR-047, ADR-049 and ADR-091 made them; `Dependencies.LifecycleManager`
  stays dropped (#93 D17); a consumer that only needs phases carries nothing for a journal.
- Nothing in this record is new surface: every decision is a pin shape with its line. The journal itself is not
  decided; Questions 1 and 2 say what it costs to decide it now or later.
- The adoption sweep in the inventory (graph-ingest's guard, the rule processor, agentic-tools, agentic-loop, agentrun,
  the two outcome mappings, maxdelivery) is filed as one tracking issue when the record is accepted.

## Related

- Kept: ADR-046 (the claim is committed on the unit before the work is published), ADR-047, ADR-049 (default
  `ENTITY_STATES`; rubric;
  CAS at a revision), ADR-053 D3, ADR-060, ADR-064, ADR-070 D1, D3, D4, D5, ADR-072, ADR-079 D4, ADR-090 D1, D2, D5,
  D9, ADR-091 §1–3, ADR-094, ADR-095, ADR-105 (loops).
- Changed or dropped: none. ADR-070 D2's `ConsumeDurable` was already retired at the pin (ADR-094 `:102`).
- Boundaries named, decided elsewhere: ADR-108 (#109, the processor shell), ADR-109 (#110, readiness; ADR-083,
  ADR-088), ADR-110 (#141, the write seam and `UpdateWithRetryRead`).
- This repository: `docs/contract.md:51-106`; `openspec/changes/archive/2026-10-01-setup-03b-contract-boundary/
  design.md:570-607`; `…/2026-10-01-setup-04a-foundation/design.md:509,648-655,717-722` and `inventory.md:535-543`;
  PR #93 design D1, D8, D13, D17, D23, K1; issues #15, #18, #19, #20, #38, #138, #141, #154, #162.
