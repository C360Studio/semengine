# Design: fix-369-subject-grammar

base: a88ee36 (claude/subject-grammar-369; inventory `inventory.md`, INVENTORY PASS round 2, committed at 807c367)

## Decision (owner ruling on #369, 2026-10-23)

One new finding type in `composition.Analyze`. Rejected by the owner: moving the grammar so `component` refuses at
decode, and the split option. The grammar stays in `internal/flowgraph` (spec "One subject grammar decides validity,
cover and overlap": no second copy). `component` and `config/stream_bounds.go` (ledger `carry`) are not touched.

## The finding

- Name: `malformed_subject`, constant `TypeMalformedSubject` in `composition/findings.go`, after
  `TypeReservedSubjectCapture`.
- Severity: error. `severityOf`'s default branch already gives error (`composition/findings.go:82-83`); nothing there
  changes. An error finding refuses boot (`service/component_manager.go:349`, before `SealComposition`, after
  construction and before any component starts).
- What it reads, all through `flowgraph.ValidSubject`:
  - every subject of every port whose `PortFacts.Kind()` is NATS, NATS-request or JetStream, in `InputFacts` and
    `OutputFacts` (`PortFacts.NATSSubjects()`, `component/port_facts.go:71`);
  - every subject of every explicit stream in `streams`, keys in sorted order.
- What it names:
  - Port: `Component` is the instance name and `Port` is the port name. Message:
    `input port "in" field "subject": subject "a.>.b" is not a valid subject`, or for a JetStream port
    `output port "out" field "subjects[1]": subject "a*" is not a valid subject`. The lane is named because one
    component may hold a same-named port in each lane (spec, "A same-named output does not hide an input").
  - Explicit stream: `Component` is `config` and `Port` is empty, as for `reserved_subject_capture`
    (`composition/analyze.go:152-153`). Message:
    `stream "EVENTS" field "subjects[0]": subject "a..b" is not a valid subject`.
  - Suggestions (both findings):
    - "Write the subject as '.'-separated tokens, none empty, with no space, tab, line feed, carriage return or form
      feed".
    - "Use '*' and '>' only as whole tokens, '>' only last; a token such as 'a*' is refused here although NATS reads it
      as a literal".
- A stream derived from a JetStream output is not reported a second time under its stream name. Its subjects are the
  port's subjects, already reported on the port.

## Where it runs

It runs immediately after `reservedCaptureFindings` (`composition/analyze.go:33`), before the empty-composition return
(`:37-48`) and the flow-graph-build return (`:51-64`). So it is reported when no component is enabled and when the
graph cannot be built. It reads only facts and streams, as the reserved check does. Output order is fixed by
`finalize` (`composition/findings.go:117-143`).

## Interaction with reserved_subject_capture

The reserved check is unchanged. A subject that fails the grammar "SHALL overlap nothing" (spec, the subject-grammar
requirement), so `flowgraph.SubjectsOverlap` (`composition/analyze.go:140`) never reports it. Each malformed filter
therefore yields exactly one `malformed_subject` and no `reserved_subject_capture`. Today such a filter passes both
checks silently. Other judgments are also unchanged: a malformed JetStream input subject is covered by nothing, so it
may also raise `stream_requirement`. That is accepted, not suppressed.

## Stricter than NATS

The grammar refuses a token that mixes a wildcard with other text (`a*`, `b*.c`), which nats-server accepts as a
literal (#367 probe: `server.IsValidSubject` true). It also refuses form feed. This is accepted on purpose: one grammar
owns validity, cover and overlap, and a subject the cover and overlap rules cannot read would be judged wrongly by
`stream_requirement` and `reserved_subject_capture`. The suggestion text tells the author. No subject in the tree is
affected (inventory, "Measurement").

## Example configuration

`config/example_config.json` holds `entity.>` and `graph.mutation.>`, both valid.
`TestValidateExampleConfigHasNoErrorFindings` (spec "The example configuration carries no error finding") stays green
unchanged. It is the guard that the new type does not refuse the shipped example.

## Proving tests

The oracle is a list of verdicts written by hand. No test calls `ValidSubject`. Its sources: the #367 probe for
`a.foo bar`, `a.foo\fbar`, `a.>.b`, `a*`, `a.b*.c`, `a..b`, `a.>` and `a.*.c` (malformed where the probe shows
`server.IsValidSubject=false`, plus the two documented stricter points); the spec's grammar text (composition-validation,
"One subject grammar decides validity, cover and overlap") for the tab, line feed, carriage return, `""`, `>` and
`$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES.>` verdicts.

- T1 `TestAnalyzeReportsMalformedSubject` (through `composition.Validate`). Malformed: `a.foo bar`, `a.foo\fbar`,
  `a.>.b`, `a*`, `a.b*.c`, `a..b`, and subjects holding a tab, a line feed and a carriage return; also `""` at the
  stream site only, since the port codec already refuses an empty port subject. Valid: `a.>`, `a.*.c`, `>`,
  `$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES.>`. Six sites: NATS input; NATS output; NATS-request input; JetStream
  input `subjects[1]`; JetStream output `subjects[1]`; explicit stream `subjects[1]`.
  - Each malformed subject at each site yields exactly one `malformed_subject` error, with the expected component,
    port and message as literal text.
  - Each valid subject yields no `malformed_subject`. The assertion counts `malformed_subject` findings only, never an
    empty `Errors` list: `>` on an explicit stream or a JetStream output also raises `reserved_subject_capture`, and
    some cases may also carry `config_invalid` through `Validate`.
- T2 `TestAnalyzeReportsMalformedSubjectBeforeEarlyReturns`: an explicit stream `a..b` with no enabled component, and
  again beside two writers to one KV bucket. Each result carries the `malformed_subject` beside `empty_composition` or
  `connection_pattern_error`.
- T3 `TestAnalyzeMalformedFilterIsNotAReservedCapture`: an explicit stream `graph.mutation.>.x` yields one
  `malformed_subject` and no `reserved_subject_capture`.
- T4 `TestValidateFindingsVocabularyIsClosed` gains a `malformed` case and the constant in `allFindingTypes`; the count
  becomes fifteen.

Shown able to fail. T1–T3 fail on the base: the type does not exist. Recorded in `notes/2.1.md`. Each mutant below,
run with `task mutate:check`, must be detected; a survivor is reported as a survivor.

| Mutant | Change | Detected by |
| --- | --- | --- |
| M1 | the check's call removed | T1, T2 |
| M2 | the call moved after the empty-composition return | T2 |
| M3 | only `NATSSubjects()[0]` checked | T1 (`subjects[1]`) |
| M4 | the input lane skipped | T1 |
| M5 | explicit streams skipped | T1 |
| M6 | `TypeMalformedSubject` given warning in `severityOf` | T1 |
| M7 | the validity guard dropped from `flowgraph.SubjectsOverlap` (`internal/flowgraph/flowgraph.go:323-325`) | T3 |

Examples, not generated checks. The grammar's law already has `TestValidSubject` and `FuzzValidSubject`. This change
only maps each site to a finding. Its inputs are a finite cross product (six sites by the verdict list), which T1
covers exhaustively. Output order is the only order-dependent property, and it is held by `finalize` and
`TestValidateIsDeterministic`.

Not covered by a test here: boot refusal on this type in particular. That is the severity rule plus
`analyzeBootComposition`, proven by `TestComponentManagerRefusesBootOnErrorFinding`. `service/` is held by sister
draft #376.

## Issues

- #369: retired by this pull request (`Closes #369`), tasks 2.1–2.2.
- #16 stays open: it is reserved-subject overlap at composition and startup, a different fact. This change does not
  alter `reservedCaptureFindings`.
- #331 stays open: its work is graph-ingest's refusal text (`processor/graph-ingest/canonical_mutations.go`), outside
  `composition`.
- #363 stays open: it is about where flow-path reachability is computed (service versus composition), a placement
  question this change does not touch.

## Who it is for

`TypeMalformedSubject` is framework API under the composition-validation spec, the requirement this delta adds. A
product calls `Validate` and requires an empty `Errors` list. Proof: T1, which calls `Validate` as a product would.
