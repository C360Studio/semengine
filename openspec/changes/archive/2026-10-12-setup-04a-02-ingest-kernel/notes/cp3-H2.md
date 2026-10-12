# Checkpoint 3 H2 notes: graph-ingest refuses outputs it does not write

The finding is Codex's checkpoint 3 record, PR #93 comment 6099921473, H2, and its portable probe
`TestReviewOutputDeclarationMustMatchStorage`. The task is 4.13. The files are `processor/graph-ingest/component.go`
(`Config.Validate`), `config_strict_test.go` (the new test), `component_test.go` (one carried case) and the package
`README.md` (the `ports` row).

## Choices the finding does not spell out

1. **Where the check runs.** In `Config.Validate` (`component.go:283-295`), beside `canonicalMutationProvider`, the
   check that already reads the ports. `resolveConfig` calls `Validate`, so `DeclarePorts` and `CreateGraphIngest`
   refuse the same configuration, and so does `component.SafeUnmarshal` (`component/validation.go:167`), which calls
   `Validate` on any config that has it. The check resolves each output with `PortDefinition.Resolve`, the way
   `canonicalMutationProvider` resolves inputs, so a port written in Go as a pointer is read the same as one decoded
   from JSON.
2. **What is accepted.** An output whose resolved config is a `component.KVWritePort` with `Bucket` exactly
   `ENTITY_STATES` (`graph.BucketEntityStates`). The codec does not trim a bucket name, so `" ENTITY_STATES"` is
   refused: it is not the bucket written. Output names stay free. Two outputs both naming `ENTITY_STATES` are each
   true and are accepted.
3. **Which output kinds are refused.** The kinds a port may take as an output are those whose binding allows
   `DirectionOutput` (`component/port_codec.go:43-67`): `network`, `file`, `nats`, `nats-request`, `jetstream`,
   `kv-write`, `store-provide`. graph-ingest's writes outside tests are: `ENTITY_STATES` `Create` and `Update`
   (`entity_writes.go:109`, `:332`), its applied-sequence guard bucket `Put` (`keyed_ingest.go:423`) and the readiness
   status publisher (`readiness.go:331`, a key in `readiness.BucketGraphStatus`). It publishes on no subject or
   stream and writes no connection, file or store. So every kind other than `kv-write` is refused, and `kv-write` on
   any bucket other than `ENTITY_STATES`.
4. **The guard and status buckets.** graph-ingest writes both, but under fixed names configuration cannot change, and
   the pin's default declares only `ENTITY_STATES`. Accepting them as outputs would add a declarable surface no
   consumer reads, so they are refused like any other bucket. This follows the finding's "a KV output naming another
   bucket" and its "`ENTITY_STATES` stays the fixed authority".
5. **The error.** `errs.WrapInvalid(errs.ErrInvalidConfig, ...)`, the class the other refusals in `Validate` use,
   worded as an action so the `errs.Wrap` suffix reads: `Config.Validate: check output port "x" (graph-ingest writes
   only a kv-write output on ENTITY_STATES) failed: invalid configuration`.
6. **A carried test case changed.** `TestConfig_Validate_ValidConfig`, case "valid full config"
   (`component_test.go:366`), declared an `entity_events` `jetstream` output on stream `GRAPH`, subject
   `graph.entity.created`, which graph-ingest never publishes. The case now declares only `entity_states`; that
   output is one of the new test's refused rows.
7. **The README.** The `ports` row said only that at least one output is required; it now says every output must be
   a `kv-write` port on `ENTITY_STATES` and any other is refused when the component is built.

## Evidence

- `TestConfigRefusesOutputsGraphIngestDoesNotWrite` (`config_strict_test.go:58`): seven unwritten outputs, each in
  place of the `ENTITY_STATES` output and beside it (14 subtests). Each must be refused by `DeclarePorts` with an
  error classified `errs.ErrorInvalid` (read with `errors.As` on `*errs.ClassifiedError`) that names the port, by
  `CreateGraphIngest`, and by `Validate`; the default configuration must still declare.
- Fail before, on `d4772ef`: all 14 subtests failed at `config_strict_test.go:88`, "DeclarePorts accepted an output
  graph-ingest never writes" (for example `outputs [{Name:unwritten_output ... Config:{Bucket:OTHER ...}}]`).
- `go test -race -count=1 -cpu=1 ./processor/graph-ingest/` passes after the fix.

## Mutation records

Each run is `task mutate:check -- -pkg ./processor/graph-ingest -test TestConfigRefusesOutputsGraphIngestDoesNotWrite
-file processor/graph-ingest/component.go -mutant <copy> -expect <line>`, the copy kept outside the repository. Each
printed `verdict: detection: each of the 3 mutant runs failed the named test with an expected line, and the
after-run passed` and exited 0.

| Wrong change | Expected line | Verdict |
| --- | --- | --- |
| The output check never refuses (`false && (...)`) | `config_strict_test.go:88` | detection |
| Only a `kv-write` output is checked (`ok && kv.Bucket != ...`) | `config_strict_test.go:88` | detection |
| Only the first output is checked (`range c.Ports.Outputs[:1]`) | `config_strict_test.go:88` | detection |
| The refusal is unclassified (`fmt.Errorf` wrapping `errs.ErrInvalidConfig`) | `config_strict_test.go:92` | detection |
| The refusal does not name the port | `config_strict_test.go:95` | detection |
| The check refuses every output (`ok \|\| kv.Bucket != ...`) | `config_strict_test.go:112` | detection |

No survivors, no inconclusive runs. No integration test was added, so no by-hand integration mutation was needed.

## Not covered

- No broker run: the refusal happens before any connection, as the finding's record notes.
- An `interface` contract on the `ENTITY_STATES` output is still accepted. No interface contract for entity-state
  writes exists to compare it with (no constant in `graph`), and refusing one would set a contract the design does
  not state. Candidate follow-up: decide whether graph-ingest's output declares an interface, and refuse any other.
- Inputs are not checked here beyond the existing mutation-provider check; H2 is about outputs.
