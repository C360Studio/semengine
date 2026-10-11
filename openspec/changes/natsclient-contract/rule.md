# The library-probe rule (#170): exact text and insertion points

The base is `main` at `0e13a73`. Each block is inserted whole, as new lines after the line named; nothing that
exists is changed. Lines are wrapped at 120 columns, as the files are. The owner put the rule in all four role
contracts (#170 comment 6109833136, answer to Q8).

## 1. `AGENTS.md`: a row after `:99`

The row goes after the pin-probe row (`:99`), and before the pin-ADR row (`:100`).

```markdown
| A statement a change adds or edits (spec requirement, design statement, ADR, doc comment or README) about how a library (nats.go, nats-server or any other) times, orders, bounds or fails cites a probe of that library at the version the tree pins (a test in the tree that drives the pinned library, not a fake, or a program posted on the pull request with its source, command, versions and output), or says the behaviour is not promised; text carried unchanged from the pin falls under `.agents/protocol.md` § Bounded port scope (#170) | architect contract § Design discipline; reviewer contract § Contract and task-truth review; developer contract § Guarantee, signal, and revision contracts; technical-writer contract, rule 10; ADR-112 | review only. A check could flag a SHALL sentence holding "bound", "bounded" or "within" that names no probe; on `main` it would flag 18 sentences, 4 of them about nats.go, and miss every claim that is not a SHALL sentence (68 of the 103 rows on `main` in PR #177's inventory are outside a spec), so none is built |
```

## 2. Architect contract: a bullet after `:350`

In `.agents/contracts/semengine-architect.md`, § Design discipline, it goes after "**Specify behaviour, not
mechanism.**" (`:346-350`), and before "Extend the model, never build a
channel beside it." (`:351`).

```markdown
- **Measure a library's behaviour; do not assume it.** A statement that a design, spec requirement, ADR or doc
  comment adds or edits about how a library (nats.go, nats-server or any other) times, orders, bounds or fails cites
  a probe of that library at the version the tree pins: a test in the tree that drives the pinned library, not a
  fake, or a program posted on the pull request with its source, command, versions and output. Without one, the text
  says the behaviour is not promised. Text carried unchanged from the pin falls under `.agents/protocol.md` § Bounded
  port scope, which adds no experiment per finding. PR #48's `Close` bound and PR #156's first specs promised more
  than nats.go gives (#170; ADR-112).
```

## 3. Reviewer contract: a bullet after `:151`

In `.agents/contracts/semengine-reviewer.md`, § Contract and task-truth review, it goes after "**Reject
artifact-free evidence.**" (`:150-151`), and before "Confirm code matches the active OpenSpec
target" (`:152`).

```markdown
- **A library claim with no probe is a finding.** A statement the change adds or edits about how a library times,
  orders, bounds or fails, with no probe of that library at the pinned version behind it and no "not promised" in
  its place (architect contract § Design discipline), is a finding; so is a cited test that drives a fake. Text
  carried unchanged from the pin is reviewed under `.agents/protocol.md` § Bounded port scope.
```

## 4. Developer contract: a bullet after `:162`

In `.agents/contracts/semengine-developer.md`, § Guarantee, signal, and revision contracts, it goes after "**A
skip, drop, or degrade is a declared event, never a private choice.**" (`:158-162`), and before
`## Runtime footguns` (`:164`).

```markdown
- **A library claim in code names its test.** A doc comment or README line you add or edit that says how a library
  (nats.go, nats-server or any other) times, orders, bounds or fails names the test in the tree that drives the
  pinned library, not a fake, or the probe posted on the pull request; without one it says the behaviour is not
  promised. When a fix changes such behaviour, that test is the failing-first test (#170; ADR-112).
```

## 5. Technical-writer contract: rule 10 after `:36`

In `.agents/contracts/semengine-technical-writer.md`, it goes after rule 9 (`:33-36`). Line `:37` is blank and
`## Handoff` is `:38`, both on `0e13a73` and on `043fda2`. Insert the block after `:36`, so the existing blank line
stays between it and the heading.

```markdown
10. A sentence about how a library (nats.go, nats-server or any other) times, orders, bounds or fails is written only
   where the change names the test or probe behind it; otherwise write that the behaviour is not promised. Ask the
   developer for the test's name rather than inferring the behaviour from code (#170; ADR-112).
```

## Measurements behind the row's last cell

These come from PR #177's inventory, revision 2, §3: `python3 -I count.py <tree>` over `openspec/specs/*/spec.md` at
`c713840`.

- It finds 282 SHALL sentences.
- 18 of them hold `bound`, `bounds`, `bounded` or `within`, and none names a probe.
- 4 of the 18 are about nats.go (`transport-client/spec.md:54-58`, `:85-91`, `:209`, `:322-327`).

The inventory's 103 rows on `main` include 68 outside a spec: README, doc comments and ADRs (M36-M96, M98-M104).
