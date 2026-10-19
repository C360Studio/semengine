# Codex review, round 1: F1 and F2

Two findings from the Codex review of PR #320 at `207da6d` (PR comment 6276828446). F1 is tracked in #326.

## F1 (HIGH): a tail wildcard matched no trailing token

At the pin, `matchTokens` returned true on reaching `>` without checking that a subject token remained, so
`events.>` matched `events`. A NATS output on `events` was joined to a required input on `events.>`, and that input
was never reported orphaned. The package's README, `doc.go` and `SubjectCovers` test all say `>` stands for one or
more trailing tokens.

What changed against the pin:

- `matchTokens` accepts `>` only when at least one subject token is left (`return j < len(subjectTokens)`).
- The pin's assertion in `flowgraph_patterns_test.go` (`foo.>` matches `foo`, "zero tokens") now asserts the
  opposite.
- New `TestTailWildcardNeedsATrailingToken` in `declarations_test.go` goes through `BuildFromDeclarations` with ports
  projected by `Port.Facts()`: with a publisher on `events`, there is no edge and the subscriber on `events.>` is
  orphaned with `no_publishers`, `Required: true`; with a publisher on `events.created`, there is one stream edge on
  `events.created` and no orphan.
- The ledger row's contract gains clause (8) and the new test is listed among its proving tests.

`TestMatchNATSPatternIsSymmetric` still passes. Its subject set already holds the `a` / `a.>` pair (the `foo` /
`foo.>` shape), so it was not extended.

Red run before the fix (`GOWORK=off go test -count=1 -run 'TestTailWildcardNeedsATrailingToken|TestFlowGraphPatterns' ./internal/flowgraph`):

```text
--- FAIL: TestTailWildcardNeedsATrailingToken/no_trailing_token
    Error: Should be empty, but was [{{publisher out} {subscriber in} stream events}]
    Error: []flowgraph.OrphanedPort{} does not contain flowgraph.OrphanedPort{ComponentName:"subscriber",
           PortName:"in", Direction:"input", ConnectionID:"events.>", Pattern:"stream", Issue:"no_publishers",
           Required:true}
--- FAIL: TestFlowGraphPatterns/NATS_wildcard_pattern_matching
    flowgraph_patterns_test.go:313: Should be false; > needs at least one trailing token
```

After the fix, `GOWORK=off go test -count=1 -race ./internal/flowgraph` passes.

## F2 (MEDIUM): request-port prose overstated the code

`README.md` and `doc.go` said matching request ports join "whatever their direction". The code joins an output and
an input with matching subjects; its same-direction loops skip equal connection IDs, so two inputs on `query.get`
do not join. The prose now says so. No behaviour changed. `notes/2.2.md` keeps its original wording as the record of
that task; this note supersedes it.
