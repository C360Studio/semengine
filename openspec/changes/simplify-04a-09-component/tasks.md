# Tasks: simplify-04a-09-component

Each task names the outcome and the gate that proves it. Tests come first. Record each one failing on the base commit
in `notes/<task>.md`. A compile failure on a missing symbol counts as failing. Stop for review at each `Review stop`.

## 1. Design

- [ ] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal, design and spec delta pass pre-owner review and are accepted
  by the owner on #153 (owner questions 1–5). Gate: `task spec:check` and `task docs:check`.

## 2. Failing tests, first batch (#189)

- [ ] 2.1 `TestValidateFactoryConfigRefusesDataAfterTheFirstValue`, with a `CreateComponent` row whose factory is never
  called (design D1). Gate: fails on the base commit.
- [ ] 2.2 `TestSafeUnmarshalRefusesANilTarget` (D2), plus, if owner question 4 is answered yes, an unknown-key row that
  names the key. Gate: fails (panics) on the base commit.
- [ ] 2.3 `TestGeneratedSchemaUsesOneWordPerType` (D3). Gate: fails on the base commit.

## 3. Implementation, first batch

- [ ] 3.1 The config check refuses data after the first value (D1). Gate: 2.1 passes; `task test:unit`.
- [ ] 3.2 `SafeUnmarshal` refuses a nil target (and unknown keys, if owner question 4 says so); `component/README.md`
  states its contract (D2). Gate: 2.2 passes.
- [ ] 3.3 Array item schemas use `int`, `float` and `bool` (D3). Gate: 2.3 passes; existing schema tests are updated
  to the one vocabulary.

Review stop A (tasks 2.1–3.3).

## 4. Registration and snapshots (#153 items 2–3)

- [ ] 4.1 `TestRegisterRefusesATypeOutsideTheFive`, `TestRegisterKeepsItsOwnCopy`,
  `TestRegisterRefusesADuplicateName` (D4), and a snapshot-independence test that reads `[]Declaration` (D5). Gate:
  fail on the base commit (no symbol, or the wrong return type).
- [ ] 4.2 `Registry.Register(Registration)` replaces `RegisterFactory`, `RegisterWithConfig` and `RegistrationConfig`.
  `Registration.Type` is `types.ComponentType`, and graph-ingest and every test move to the new call (D4). Gate: 4.1's
  registration tests pass; `task test:unit`; `go test -tags integration -race ./component/ ./processor/graph-ingest/`.
- [ ] 4.3 `Snapshots() []Declaration`. `declarationSnapshot`, its revive directive and `Registry.declaration` are
  removed (D5). Gate: 4.1's snapshot test passes; `task lint`.

Review stop B (tasks 4.1–4.3).

## 5. Interface, test code and prose (#197, #153 items 4–5)

- [ ] 5.1 `TestDiscoverableNeedsNoConfigSchema` (D6). Gate: does not compile on the base commit.
- [ ] 5.2 `ConfigSchema()` leaves `Discoverable`, graph-ingest and the test components; graph-ingest's schema test
  reads the registration (D6). Gate: 5.1 passes; `task test:unit`.
- [ ] 5.3 `SimpleMockComponent` moves into a `_test.go` file; `createSimpleMockComponent` and `attack_test.go` are
  deleted (D7); if owner question 5 is answered yes, `ValidateConfig`, `ValidationError` and their tests are deleted
  (D9). Gate: `task build`, `task lint`, `task cover:check`.
- [ ] 5.4 The `LifecycleComponent` doc lists the observable rules, and the `PlatformMeta` comment is corrected (D8).
  Gate: review.

Review stop C (tasks 5.1–5.4).

## 6. Documentation and follow-ups

- [ ] 6.1 `component/README.md` and `doc.go` describe one `Register` call, the schema's one home, and `Snapshots`
  returning declarations; the `docs/admission-ledger.yaml` `component` row's `contract` lists these adaptations and
  its `proving_tests` drops `attack_test.go`. Gate: `task ledger:check`, `task docs:check`.
- [ ] 6.2 If owner question 2 is answered yes, file the rest of #153 as one issue (items 6 and 7 outside `component`,
  and `Declaration`'s parallel lists) and link it from the PR body without a closing keyword. Gate: the issue number is
  on the PR body's follow-up line, and the PR body's `closingIssuesReferences` is read back as #153, #189, #197.

## 7. Verify, review and close

- [ ] 7.1 `task verify` passes on a clean tree at the implementation head.
- [ ] 7.2 Cross-agent review of record on the implementation commit; `reviewed-by:` in the PR body.
- [ ] 7.3 Archive the change (`openspec archive simplify-04a-09-component`) as the last content commit. Gate:
  `task spec:check`.
