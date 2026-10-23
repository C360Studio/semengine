# Proposal: fix-369-subject-grammar

## Why

Component port subjects and explicit stream subjects are checked only for emptiness, never against the subject grammar
(#369). The port codec's checks are `strings.TrimSpace(...) == ""` (`component/port_codec.go:322`, `:333`, `:356-358`),
and nothing reads `config.StreamConfig.Subjects` (`config/streams.go:28`) against a grammar. A subject NATS cannot
serve is therefore accepted, and it fails later, far from the config line. The #367 probe (comment 6343878154, nats.go
v1.54.0, nats-server v2.15.1) shows `a.>.b` subscribes without an error and then the server closes the connection;
form feed is accepted by the client and refused by the server; whitespace and empty tokens fail only at subscribe.

The owner ruled on #369 (2026-10-23): "Composition check, new finding type". `composition.Analyze` is the one
interpreter that judges a composition offline and at boot (ADR-100). It already reads every port's facts and the
explicit `streams` block through `internal/flowgraph` (`composition/analyze.go:26`, `:134`). The grammar stays in
`internal/flowgraph` (composition-validation spec, "One subject grammar decides validity, cover and overlap").
`component` and the carry file `config/stream_bounds.go` are not touched.

## What Changes

- `composition` gains one finding type, `malformed_subject` (error). `Analyze` reports one for each NATS, NATS-request
  or JetStream port subject, in either lane, and each explicit stream subject, that fails `flowgraph.ValidSubject`. It
  runs before `Analyze`'s early returns.
- The closed vocabulary grows from fourteen to fifteen types.
- Boot refuses such a composition through the existing error-severity rule (`service/component_manager.go:349`). No
  `service/` file changes.

## Capabilities

### Modified Capabilities

- `composition-validation`: the closed vocabulary gains `malformed_subject`; a new requirement states what is checked,
  what each finding names, and where the check runs.

## Impact

- Code: `composition/analyze.go`, `composition/findings.go`, `composition/doc.go`; tests under `composition/`.
- Ledger: the `composition` row's `contract` gains one clause.
- Behaviour: a configuration holding a subject that fails the grammar now fails `composition.Validate` and boot. This
  includes a token such as `a*`, which NATS reads as a literal; no subject in the tree is affected (inventory,
  "Measurement").
- Issues: `Closes #369`. #16, #331 and #363 stay open (design, "Issues").
