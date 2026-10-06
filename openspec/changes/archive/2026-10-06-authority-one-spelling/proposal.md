# authority-one-spelling

Status: revision 1, drafted after `INVENTORY PASS` (issue #72, round 2) and the owner's ruling on #72 (comment
5969505488, 2026-10-03). It rests on `inventory.md` (the passed inventory, carried verbatim) and `design.md`, which
awaits independent pre-owner design review and then the owner's acceptance.

## Why

The deployment's authority pair (`platform.org`, `platform.id`) is spelled 41 times in the tier-0 port set at the
SemStreams pin `8b99efe9`: one source, one establishing step, one carrier, one composer, and beside them four second
spellings — `message.FederationMeta`, `vocabulary.EntityIRI`, `graph/llm`'s `EntityParts`, and a hand-built
`org.platform` in `graph/clustering`. Round 1 of the inventory listed the first two; the last two, the composers and
the rule engine's run mint were found only by the review's shape-based search (inventory §0, B1–B3). The owner ruled
the rule's wording (A) and its enforcer (C). Without a command behind it the rule is a review-only row, and
SemStreams' record is that review-only rows drift.

## What Changes

- **The rule, as the owner worded it,** becomes a requirement of the `harness-boundaries` capability: a deployment's
  authority (`org`, `platform`) is established once, by `config.Manager.Start` under ADR-104, carried to components
  once as `deps.Platform`, and becomes positions 1–2 of an identity only through `FrameworkIdentityFamily.EntityID`;
  no other package computes, parses, re-joins or carries it, and no message, envelope or metadata holds a copy.
- **Check C-1, names.** The `go/types` public-signature contract (`internal/harness/contract/signatures_test.go`,
  PR #48) gains a second pass: an exported identifier in any non-test package of the module whose name contains
  `Federation`, `GlobalID` or `EntityIRI` fails the test, which names the file, the line, the identifier and the rule.
  It lands after PR #48, which carries the contract it extends; the task says so with `Hold:`.
- **Check C-2, fields.** A new contract test fails on an exported struct field named `Org` or `Platform` in a non-test
  file, unless the field is in `pkg/types`, `pkg/platform` or `config`, is a field of `PlatformMeta` in the
  top-level package `types`, is `processor/rule.CallerContext.Org` (a caller's claim, a different fact), or is the
  carrier `deps.Platform`, excepted by exact field — `component.Dependencies.Platform`,
  `service.Dependencies.Platform`, `processor/rule.Dependencies.Platform` (owner ruling, #72 comment 5969757891). It
  starts green on `main` and at PR #48's head and lands first.
- **Each check carries a paired sensitivity test** that plants the pin's real shapes (`HierarchyConfig.Org`,
  `EntityParts.Platform`, `WithFederation`, `EntityIRI`, `BuildGlobalID`) in a temporary module and requires the check
  to name each, and to stay silent on the owners, the carrier and `CallerContext.Org`.
- **`AGENTS.md`** gains the rule's row, naming both tests under "Enforced by" and what stays review only;
  `docs/testing.md` lists the two sensitivity pairs.

Not in this change: ruling B (the exported "extract after `Start`" entry point, change 3); ruling D (the five
superseded passages, PR #48 and change 2); ruling E (the surface audits of `graph.NewAlertEvent` and
`config.MinimalConfig`); any consolidation of the carrier copies (inventory #19–#25); any check of lowercase `org`
and `platform` fields, which are those copies.

## Capabilities

### Modified Capabilities

- `harness-boundaries`: one requirement modified, "No second spelling of deployment authority" (added by PR #48 with
  the name check): it gains the rule's full statement and the field check, so the two checks are its enforcement and
  the rule has one requirement, not two. No other requirement changes. Its Purpose names the harness's own
  boundaries today and is edited in the archive commit to say it also holds the machine-checked shape rules over
  ported production code, which is what its "No retained context" and (from PR #48) "Public signatures name no
  internal type" requirements already are.

## Impact

- `internal/harness/contract/authority_test.go` (new: `TestNoSecondAuthorityField` and its `…Sensitivity` pair);
  `internal/harness/contract/signatures_test.go` (C-1, `TestNoDeploymentAuthorityNames` and its `…Sensitivity` pair,
  adopted from PR #48 (`bb004ef`), not written here); `AGENTS.md` (one row); `docs/testing.md` (one line);
  `docs/repository-map.md` (one clause).

- Test time: two more `go/packages` loads of the module in the `contract` package per `task test:unit`, and five
  more per `task test:repeat`; budget 15 s over the three steps together, measured in task 2.4.
- At PR #48's head `c64ac338` C-1 names seven identifiers (`message/federation.go`, `message/base_message.go`,
  `vocabulary/iris.go`), every one of which #48 is ruled to remove; C-1 therefore waits for #48 and starts green on
  the head that merges. C-2 names nothing at that head.
- Change 2 ports `graph/inference.HierarchyConfig` (`Org`/`Platform` strings, `hierarchy.go:100-101` at the pin) and
  `graph/llm.EntityParts` (`prompt_types.go:14-15`): both fail C-2 as ported. The change-2 design decides the shape
  (the carrier type, or a constructor); this change records the consequence and decides nothing for it.
- PR #48 also changes `openspec/specs/harness-boundaries` (three requirements), one of them "No second spelling of
  deployment authority" for the name check. This change modifies that requirement rather than adding a second, so
  `harness-boundaries` holds one requirement for the rule; #48 archives first, and this change's archive applies
  after it.
