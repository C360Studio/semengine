# Bounded port scope design

Status: approved target, owner instruction of 2026-10-10. Based on main `0e118245154a6e3bd8b77f632ca22374d919c494`.

The change is to stop automatically converting inherited maintenance findings into required port refactors.
Correctness obligations and deliberate product requirements continue to govern admission.

Keeping the current rules preserves automatic cleanup expansion. Extending the existing “avoid opportunistic
refactors” instruction into one shared scope rule requires four coordinated guidance edits and leaves some inherited
maintenance debt in place. The latter is recommended.

## Approved rule

Canonical home: `.agents/protocol.md`, “Bounded port scope.”

A port implements the admitted capability and its accepted contract. Its reviewed design establishes the work to
deliver. A finding adds required work only when the correction is necessary to satisfy a binding requirement, an
admission or verification gate, an explicit owner ruling, an accepted design commitment, or the dependency and
packaging changes needed to build the admitted package. The finding names that obligation and the concrete failure,
conflicting implementation, or missing required evidence. A maintenance preference alone does not establish required
work.

Required work includes correcting broken integrity, silent-loss prevention, context ownership, completed joins,
authority and readiness, acknowledged durability, and metadata or content preservation. All other explicit
correctness, language, surface, testing and repository requirements remain binding, except the generic
inherited-cleanup mandates this section expressly supersedes. Missing proof of a required guarantee calls for targeted
qualification; it is neither a passing gate nor automatic evidence that a rewrite is necessary. A scope freeze cannot
excuse a broken contract, a newly introduced defect, or an unproved admission gate.

Use the evidence already available: the retained contract, code path, existing reproduction, failing regression, or
required qualification result. State what it proves and what remains unknown. Whether the defect originated at the
source pin or during the port may remain unknown; attribution is not a prerequisite to correcting a demonstrated
defect. This rule adds no pin experiment per finding. Existing pin-probe obligations and evidence requirements still
apply to claims made about pin behavior.

Inherited duplication, copied scaffolding, unnecessary indirection, naming and similar maintenance findings are
optional unless they meet the required-work rule above. “Inherited” means structure retained from the source package,
not every addition that first appears in this repository. This exception does not permit newly introduced duplicate
owners, unconsumed new exports or parallel spellings. The surface audit remains required: truly dead surface is not
protected by this exception, and admitted packages and consumers still count when identifying readers.

The inventory records every shape-sweep finding. For each, the design records either the required correction and its
binding basis, or the retained structure and why changing it is optional. Required corrections receive the appropriate
ledger disposition; an optional finding does not force a refactor or an `adapt` item. Record retained costs through
existing design and ledger fields where applicable. Use the existing finding-routing rule; do not create a separate
cleanup ledger.

An optional finding cannot veto design or implementation approval, become a blocking or high finding merely by
invoking a generic consolidation rule, or add a prerequisite to the MVP path. Present optional work as optional before
design acceptance. Scheduling it as additional work requires an explicit owner decision identifying that work; a
general instruction to keep code simple is insufficient. Required corrections use the smallest complete change that
meets the obligation; unrelated cleanup does not join that correction automatically.

This rule supersedes generic wording that makes every inherited shape finding a mandatory consolidation, port refactor
or `adapt` item. Stronger explicit requirements, package-specific ADR decisions, other owner rulings and accepted
tasks remain binding. Only the owner can change those commitments. Enforcement is review only: within the existing
finding and verdict format, the independent reviewer checks the obligation and evidence supporting required work and
does not block on optional maintenance.

## Exact integration scope

| Home | Edit |
| --- | --- |
| `.agents/protocol.md` | Add the canonical rule above. Keep existing finding routing, claims, review, verification, known-flake, archive and merge gates. |
| `.agents/contracts/semengine-architect.md` | Replace the shape sweep’s unconditional refactor/`adapt` mapping. Qualify the inherited-consolidation demand in inventory category 2 and the matching “one home” instruction. Point to the protocol; preserve inventories, new-surface rules, surface audit and explicit adoption obligations. |
| `.agents/contracts/semengine-reviewer.md` | Replace unconditional shape-to-refactor mapping; distinguish inherited structure from new duplicate owners in pattern review. Reference the optional-finding restriction in verdict guidance. |
| `AGENTS.md` | Replace the conflicting shape-sweep row’s “each finding becomes” clause with the disposition rule. Add one index row: bounded port scope; canonical home `.agents/protocol.md`; enforcement “review only.” |

The developer contract already prohibits opportunistic refactors and says adoption enumeration does not require
migrations; it needs no new rule.

## Transition and acceptance

Adoption revises #159’s requirement that every shape-sweep finding become a refactor and an `adapt` item; the sweep,
package-size rules and specifically directed adoption work remain binding.

On adoption, use this rule for future port designs and newly raised findings on open ports. Existing approved tasks
remain commitments: this proposal does not remove PR #93 tasks, the #170 prerequisite, or package-specific ADR
adoption. This also adds no prerequisite to #93. Changing those commitments requires a separate explicit owner ruling.
It requires no backlog reclassification, new checker, issue-count gate, or broad inventory.

| Scenario | Required outcome |
| --- | --- |
| Inherited copied shell passes required lifecycle checks; no specific consolidation obligation | Record optional maintenance; approval may proceed. |
| That shell fails to join a worker | Correct the lifecycle violation, including after scope freeze. |
| Required restart evidence is absent | Run targeted qualification; do not assume either success or the need to rewrite. |
| Port adds a duplicate resolver or a new export without a consumer | Existing surface rules still require correction. |
| Late inherited-indirection finding is labelled HIGH solely under the old generic consolidation rule | Treat as optional; it cannot delay approval without an explicit owner scheduling decision. |
| Owner-approved stronger guarantee, accepted task or ADR requires a change | Complete it and its required proof even if the old workload passed. |

Before merging the eventual documentation change, independently walk these scenarios, confirm the four homes agree,
and run applicable existing documentation/spec checks and required merge checks. Runtime and automated verification
behavior do not change.

Sources: [architect
contract](https://github.com/C360Studio/semengine/blob/0e118245154a6e3bd8b77f632ca22374d919c494/.agents/contracts/semengine-architect.md#L144),
[reviewer
contract](https://github.com/C360Studio/semengine/blob/0e118245154a6e3bd8b77f632ca22374d919c494/.agents/contracts/semengine-reviewer.md#L129),
[existing developer
restraint](https://github.com/C360Studio/semengine/blob/0e118245154a6e3bd8b77f632ca22374d919c494/.agents/contracts/semengine-developer.md#L35).
