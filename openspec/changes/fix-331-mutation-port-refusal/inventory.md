# Inventory: fix-331-mutation-port-refusal

Base: main 2dab437 (claim commit bdd25af). Issue #331, claim draft PR #397. Not a porting change: graph-ingest is
already ported (ledger row `repair-before-port`), so no lifecycle-suite or test-repeat run on a pin copy applies. Every
pin fact below is read with `gh api 'repos/C360Studio/semstreams/contents/<path>?ref=8b99efe9'`.

## 1. The claimed gap

The refusal and the loop it sits in, `processor/graph-ingest/canonical_mutations.go`:

- `:103` — `func requireMutationProviderPort(ports *component.PortConfig) error {`
- `:108` — `for _, definition := range ports.Inputs {` (the port's name is in scope as `definition.Name`)
- `:117` — `member, valid := graphmutation.ClassifyPort(facts, port.Required)`
- `:121-122` — `if found {` / `return errors.New("graph mutation provider port is ambiguous")`
- `:124-128` — `if !valid {` / `return fmt.Errorf("graph mutation provider must be a required %s %s nats-request input
  on %s", graphmutation.InterfaceType, graphmutation.InterfaceVersion, graphmutation.SubjectFamily)`. No port name.
  The issue's `:131-134` (at e6b61da) is stale.
- `:130` — `found = true`
- `:105`, `:133` — `graph mutation provider port is required` (no member found, so there is no port to name).

The order matters: the ambiguous check (`:121`) runs before the validity check (`:124`). So the malformed refusal fires
only when the input that is not valid is the **first** graph-mutation input. A later graph-mutation input, valid or not,
is refused as ambiguous, and that refusal names no port. The pin has the same order (§ 2).

A port name is never empty here: `component/port_resolver.go:17-18` — `if strings.TrimSpace(def.Name) == "" {` refuses
a blank name inside `Resolve`, which `:109` calls before the classification.

Callers (`git grep -n requireMutationProviderPort`: the definition and two calls):

- `processor/graph-ingest/component.go:315-316` — `if err := requireMutationProviderPort(c.Ports); err != nil {` /
  `return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", err.Error())`. `Config.Validate` runs in the
  factory (`:614`) and in `Initialize` (`:941`).
- `processor/graph-ingest/canonical_mutations.go:84`, inside `canonicalMutationRoutes`, which Start reaches through
  `mutation_runtime.go:18-20` (`return fmt.Errorf("resolve mutation provider routes: %w", err)`). This path returns
  the plain error, with no class.

What an operator reads from `Config.Validate` today, from `pkg/errs/errs.go:364` (`"%s.%s: %s failed: %w"`),
`:386-392` (`WrapInvalid`) and `:103-105` (`ClassifiedError.Error` returns its message): `Config.Validate: graph
mutation provider must be a required semengine.graph.mutation v1 nats-request input on graph.mutation.> failed:
invalid configuration`.

graph-ingest's `Config` (`component.go:279-289`: `Ports`, `EnableHierarchy`, `IngestLanes`) holds no instance name, so
this refusal can name the port but not the component.

"No graph-ingest test pins it today" is stale. `processor/graph-ingest/mutation_port_contract_test.go:37-38` —
`const malformed = "graph mutation provider must be a required semengine.graph.mutation v1 nats-request " +` — is
asserted by four cases, `:75-81`: `wrong version` and `not required` (port `mutations`), and `untyped
graph.mutation.*` and `untyped graph.mutation.entity.create` (port `legacy`). The check is `:98` — `if err == nil ||
!errs.IsInvalid(err) || !strings.Contains(err.Error(), tt.want) {`. The old text is not a substring of the wanted new
one (`provider must` becomes `provider port "<name>" must`), so these four cases fail after the fix until their wanted
text changes. In every case the first input is `entity_stream` (`configWith(entityStream, ...)`, `:39-41`), so a
refusal that named the first input instead of the right one would be told apart. In all four cases the malformed
input is also the last input, so a refusal naming the last input would not be (design adds a case).

`processor/graph-ingest/canonical_mutations_test.go:48-55` (`TestCanonicalMutationRoutesRejectLegacyCompanionPort`)
appends `legacy_mutations` after the default provider, so it reaches the ambiguous branch, and checks only
`err == nil`. It does not read the text.

## 2. Other spellings and the pin

- Readers of the old text: `git grep -n -E "must be a required|nats-request input on|provider must be" --
  ':!openspec/changes/archive'` gives the code (`canonical_mutations.go:126`), the test
  (`mutation_port_contract_test.go:37`), this change's `proposal.md:6-7` and delta `:36`, and
  `docs/admission-ledger.yaml:2344`, which describes the accepted port shape, not the refusal. History only:
  `openspec/changes/archive/2026-10-19-one-mutation-port-check/design.md:30`. The live spec pins no
  graph-ingest malformed text (§ 3).
- Nothing parses the text: `git grep -n -E 'strings\.(Contains|HasPrefix)\([a-zA-Z.]*(err|Err)[a-zA-Z.]*\.Error\(\)'
  -- '*.go' ':!*_test.go'` gives `internal/harness/natsfixture/fixture.go:310` (address in use) and the doc example
  `pkg/errs/doc.go:115`.
- Package docs: `processor/graph-ingest/README.md:80` says any other graph-mutation input "is refused", with no text.
  `doc.go`: no hit.
- Flowgraph's spelling of the same refusal: `internal/flowgraph/flowgraph.go:213-215` — `"component %s port %s is not
  a required nats-request %s %s port on %s: %w"` (the issue's `:222` is stale). It names the component and the port,
  with `%s`. Pinned by `internal/flowgraph/graph_mutation_contract_test.go:77` and `declarations_test.go:261,374`.
- Pin probe (`processor/graph-ingest/canonical_mutations.go?ref=8b99efe9`): the function is `canonicalMutationProvider`
  (`:158`). It has the same text at `:185-188` (`"graph mutation provider must be a required %s %s nats-request input
  on %s"`), the ambiguous check first at `:179-180`, and callers `:129` (`canonicalMutationRoutes`) and
  `component.go:365` (`Config.Validate`). The base text is carried from the pin; after the fix it differs from the pin.

## 3. Adjacent claims

- Spec: `openspec/specs/graph-transport-boundary/spec.md:208` — `### Requirement: One decision on graph-mutation
  ports`. `:215-217` — "graph-ingest SHALL refuse a member input that is not valid, in `Config.Validate` and when it
  builds its mutation routes. Each caller keeps its own refusal texts, direction rule and provider count." Scenarios at
  `:219` (flowgraph's text), `:225` (graph-ingest, JetStream input, `ambiguous`) and `:232` (the valid provider).
- The drafted delta against the live text: the requirement is the live text word for word plus ", with an error that
  names the port", and one scenario is added (wrong interface version). The added clause is false for a shape the live
  spec already pins: in the JetStream scenario (`:225-230`, kept in the delta), the `events` input is a graph-mutation
  input that is not valid, and graph-ingest refuses it as ambiguous, naming no port, because it follows the valid
  provider (§ 1). As drafted, the requirement contradicts its own scenario.
- Owner ruling: #323 comment 6279624359 (2026-10-19), Q2: "the refusal texts: both stay as they are. Graph-ingest's
  text not naming the port is filed as #331." The ambiguous text and flowgraph's text stay under that ruling.
- Ledger: `docs/admission-ledger.yaml:2321` — `- source_path: processor/graph-ingest`, `source_sha` the pin,
  `:2374` — `disposition: repair-before-port`. Its `contract` lists changes from the pin: `:2336` — `(5) hierarchy
  refusal names inference.RegisterPayloads (#33)` (a refusal-text change recorded there) and `:2341-2345` — `(8) the
  mutation provider input is decided by graphmutation.ClassifyPort: ... (one-mutation-port-check, #323)`. `:2368`
  already names `mutation_port_contract_test.go (TestConfigValidateMutationProviderRefusals)`. The separate
  `processor/graph-ingest/component.go` row (`:2376`, `adapt`) is not touched: the fix is in `canonical_mutations.go`.
- Open PRs (`gh pr list --state open --json number,title,changedFiles,files`):
  - #397, this claim: four files under `openspec/changes/fix-331-mutation-port-refusal/`.
  - Draft #393, the log forwarder: 22 files at ba27e15. No file under `processor/graph-ingest/` or a `graph-transport-boundary`
    delta. It shares `docs/admission-ledger.yaml` (its own rows).
  - Draft #398, the spec split for #305: no files yet (head 52f565e, one empty commit ahead of 2dab437,
    `gh api repos/C360Studio/semengine/compare/2dab437...52f565e`). Its body: split every requirement over 500
    characters, never rename a `### Requirement:` header. "One decision on graph-mutation ports" is 903 characters
    (`sed -n 210,217p openspec/specs/graph-transport-boundary/spec.md | tr '\n' ' ' | wc -c`), so #398 will
    rewrite the requirement this change modifies. The tree pins `@fission-ai/openspec` 1.14.0 (`package.json:7`),
    which does not fail on length, so the delta passes `task spec:check` today.
- Open issues: `gh issue list --state open --search <q>` for `requireMutationProviderPort`, `canonical_mutations`,
  `mutation provider`, `ClassifyPort`, `graph-ingest refusal`. Kept: #331. Names the word, not the function: #26 #7 #9
  #33 #300 #196 #111 #109 #103 (#300 cites `canonical_mutations.go:46-53`, the reply re-decode, not the refusal).

## 4. Who it is for

Nothing new: no exported symbol, port, subject, bucket or config field. One error string changes.

## 5. Problem shape

A refusal at a configuration seam that names the element it refuses. Closest instance, in the same `Validate`:
`processor/graph-ingest/component.go:311-312` — `"check output port %q (graph-ingest writes only a kv-write output on
%s)", definition.Name, graph.BucketEntityStates))`, which quotes `definition.Name` with `%q`. Flowgraph's
(`flowgraph.go:213`) uses `%s`. The `AGENTS.md` rule "refuse unknown config keys ..., naming the key" is the same shape.

## 6. Pin decision records and adopters

- The change carries no package from the pin. The shape the refusal enforces is pin ADR-091 §2, kept by ADR-110
  (#323 ruling Q1). Neither records a refusal text. Pin
  `docs/adr/091-graph-mutation-authority-without-semantic-ownership.md`, grepped for
  `refus|must be a required|error|ambiguous`, has one hit, `:12`, the RPC error contract.
  `git grep -n -i "mutation provider\|provider port" -- docs/adr` gives 0.
- Consumers. Question: does a starter consumer match on this text? Read-only grep of local checkouts, no `git`
  command, over `*.go/md/json/yaml/sh` for `graph mutation provider|mutation provider (port )?must`: semsource 0,
  semteams 0; semconnect 1 (`openspec/changes/migrate-semstreams-beta160/tasks.md:69`, prose) and semboids 1
  (`configs/flock.json:175`, a port description). None reads the refusal.
- Adopter seam (a config author or operator with several inputs). (1) What must they know? Which of their inputs is
  the graph-mutation one, and the one valid shape. (2) If they do nothing: the component is refused at construction
  with a message naming no port; they compare each input against the shape. (3) Where do they find out? A boot error
  (factory `Validate`, `component.go:614`): the right place, with the wrong content. (4) What should they have to
  know? Nothing beyond the message: it names the port. Gap left after #331: the ambiguous refusal (`:122`) names
  neither port, including the case of a stray input after a valid provider (§ 1).

## 7. Proving test and sensitivity

- On the base, the four malformed cases pass with the old constant (§ 1). With the port name in the wanted text,
  each fails on the base.
- The independent oracle is a hand-written literal per port (`mutations`, `legacy`), never built from
  `graphmutation`'s constants or from `definition.Name`, as the test's doc comment (`:32-35`) already requires.
- Mutation run: `task mutate:check -- -pkg ./processor/graph-ingest -test TestConfigValidateMutationProviderRefusals
  -file processor/graph-ingest/canonical_mutations.go -mutant <copy> -expect mutation_port_contract_test.go:<line>`
  (`Taskfile.yml:73-76`; flags `internal/harness/mutcheck/inputs.go:46-52`). The `Fatalf` that reports a mismatch is
  `:99` today. The route path calls the same function, so a mutant of the text is seen by the `Validate` test.
