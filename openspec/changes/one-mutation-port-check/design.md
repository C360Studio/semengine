# Design: one-mutation-port-check

Accepted by the owner on 2026-10-19 (#323 comment 6279624359): Q1 yes, Q2 both texts unchanged. Line numbers in
"What exists" are at base `e6b61da`.

## Terms

- **Graph-mutation port:** a component port on the graph-mutation request family, `graph.mutation.>`, declared in
  `internal/graphmutation/protocol.go:10-17` with its interface type `semengine.graph.mutation` and version `v1`.
- **Prefix member:** a port whose interface type is `semengine.graph.mutation`, or any one of whose NATS subjects
  starts with `graph.mutation.` (ruled Q1).
- **Valid:** a member that is a required `nats-request` port whose subjects are exactly `[graph.mutation.>]` and whose
  interface type and version are `semengine.graph.mutation` and `v1`.

## What exists

- **Flowgraph's copy.** `internal/flowgraph/flowgraph.go:203-239` (`validateGraphMutationProviders`, called at
  `:177`). Membership `:213-214`: request pattern and subject starts with `graph.mutation.`, or interface type
  matches; it checks inputs and outputs. Shape `:219-221`: request pattern, `ConnectionID == SubjectFamily`, interface
  present with type and version, `Required`. Refusal `:222` "component %s port %s is not a required nats-request %s
  %s port on %s: %w" (wraps `errs.ErrInvalidConfig`). Count `:234-236` "graph mutation flow requires exactly one
  compatible provider, found %d". It reads `portInfo` (`:28-38`); `portInfo.Interface` (`:34`, built `:153-155`) has
  no other reader.
- **Graph-ingest's copy.** `processor/graph-ingest/canonical_mutations.go:104-142` (`requireMutationProviderPort`),
  called at `component.go:315` (`Config.Validate`, wrapped `errs.WrapInvalid`) and `canonical_mutations.go:85` (route
  build at `Start`). Inputs only; outputs never reach it (`component.go:305-313`). Membership `:120-121`: exactly one
  NATS subject starting with the family prefix (any kind), or interface type matches. Shape `:128-130`: kind
  `nats-request`, `Required`, subjects equal to `[graph.mutation.>]`, type, version. Refusals: "graph mutation
  provider port is required" (`:106`, `:139`), "graph mutation provider port is ambiguous" (`:126`), "graph mutation
  provider must be a required %s %s nats-request input on %s" (`:131-134`).
- **The shapes are equivalent.** Only `nats-request` facts carry `PatternRequest` (`component/port_facts.go:191-196`),
  and there `ConnectionIDs[0] == NATSSubjects[0] == Subject`.
- **A third spelling** of the valid port: `processor/graph-ingest/component.go:350` (`DefaultConfig`).
- **Tests.** Flowgraph `graph_mutation_contract_test.go:11,25-41,51`; `declarations_test.go:252-263`, `:370-387`.
  Graph-ingest `component_test.go:343`, `:427-433`; `canonical_mutations_test.go:48-56`;
  `mutation_port_contract_test.go:10-28`. No graph-ingest test pins the malformed or ambiguous text.
- **ADRs.** Pin ADR-091 §2 (`docs/adr/091-...md:49-58` at `8b99efe9`): one typed provider input port; `:58` "No
  JetStream mutation stream ... is added". Kept by `docs/adr/110-write-seam.md:68-69`. This change keeps it.
- **Consumers.** None is affected by Q1: semsource `cmd/semsource/run.go:755-763`, semconnect
  `gateway/cs-api/component.go:266` and SemStreams `deploy/semstreams.json:35-41` all declare `nats-request` on
  `graph.mutation.>`. The one existing test declaration newly refused is
  `internal/flowgraph/flowgraph_validation_test.go:468-501` (`TestJetStreamSubjectMatching`, subtest "no duplicate
  edges..."), which uses `graph.mutation.write` only as an example subject.
- **Open PRs.** #330 (this) and #329 (config, no overlap). #16 keeps wildcard stream overlap (`graph.>`, `>`).

## Behaviour

- Q1 (ruled yes): a port is a member when it is a prefix member. Flowgraph newly refuses a `nats` or JetStream port
  that is a prefix member; graph-ingest newly refuses a multi-subject JetStream input with one prefix-member subject.
- Q2 (ruled): both refusal texts are unchanged. Direction, provider count and refusals stay with their callers.

## The function

`internal/graphmutation/port.go`:

```go
func ClassifyPort(facts component.PortFacts, required bool) (member, valid bool)
```

`valid` is member, kind `nats-request`, subjects exactly `[SubjectFamily]`, interface type and version matching, and
required. Valid implies member by construction, not by a test.

- Importing `component` closes no cycle (`internal/harness/contract/componentdeps_test.go:66,112`). Cost:
  `pkg/lifecycle` and `pkg/projection` newly reach `component`, `internal/cache`, `model`, `storage`,
  `storage/storeregistry` and `types`.
- Rejected: plain values as arguments (the shape leaks into callers); a subpackage (one more package).
- Flowgraph's `portInfo` gains `facts` and drops `Interface`, `kind` and `natsSubjects`; their readers move to
  `facts.Kind()` and `facts.NATSSubjects()`.

### Who should adopt it

`ClassifyPort` is the only membership-and-shape decision for graph-mutation ports. Its two callers are flowgraph and
graph-ingest. The third spelling, `DefaultConfig` (`processor/graph-ingest/component.go:350`), constructs the valid
port rather than deciding on one, and `mutation_port_contract_test.go` checks it; it stays.

## Ruling to implementation

Line numbers in this table are at the commit that lands task 4.1.

| Ruling (#323 comment 6279624359) | Where it is implemented | Where it is proved |
| --- | --- | --- |
| Q1: membership is the interface type, or any NATS subject on `graph.mutation.` | `internal/graphmutation/port.go:28-35` | `internal/graphmutation/port_test.go`, `TestClassifyPort` |
| Q1: graph-ingest refuses a prefix member that is not valid | `processor/graph-ingest/canonical_mutations.go:115-124` | `processor/graph-ingest/mutation_port_contract_test.go`, `TestConfigValidateMutationProviderRefusals`, row "jetstream input with one graph.mutation subject" |
| Q1: flowgraph refuses a prefix member that is not valid | `internal/flowgraph/flowgraph.go:205-216` | `internal/flowgraph/graph_mutation_contract_test.go`, `TestGraphMutationFlowRejectsInvalidProviderTopology`, rows "nats output on a graph.mutation subject" and "jetstream output with one graph.mutation subject" |
| Q2: graph-ingest's texts unchanged | `processor/graph-ingest/canonical_mutations.go:106,119,124,129` | `TestConfigValidateMutationProviderRefusals`, literal texts |
| Q2: flowgraph's texts unchanged | `internal/flowgraph/flowgraph.go:214,230` | `declarations_test.go`, `TestRefusalsNameNoInternalFunction`, literal texts |
| Q2: graph-ingest's text naming no port is out of scope | not changed | filed as #331, which stays open |

## Tasks

| Task | Work and proving test | Oracle | Mutant |
| --- | --- | --- | --- |
| 1.1 | `TestConfigValidateMutationProviderRefusals` (graph-ingest) via `Config.Validate`, rows: no ports, no candidate, two candidates, wrong version, not required, `graph.mutation.*`, untyped `graph.mutation.entity.create`; passes on main. Add a multi-subject JetStream row, failing first. | Literal texts on main | `canonical_mutations.go:130` version comparison becomes `false`; the wrong-version row fails |
| 1.2 | Move the subjects in `flowgraph_validation_test.go:468-501` to `events.write` (green before and after). Add a failing row to `graph_mutation_contract_test.go`: a `nats` output on `graph.mutation.entity.create` is refused. | Literal texts | `flowgraph.go:221` drops `\|\| !port.Required`; the "not required" row fails |
| 2.1 | `internal/graphmutation/port_test.go` `TestClassifyPort` (red: does not compile), rows built from real `component.Port.Facts()` | Literal `"graph.mutation.>"`, `"semengine.graph.mutation"`, `"v1"`, never the constants | (a) drop `!required \|\|`: the optional row fails; (b) the prefix test becomes `s == SubjectFamily`: the legacy row fails |
| 2.2 | Implement `ClassifyPort`; the 80% coverage floor holds (`internal/harness/contract/cover_test.go:50`) | | |
| 3.1 | Graph-ingest calls `ClassifyPort`; 1.1 green | | `if !valid {` becomes `if false && !valid {`; 1.1's wrong-version row fails |
| 3.2 | Flowgraph calls it; `portInfo` carries `facts`, loses `Interface`, `kind`, `natsSubjects`; 1.2 and the existing tests green | | the same mutant; the `graph_mutation_contract_test.go:34` row fails |
| 4.1 | Docs, ledger, spec delta (archive comes later) | | |

### Generated checks: not used

The rule is a predicate over one port's facts and one flag: member when the interface type matches or any subject
has the prefix; valid when member and five shape conditions all hold. It has no transformation law and no history
(each port is classified on its own, and each caller's count is a plain tally). The input does interact, so the
example tests cover it this way:

- **Membership.** Each way in is a row of `TestClassifyPort`: the interface type alone (an untyped subject outside the
  family), the prefix alone (an untyped `graph.mutation.entity.create`), the prefix as the second subject of a
  JetStream port, and neither (an unrelated port, not a member). A wrong interface type with a family subject is a
  member through the prefix.
- **Shape.** Starting from the valid port, each row breaks exactly one of the five shape conditions (kind, subjects,
  type, version, required), so each condition is shown to be checked on its own. Kind is broken twice, by a `nats`
  port and by a JetStream port, since both are now members.
- **Callers.** Each caller's tests add the cases only the caller decides: direction, count (none, two), and the
  refusal texts. Order does not matter: both callers visit ports in a fixed order (flowgraph by component name,
  graph-ingest in declaration order), and `TestFlowGraphOutputIsTheSameEveryRun` already pins flowgraph's.

No input format with size or count limits is involved: subjects are not parsed here, only compared.

## Spec delta

ADDED to `graph-transport-boundary`: "One decision on graph-mutation ports". A graph-mutation port's membership and
shape are decided in `internal/graphmutation`, and flowgraph and graph-ingest both use that decision. Scenario: a
`nats` or JetStream port that is a prefix member is refused by both callers.

## Docs and ledger

- `docs/admission-ledger.yaml`:
  - `internal/graphmutation` row: `ClassifyPort` in the contract, `component` in the dependencies, the new test in
    `proving_tests`.
  - `component/flowgraph` row: its adaptation item (1) becomes "membership and shape come from
    `graphmutation.ClassifyPort`"; the #323 `known_risks` entry is deleted; the membership statement is widened.
  - `processor/graph-ingest` row: the contract gets the widened membership.
- `docs/repository-map.md`, the `internal/graphmutation/` row; `internal/graphmutation/protocol.go` package comment;
  `internal/flowgraph/README.md` and `doc.go`; `processor/graph-ingest/README.md` (membership, and the port's name,
  `graph_mutations` corrected to `mutations`).

## Merge order

- #329 (config) touches none of these files. Whichever lands second merges `origin/main` and re-checks
  `docs/admission-ledger.yaml` and `docs/repository-map.md`.

## Not in scope

- #331, graph-ingest's refusal text not naming the port.
- `DefaultConfig`'s construction of the valid port (a constructor, not a decision).
