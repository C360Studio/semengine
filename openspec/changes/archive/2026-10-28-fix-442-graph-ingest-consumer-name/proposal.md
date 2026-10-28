# Proposal: fix-442-graph-ingest-consumer-name

## Why

Issue #442. graph-ingest names the durable consumer of each JetStream input itself, `graph-ingest-<subject>` with
the subject's `.`, `*` and `>` replaced (`processor/graph-ingest/consume.go:95-98`), and never reads the input's
declared `consumer_name` (`component.JetStreamPort.ConsumerName`, `component/port_jetstream.go:20`). A configuration
that sets it is accepted and the value is dropped without a word. That breaks "Failure paths fail closed; any skip,
drop, or degrade is declared" (AGENTS.md § 1). The SemStreams pin `8b99efe9` drops it the same way
(`processor/graph-ingest/component.go:1525` there names the consumer the same way).

Who sets it today: nothing. No configuration, test or document in this repository declares `consumer_name` for
graph-ingest, and none of the starter consumers (semsource, semconnect, semteams, semboids, at their local checkouts)
does. Refusing it therefore breaks no one.

The precedent is PR #437 (change 4 PR 5b, open), which refuses a declared `consumer_name` on the objectstore
Component at construction, naming the port. Honouring the value instead would change the consumer that graph-ingest's
readiness, backlog and lost-binding checks look up by name, for a setting nobody uses; refusing is the smaller
change.

Light process, owner 2026-10-28: proposal, tasks and spec delta; no design document; Codex reviews the code.

## What Changes

- `Config.Validate` (`processor/graph-ingest/config.go:27`) resolves each input port and refuses a JetStream input
  that declares `consumer_name`, as invalid configuration naming the port and `consumer_name`. `Validate` is the one
  check that `CreateGraphIngest` and `DeclarePorts` both run, so both refuse it, as they already refuse an output
  graph-ingest does not write.
- A test in `processor/graph-ingest/config_strict_test.go` shows `DeclarePorts`, `CreateGraphIngest` and `Validate`
  each refuse such an input, naming the port and the key, and that the default configuration still builds.
- Nothing else changes: the consumer name graph-ingest gives an input stays as it is.

## Capabilities

### Modified Capabilities

- `graph-ingest-recovery`: one added requirement, "A declared consumer name is refused".
