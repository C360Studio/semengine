# Tasks: fix-674-lexical-body-revisions

Tests come first. Each code task ends at a clean-tree local `task verify`. "Shown to fail" is failing-first output
in `notes/<task>.md` or `task mutate:check`. Premises cite `proposal.md`. Single PR (#675, `Closes #674`).

## 1. Proposal

- [ ] 1.1 Architect proposal and spec delta (`proposal.md`, `specs/graph-lexical/spec.md`): a revision with the same
  reference and inline text re-indexes nothing and reads nothing; a body read serves its reference, not its
  revision. `task spec:check` and `task docs:check` pass. Hold: OPEN-Q proposal awaits owner approval

## 2. Code

- [ ] 2.1 Tests first (`processor/graph-lexical/apply_prop_test.go`). Premises: every revision puts the inline text
  alone and queues a read (`apply.go:150-160`); a read is indexed only at its own revision (`apply.go:170`, `:185`);
  the test helper gives every revision of a key the same reference (`apply_test.go:48-50`). Accept: the model keys
  body words by reference; deliveries draw "keep the reference" or "new reference" (via `text.bodyKey`) and "keep
  the inline text"; `mustDeliver`'s read expectation follows the model, not `s.offloaded` alone. New witnesses: a
  non-text revision after a successful read keeps the body's word and asks for no read; three same-reference
  revisions while the first read is out end with the body indexed and every revision complete after that one read;
  an inline-text change with the same reference asks for one read and shows the new inline words at once; a
  reference change drops the old body's word at once; a `missing` body with an unchanged revision text is not read
  again; the reference stays and the inline text changes while the read is out: the late read is indexed joined to
  R+1's inline text, R+1's inline words are found beside the body word, and R+1 completes (shown to kill a mutant
  that joins the body to the inline text saved when the read was queued, in `notes/2.1.md`); a revision equal but
  for the reference's `Size` or `ContentType` asks for a read. `TestStaleBodyReadDropped` gives R+1 its own `bodyKey`. `notes/2.1.md` records the witnesses and the
  generated run failing on today's code, with seed. Generated checks: the history is order-dependent, so the
  property model is the check; the witnesses name the cases it must reach.
- [ ] 2.2 Implement in `processor/graph-lexical/apply.go`: keep each offloaded entity's inline document and reference
  after its read; `deliver` skips the put and the read when both are equal; `wanted` and `bodyDone` match on the
  reference while the key waits on it, join the body to the latest inline text, and complete the key's latest
  revision. The no-op path still advances the held revision, so an older delivery stays ignored; a revision that
  removes the reference clears the held reference and inline text. Equality is `==` on the whole
  `StorageReference`. Update the `applier`, `held`, `pending`, `deliver`, `wanted` and `bodyDone` comments and the README's
  body section (plain words, no history) with the producer contract: a reference names fixed content and new
  content needs a new reference (whole-value equality); content replaced behind an unchanged reference is read again
  only when the reference or inline text changes, or at start. Accept: 2.1's tests pass;
  `TestDeliveriesAndBodyReadsConcurrent` passes under `-race`; full `task verify` passes on a clean tree.
- [ ] 2.3 Archive: `openspec archive fix-674-lexical-body-revisions --yes` promotes the delta; `task spec:check` and
  `task docs:check` pass.
