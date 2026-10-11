# The library-probe rule (#170): exact text and insertion points

Base: `0a4d6d5` (claim commit over `main` `c713840`). Each block is inserted whole, as a new line or lines, after the
line named; nothing existing is changed. Lines are wrapped at 120 columns as the files are.

## 1. `AGENTS.md`, rule table: a new row after `AGENTS.md:99` (the pin-probe row)

```markdown
| A statement a change adds or edits (spec requirement, design statement, ADR, doc comment or README) about how a library (nats.go, nats-server or any other) times, orders, bounds or fails cites a probe of that library at the version the tree pins (a test in the tree that drives the pinned library, not a fake, or a program posted on the pull request with its source, command, versions and output), or says the behaviour is not promised; text carried unchanged from the pin falls under `.agents/protocol.md` § Bounded port scope (#170) | architect contract § Design discipline; reviewer contract § Contract and task-truth review; ADR-112 | review only. A check could flag a SHALL sentence holding "bound", "bounded" or "within" that names no probe; on `main` it would flag 18 sentences, 4 of them about nats.go, and miss every claim that is not a SHALL sentence (68 of the 103 rows on `main` in PR #177's inventory are outside a spec), so none is built |
```

## 2. The architect contract: a new bullet after `:350`

In `.agents/contracts/semengine-architect.md`, § Design discipline.

After the bullet "**Specify behaviour, not mechanism.**" (`:346-350`), before "Extend the model, never build a channel
beside it." (`:351`):

```markdown
- **Measure a library's behaviour; do not assume it.** A statement that a design, spec requirement, ADR or doc
  comment adds or edits about how a library (nats.go, nats-server or any other) times, orders, bounds or fails cites
  a probe of that library at the version the tree pins: a test in the tree that drives the pinned library, not a
  fake, or a program posted on the pull request with its source, command, versions and output. Without one, the text
  says the behaviour is not promised. Text carried unchanged from the pin falls under `.agents/protocol.md` § Bounded
  port scope, which adds no experiment per finding. PR #48's `Close` bound and PR #156's first specs promised more
  than nats.go gives (#170; ADR-112).
```

## 3. The reviewer contract: a new bullet after `:151`

In `.agents/contracts/semengine-reviewer.md`, § Contract and task-truth review.

After the bullet "**Reject artifact-free evidence.**" (`:150-151`), before "Confirm code matches the active OpenSpec
target" (`:152`):

```markdown
- **A library claim with no probe is a finding.** A statement the change adds or edits about how a library times,
  orders, bounds or fails, with no probe of that library at the pinned version behind it and no "not promised" in
  its place (architect contract § Design discipline), is a finding; so is a cited test that drives a fake. Text
  carried unchanged from the pin is reviewed under `.agents/protocol.md` § Bounded port scope.
```

## Measurements behind the row's last cell

PR #177's inventory (revision 2), §3: `python3 -I count.py <tree>` over `openspec/specs/*/spec.md` at `c713840`
finds 282 SHALL sentences, 18 holding `bound`, `bounds`, `bounded` or `within`, 4 of them about nats.go
(`transport-client/spec.md:54-58`, `:85-91`, `:209`, `:322-327`), none naming a probe. The inventory's 103 rows on
`main` include 68 outside a spec (README, doc comments, ADRs: M36-M96, M98-M104). Not edited: the developer and
technical-writer contracts (owner question 8 in `owner-questions.md`).
