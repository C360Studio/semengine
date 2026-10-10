# Bound required work during a port

## Why

The shape sweep currently turns every inherited maintenance finding into a required refactor and ledger adaptation.
That expands port scope even when the carried structure meets the admitted contract. The owner approved a bounded
replacement in this conversation on 2026-10-10: "Implement this draft as one documents-only PR, preserving the
existing correctness gates and approved commitments."

## What Changes

- Put one required-work rule in `.agents/protocol.md`: a blocking correction names its binding obligation and evidence.
- Reconcile the architect, reviewer and `AGENTS.md` wording so optional inherited cleanup cannot block approval.
- Explicitly revise #159's automatic refactor mapping while preserving its sweep and package-size rules.
- Preserve correctness gates, new-surface rules, approved tasks, package-specific ADRs and #170's prerequisite.

## Capabilities

No capability spec changes. This changes agent conduct in its existing canonical homes; `.openspec.yaml` declares
`skip_specs: true`. No runtime, test, gate implementation or dependency changes.

## Impact

Addresses epic #9. Four existing guidance files change, plus this OpenSpec record. The developer contract already
prohibits opportunistic refactors and does not need another rule. Enforcement remains independent review only.
