# Inventory: fix-369-subject-grammar

base: 95120560f3e90867d2025d368fb325d70e42e99d (claim commit on claude/subject-grammar-369, which touches no code)
parent: 67d2dc6fc5d0a6cbd4b744f031e2ad771b58bab9 (origin/main at claim; main is now eefb520, which changes only
go.mod and go.sum). Every `path:line` below reads the same at all three commits.

## 1. The claimed gap

- `component/port_codec.go:322` — `if strings.TrimSpace(port.Subject) == "" {` (normalizeNATSPort; emptiness only)
- `component/port_codec.go:333` — `if strings.TrimSpace(port.Subject) == "" {` (normalizeNATSRequestPort)
- `component/port_codec.go:356-358` — `if strings.TrimSpace(subject) == "" {` /
  `return nil, fmt.Errorf("field \"subjects[%d]\" must not be empty", index)`
- `config/streams.go:28` — `Subjects  []string` with `json:"subjects"` (no grammar check)
- Port reach: decode `component/port_codec.go:163` decodePortable -> `:206-212` decodeBoundConfig -> binding.normalize;
  construction `component/port_resolver.go:23` canonicalizePortable -> the same normalize. One choke point per kind.
- Stream reach (config side): `config/config.go:270` `if _, err := ValidateStreamDeclarations(c); err != nil {` ->
  `config/stream_bounds.go:609-611` -> planStreams `:440` -> resolveStreamDeclarations `:218`: framework constants
  `:231`, operator map `:241-257` (name check only, `:247` checkOrdinaryStream), port-derived `:262-298` (subjects
  come from JetStreamPort facts). `EnsureStreams` (`config/streams.go:329-330`) reaches the same planStreams; gopls
  finds no production caller of EnsureStreams today.
- Stream reach (composition side): see § 3, composition.Analyze. It reads the explicit `streams` block and every
  JetStream output's subjects at validation and at boot. It does not check them against the grammar.
- Searches: `git grep -n -i "func .*subject" -- '*.go' ':!*_test.go' | grep -i valid` -> only flowgraph:346/350 and
  config/config.go:286; `git grep -nE 'len\(.*\.Subjects\) == 0' config/` -> none.

## 2. Every spelling of the subject grammar

- `internal/flowgraph/flowgraph.go:346` — `func ValidSubject(subject string) bool {` (owner per composition-validation
  spec)
- `internal/flowgraph/flowgraph.go:350` — `func validSubjectTokens(tokens []string) bool {` (read by ValidSubject and
  by SubjectsOverlap at :323)
- `internal/flowgraph/flowgraph.go:377` — `func matchNATSPattern(subject, pattern string) bool {` (finds wildcards with
  strings.Contains and never reads the grammar; already in the tree)
- `config/config.go:286` — `func isValidNATSSubjectPart(s string) bool {` (a different fact: the allowed characters of
  platform.org)
- `natsclient/kv_key_contract.go:94,118,157` — ValidateKVLiteralToken / ValidateKVLiteralKey /
  ValidateKVWildcardFilter (KV key grammar, stricter; a different fact)
- Readers, not spellings: `service/message_logger.go:318,370` — `if !flowgraph.ValidSubject(subject) {`;
  `composition/analyze.go:140` — `if !flowgraph.SubjectsOverlap(filter, subject) {`; `composition/analyze.go:198` —
  `if flowgraph.SubjectCovers(filter, subject) {`

## 3. Adjacent claims

- **Existing validation-time checker.** `composition/analyze.go:26` —
  `func Analyze(declarations []component.Declaration, streams config.StreamConfigs) *Result {`;
  `:33` — `for _, finding := range reservedCaptureFindings(sorted, streams) {` (runs before the early returns,
  `:30-32`); `:134` —
  `func reservedCaptureFindings(declarations []component.Declaration, streams config.StreamConfigs) []Finding {`
  (checks the explicit block, named by key, and each JetStream output, `:127-133`). Callers: boot,
  `service/component_manager.go:362` — `result := composition.Analyze(cm.registry.Snapshots(), cm.bootStreams)` (an
  error finding refuses boot, `:357-359`); offline, `composition/validate.go:21` —
  `func Validate(catalog *component.Registry, cfg *config.Config) (*Result, error) {`.
  `composition` already imports both `internal/flowgraph` and `config` (`composition/analyze.go:11`).
  Today a malformed filter passes this check silently: per spec `:137`, a subject that fails the grammar "SHALL
  overlap nothing".
- What binds that seam: spec `openspec/specs/composition-validation/spec.md:13` — "Requirement: Validation is a pure
  function with a closed vocabulary"; `:26` — "`type` SHALL be one of the exported constants ..."; `:42-46` —
  "Scenario: The vocabulary is closed" (`TestValidateFindingsVocabularyIsClosed`); `composition/findings.go:40` —
  `TypeReservedSubjectCapture = "reserved_subject_capture"` (the last constant in the list). A new finding type extends
  that closed list. `:121-124` — "The example configuration carries no error finding". ADR-100,
  `docs/adr/100-compositions-are-validated-diagrams-are-projections.md` (one judgment shared by offline validation and
  boot).
- Spec `composition-validation/spec.md:134` — "`internal/flowgraph` SHALL own one subject grammar"; `:139-140` — "the
  grammar SHALL have no second copy."
- Spec `openspec/specs/component-registration/spec.md:43` — "`component` SHALL NOT import, directly or through its
  dependencies, ... the `graph` package or a package under it, `internal/graphmutation`"; guard
  `internal/harness/contract/componentdeps_test.go:112`.
- `internal/flowgraph/flowgraph.go:10` — `"github.com/c360studio/semengine/component"`; `:11` —
  `"github.com/c360studio/semengine/internal/graphmutation"`. `go list -deps ./internal/flowgraph` includes component,
  graph and internal/graphmutation, and does not include config.
- Spec `openspec/specs/service-lifecycle/spec.md:640-644` — the logger's Start check (stricter than NATS on `a*` and
  form feed).
- Ledger: `component` adapt (`docs/admission-ledger.yaml:2262`), `internal/flowgraph` adapt (`:2642`), `config` adapt
  (`:2711`), `config/stream_bounds.go` **carry** (`:2751`, `:2760`), "held byte-equal to the pin" (checked by
  `task ledger:check`).
- Issues: #369 (this one). Adjacent, staying open: #16 (stream filters overlapping reserved RPC subjects; its check is
  reservedCaptureFindings above); #331 (the graph-ingest refusal does not name the port); #363 (flow-path reachability
  is computed in service, not composition). The queries `port_codec`, `streams.go`, `subject grammar`, `ValidSubject`,
  `StreamConfig`, `normalizeNATSPort` and `normalizeJetStreamPort` returned only #369 and epics #7 and #9. The other
  hits for `component`, `config` and `flowgraph` match the word, not these sites.
- Open PRs (`gh pr list --state open --json files`): #376 (service/*), #374 (harness/agents), #358 (go.mod, merged
  since as eefb520). None overlap. #380 is this claim.
- Probe evidence: #367 comment 6343878154 (nats.go v1.54.0, nats-server v2.15.1): `a.>.b` subscribes without error,
  then the connection is CLOSED; `a.foo\fbar` is accepted by the client, with server.IsValidSubject=false; `a*` and
  `a.b*.c` are valid to the server. That probe covers subscribe only; publish was not probed.

## 4. Who it is for

Issue #369 itself implies no new exported surface. The people who read these values are config authors and component
authors (port config) and operators (`config.streams`). Any relocated grammar symbol, or any new finding type, is a
question for after the pass.

## 5. Problem shape

Admit-or-refuse at a validation seam, with an error naming the field. Closest existing instances:

- `composition/analyze.go:134-150` — reservedCaptureFindings: stream and port subjects judged through flowgraph at
  offline validation and at boot, reported as findings that name the component, port and stream.
- `component/port_codec.go:253-255` — `portConfigError`: `port %q kind %q field %q: %v`, a refusal at decode.
- `service/message_logger.go:318-319` — ValidSubject plus a named source, a refusal at Start.

## 6. Pin decision records

Not a porting change; it carries no package from the pin.

## Measurement: fixtures against the grammar

On a `git archive` copy of base (deleted afterwards), a copy of the ValidSubject grammar was added in three places:
the three port normalizers in `component/port_codec.go`, and the framework-constant and operator-map stream subjects in
`resolveStreamDeclarations`, **inside the carry file `config/stream_bounds.go`**. Editing that file breaks its
byte-equality to the pin, so `task ledger:check` was not run: it cannot pass that way. The edit was for measurement
only and is no placement proposal.
`go test -count=1 ./...` passed for every package except `internal/harness/contract`, which failed with
`git ls-files: exit status 128` because the copy has no .git. A probe test showed the check was live: `a*`, `a.>.b`,
`a..b`, `a b` and `a.\fb` were each refused naming `field "subject"`. Integration-tagged tests were not run.
`composition.Analyze` was not probed with the check. `config/example_config.json` subjects: `entity.>` and
`graph.mutation.>`. Production port literals: `processor/graph-ingest/component.go:346` (`entity.>`) and `:350`
(`graphmutation.SubjectFamily` = `graph.mutation.>`, `internal/graphmutation/protocol.go:18`). Framework streams
`config/streams.go:97,111,123,135,160,174` (`logs.>` ... `$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES.>`) all pass
the grammar.

## Adopter seam inventory

1. Must know: nothing new. The NATS subject rules are what they must already obey.
2. Do nothing: today a malformed subject is accepted at decode and passes composition analysis (it overlaps nothing).
   It then fails at subscribe time with ErrBadSubject, or the server closes the connection (`a.>.b`), or the server
   rejects it (form feed): far from the config line.
3. Finds out: today at runtime, as a closed connection at worst. The target is a validate or boot error, or an error
   finding, that names the component, the port and field, or the stream and index.
4. Should know: nothing. The gap: `a*` (a valid NATS literal) will be refused, so an author who uses one learns the
   stricter rule from the error text.
