# Tasks: simplify-04a-09-component

Each task names the outcome and the gate that proves it. Tests come first. Record each one failing on the base commit
in `notes/<task>.md`. A compile failure on a missing symbol counts as failing. Stop for review at each `Review stop`.

## 1. Design

- [ ] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal, design and spec delta pass pre-owner review and are accepted
  by the owner on #153 (owner questions 1–5). Gate: `task spec:check` and `task docs:check`.

## 2. Failing tests, first batch (#189)

- [ ] 2.1 `TestValidateFactoryConfigRefusesDataAfterTheFirstValue`, with a `CreateComponent` row whose factory is never
  called (design D1). Gate: fails on the base commit.
- [ ] 2.2 `TestSafeUnmarshalRefusesANilTarget` (D2), with a valid-input row and an empty-input row, plus, if owner
  question 4 is answered yes, an unknown-key row that names the key. Gate: fails on the base commit (the first row
  panics, the second returns nil).
- [ ] 2.3 `TestGeneratedSchemaUsesOneWordPerType` (D3). Gate: fails on the base commit.

## 3. Implementation, first batch

- [ ] 3.1 The config check refuses data after the first value (D1). Gate: 2.1 passes; `task test:unit`.
- [ ] 3.2 `SafeUnmarshal` refuses a nil target (and unknown keys, if owner question 4 says so); `component/README.md`
  states its contract (D2). Gate: 2.2 passes.
- [ ] 3.3 Array item schemas use `int`, `float` and `bool` (D3). Gate: 2.3 passes; existing schema tests are updated
  to the one vocabulary.

Review stop A (tasks 2.1–3.3).

## 4. Registration and snapshots (#153 items 2–3)

- [ ] 4.1 `TestComponentTypeValidate` (package `types`), `TestRegisterRefusesATypeOutsideTheFive`,
  `TestRegisterKeepsItsOwnCopy`, `TestRegisterRefusesADuplicateName` (D4), and a snapshot-independence test that
  reads `[]Declaration` (D5). Gate: fail on the base commit (no symbol, or the wrong return type).
- [ ] 4.2 `types.ComponentType.Validate()` holds the five-type switch, and `ComponentConfig.Validate` calls it.
  `Registry.Register(Registration)` replaces `RegisterFactory`, `RegisterWithConfig` and `RegistrationConfig`, calls
  `Validate` on the type, and copies the registration with the helper `ListFactories` also uses.
  `Registration.Type` is `types.ComponentType`, and graph-ingest and every test move to the new call (D4). Gate: 4.1's
  type and registration tests pass; `task test:unit`;
  `go test -tags integration -race ./component/ ./processor/graph-ingest/`.
- [ ] 4.3 `Snapshots() []Declaration`. `declarationSnapshot`, its revive directive, `Registry.declaration` and
  `Registry.declarationsSnapshot` are removed (D5). Gate: 4.1's snapshot test passes; `task lint`.

Review stop B (tasks 4.1–4.3).

## 5. Interface, test code and prose (#197, #153 items 4–5)

- [ ] 5.1 `TestDiscoverableNeedsNoConfigSchema` (D6). Gate: does not compile on the base commit.
- [ ] 5.2 `ConfigSchema()` leaves `Discoverable`, graph-ingest and the test components; graph-ingest's schema test
  reads the registration (D6). Gate: 5.1 passes; `task test:unit`.
- [ ] 5.3 `SimpleMockComponent` moves into a `_test.go` file; `createSimpleMockComponent` and `attack_test.go` are
  deleted (D7); if owner question 5 is answered yes, `ValidateConfig`, `ValidationError` and their tests are deleted
  (D9). Gate: `task build`, `task lint`, `task cover:check`.
- [ ] 5.4 The `LifecycleComponent` doc lists ADR-108 Decision 3's rules; the `component.PlatformMeta` alias is
  removed and `Dependencies.Platform` is `types.PlatformMeta` (D8). Gate: review for the doc; `task build` and
  `task test:unit` for the alias.

Review stop C (tasks 5.1–5.4).

## 6. Documentation and follow-ups

- [ ] 6.1 `component/README.md` and `doc.go` describe one `Register` call, the schema's one home, and `Snapshots`
  returning declarations, and `Dependencies.Platform` as `types.PlatformMeta`; the `docs/admission-ledger.yaml`
  `component` row's `contract` lists these adaptations and its `proving_tests` drops `attack_test.go`; the `types`
  row's `contract` names `ComponentType.Validate`. Gate: `task ledger:check`, `task docs:check`.
- [ ] 6.2 If owner question 2 is answered yes: comment #153 item 6 onto #196 and item 7's `pkg/lifecycle` structs
  onto #272; file one issue for the storage doc comment and `Declaration`'s parallel lists, with the reason they wait
  (`Port.Facts()` can fail), and link it from the PR body without a closing keyword. Gate: each comment and the issue
  are read back, and the PR body's `closingIssuesReferences` is read back as #153, #189, #197.

## 7. Verify, review and close

- [ ] 7.1 `task verify` passes on a clean tree at the implementation head.
- [ ] 7.2 Cross-agent review of record on the implementation commit; `reviewed-by:` in the PR body.
- [ ] 7.3 Archive the change (`openspec archive simplify-04a-09-component`) as the last content commit. Gate:
  `task spec:check`.
