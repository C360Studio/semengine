# Design: fix-331-mutation-port-refusal

Base: main 2dab437 (claim commit bdd25af). Rests on `inventory.md`. Issue #331, claim draft PR #397.

## Options

1. **Do nothing.** Rejected: the owner kept the texts on #323 (Q2) and filed this one as a defect to fix.
2. **Name the port in the malformed refusal.** Chosen. One format string in `requireMutationProviderPort`
   (`processor/graph-ingest/canonical_mutations.go:125-128`); both callers get it.
3. **Use flowgraph's wording.** Rejected: Q2 and the spec (`graph-transport-boundary` `:217`, "Each caller keeps its
   own refusal texts") keep two texts, and graph-ingest's `Config` has no instance name to put in the component slot
   (inventory § 1).
4. **Also name ports in the ambiguous refusal, or check validity before ambiguity.** Not here (Non-goals).

## Decisions

- **D1, the text.** The format becomes `"graph mutation provider port %q must be a required %s %s nats-request input
  on %s"`, with `definition.Name` first, then `graphmutation.InterfaceType`, `InterfaceVersion` and `SubjectFamily` as
  today. `%q` and `definition.Name` follow the output-port refusal in the same `Validate` (`component.go:311-312`).
  For an input named `mutations` the refusal reads `graph mutation provider port "mutations" must be a required
  semengine.graph.mutation v1 nats-request input on graph.mutation.>`.
- **D2, the spec delta is corrected.** As drafted it says graph-ingest's refusal of any member input that is not
  valid names the port. That is false when the input follows another graph-mutation input: it is refused as
  ambiguous, naming no port, which the delta's own JetStream scenario pins (inventory § 1, § 3). The corrected
  requirement says the refusal names the port when the input is graph-ingest's first graph-mutation input, and that a
  later one is refused as ambiguous. Full text below.
- **D3, the ledger.** The `processor/graph-ingest` row's `contract` item (8) (`docs/admission-ledger.yaml:2341-2345`)
  gains, before `(one-mutation-port-check, #323)`: `; a first graph-mutation input that is not valid is refused with
  an error naming the port (#331)`. The pin's text has no port name (pin `canonical_mutations.go:185-188`), and item
  (5) (`:2336`) is the precedent for recording a refusal-text change. Done with the code in task 2.2.
- **D4, no README change.** `processor/graph-ingest/README.md:80` says any other graph-mutation input "is refused".
  Most such inputs reach the ambiguous refusal, so adding "with an error naming the port" there would be false.
- **D5, proposal corrections.** What Changes: "names the port when that input is graph-ingest's first graph-mutation
  input". Why: flowgraph's refusal is at `internal/flowgraph/flowgraph.go:213-215`. Impact: add
  `docs/admission-ledger.yaml`.

## Observable behaviour

Only the text of one refusal changes. What is refused, its class and where are unchanged: `Config.Validate` (in the
factory, `component.go:614`, and `Initialize`, `:941`) returns an invalid-class error reading `Config.Validate: graph
mutation provider port "mutations" must be ... on graph.mutation.> failed: invalid configuration`. Building the
mutation routes at Start returns the plain error, wrapped as `resolve mutation provider routes: ...`
(`mutation_runtime.go:20`). The `port is ambiguous` and `port is required` texts do not change.

## Proving tests

- **`TestConfigValidateMutationProviderRefusals`** (`mutation_port_contract_test.go`). Replace `const malformed`
  (`:37-38`) with two hand-written literals, never built from `graphmutation`'s constants or the port definitions:

  ```go
  const (
      malformedMutations = `graph mutation provider port "mutations" must be a required semengine.graph.mutation v1 ` +
          `nats-request input on graph.mutation.>`
      malformedLegacy = `graph mutation provider port "legacy" must be a required semengine.graph.mutation v1 ` +
          `nats-request input on graph.mutation.>`
  )
  ```

  `wrong version` and `not required` want `malformedMutations`; the two `untyped` cases want `malformedLegacy`.
  On the base all four fail at the `Fatalf` (`:99` today), for example `Validate() = Config.Validate: graph mutation
  provider must be a required ... failed: invalid configuration, want an invalid-class refusal containing "graph
  mutation provider port \"mutations\" must be ..."`. Recorded verbatim in `notes/2.1.md`.
- No route-path test is added: both callers share the one function, and
  `TestCanonicalMutationRoutesRejectLegacyCompanionPort` reaches the ambiguous branch (inventory § 1).
- **Mutation runs,** each `task mutate:check -- -pkg ./processor/graph-ingest -test
  TestConfigValidateMutationProviderRefusals -file processor/graph-ingest/canonical_mutations.go -mutant <copy>
  -expect mutation_port_contract_test.go:<the Fatalf line>`, each reporting detection:
  - M1: the base text restored (no `port %q`, no name argument).
  - M2: `ports.Inputs[0].Name` in place of `definition.Name`, so the refusal names `entity_stream`, the first input
    of every case: naming the wrong port.
  - M3: `%s` in place of `%q`, so the name is unquoted.
- Examples are enough: one format string, and the table already covers the four malformed shapes.

## Non-goals

- **The ambiguous refusal** (`canonical_mutations.go:121-122`) names neither port, and because it is checked first, a
  stray graph-mutation input after a valid provider gets it instead of the named refusal. Same operator cost as #331,
  LOW per #185, but Q2 kept that text and #331 does not ask for it. Recommended: file it as its own issue (below);
  this change does not touch it.
- Flowgraph's text (`%s`, unchanged, Q2); the `port is required` texts (no port to name); the unclassified error on
  the route path.

## Docs and spec sync

Task 4.1 syncs the corrected delta. D3's ledger line rides task 2.2. No README or `doc.go` change (D4).

## Merge order

No open PR shares a code file. #393 edits other rows of `docs/admission-ledger.yaml`; whichever lands second merges
`origin/main`. Draft #398 (#305) will rewrite this requirement, which is over 500 characters. If #398 lands first,
this delta is re-copied from the requirement #398 leaves before archive, since a MODIFIED delta replaces the whole
requirement and a stale copy would undo #398's split. If this PR lands first, #398 splits the corrected text. If the
openspec bump to 1.14.1 lands before this archives, the delta must also be under 500 characters.

## Corrected spec delta

`openspec/changes/fix-331-mutation-port-refusal/specs/graph-transport-boundary/spec.md`, in full:

```markdown
# Delta: graph-transport-boundary

## MODIFIED Requirements

### Requirement: One decision on graph-mutation ports

Whether a component port is a graph-mutation port, and whether it has the valid shape, SHALL be decided in
`internal/graphmutation` (`ClassifyPort`), and `internal/flowgraph` and graph-ingest SHALL both use that decision. A
port SHALL be a member when its interface type is `semengine.graph.mutation`, or when any one of its NATS subjects
starts with `graph.mutation.`. A member SHALL be valid only when it is a required `nats-request` port whose subjects
are exactly `graph.mutation.>` and whose interface is `semengine.graph.mutation` `v1`. Flowgraph SHALL refuse a
member that is not valid, on an input or an output, with an error wrapping `errs.ErrInvalidConfig` that names the
component and the port. graph-ingest SHALL refuse a member input that is not valid, in `Config.Validate` and when it
builds its mutation routes; when that input is its first member input, the refusal SHALL name the port, and a member
input after the first is refused as ambiguous. Each caller keeps its own refusal texts, direction rule and provider
count.

#### Scenario: A nats port on a graph.mutation subject

- **WHEN** a flow declares a `nats` output on `graph.mutation.entity.create` beside one valid provider
- **THEN** flowgraph refuses it with `component requester port legacy is not a required nats-request
  semengine.graph.mutation v1 port on graph.mutation.>`

#### Scenario: A JetStream input with one graph.mutation subject

- **WHEN** graph-ingest's configuration declares the valid provider and a JetStream input on stream `EVENTS` whose
  subjects are `events.>` and `graph.mutation.entity.create`
- **THEN** `Config.Validate` refuses it with an invalid-class error containing `graph mutation provider port is
  ambiguous`

#### Scenario: A graph-ingest input with the wrong interface version

- **WHEN** graph-ingest's configuration declares a JetStream input `entity_stream` on `entity.>` and a required
  `nats-request` input `mutations` on `graph.mutation.>` with interface `semengine.graph.mutation` `v2`
- **THEN** `Config.Validate` refuses it with an invalid-class error containing `graph mutation provider port
  "mutations" must be a required semengine.graph.mutation v1 nats-request input on graph.mutation.>`

#### Scenario: The valid provider

- **WHEN** a port is a required `nats-request` port on `graph.mutation.>` with interface `semengine.graph.mutation`
  `v1`
- **THEN** `ClassifyPort` reports it a member and valid, and both callers accept it
```

## Follow-up to file (recommended)

Title: `graph-ingest: the ambiguous graph-mutation port refusal names no port`. Body: `canonical_mutations.go:121-122`
refuses a second graph-mutation input with `graph mutation provider port is ambiguous`, naming neither input; because
the check runs before the validity check, a stray input (for example a JetStream input with one `graph.mutation.`
subject, the `graph-transport-boundary` scenario) after a valid provider gets this text, not the named one #331
adds. LOW per #185. The owner kept the text on #323 (Q2), so whether to change it is his call.
