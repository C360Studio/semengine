# Existing port-rule homes

base: `0e118245154a6e3bd8b77f632ca22374d919c494`

Question: which existing instructions turn inherited maintenance findings into required port work?
Scope: SemEngine only; reuse the prior reviewed process audit. No sister repository inventory is needed.

## Conflicting instructions

| Home at the base | Existing instruction | Required reconciliation |
| --- | --- | --- |
| `.agents/contracts/semengine-architect.md:151` | "more than one home is a defect to consolidate toward one" | Qualify inherited structure under the protocol's rule. |
| `.agents/contracts/semengine-architect.md:163` | "The design turns each one into an item on its list of port refactors" | Permit an evidenced optional disposition without a refactor or adapt item. |
| `.agents/contracts/semengine-architect.md:185` | "More than one home is a defect to consolidate toward ONE shared primitive" | Preserve the ban on new duplicates; qualify inherited consolidation. |
| `.agents/contracts/semengine-reviewer.md:132` | "mapped to an item on the design's list of port refactors and an adapt item" | Check disposition, not an automatic correction. |
| `.agents/contracts/semengine-reviewer.md:162` | "A diff that reimplements a shape the repository already owns is a finding" | Distinguish inherited structure from newly introduced duplication. |
| `.agents/contracts/semengine-reviewer.md:377` | "HIGH for a likely functional defect or known project discipline failure" | An optional finding cannot become HIGH through the superseded generic rule. |
| `AGENTS.md:119` | "each finding becomes a port refactor" | Replace this clause and index the new canonical rule. |

## Existing authority retained

`.agents/protocol.md` owns finding routing and delivery gates. The developer contract already says "Avoid
opportunistic refactors" and that listing other adopters does not require migrating them. Admission, correctness,
context, surface, evidence, testing and coverage obligations remain intact. #159's specific sweep and package-size
requirements remain; its automatic finding-to-refactor conversion is the explicit exception. Accepted #93 tasks,
Issue #170 and specific ADR adoptions are not cancelled or made dependent on this change.

## Claims, overlap and review

Live GitHub was checked on 2026-10-10. Open claims are #93 at `f3d5ae56ef62b7caacf0a1bf7f63f0869ab2b047` and
Dependabot #116-120. The complete #93 file list, fetched with the paginated pulls/files endpoint, has no overlap
with the four rule homes. The dependency PRs do not change them either. This documents-only claim can land
independently; a later overlapping content change needs reconciliation before delivery.

The independent Codex SemEngine reviewer passed this inventory and then the exact advisory draft before the owner
asked for implementation. The reviewed draft SHA-256 was
`d81ce23f5f623921b14fecd4f27c0d3990ef5914c0d32a48f41de3859b7e24d3`.
The implementation review will recheck conformance to this target and the six design scenarios.
