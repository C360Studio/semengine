# Tasks: simplify-04a-09-component

Each task names the outcome and the gate that proves it. Tests come first. Record each one failing on the base commit
in `notes/<task>.md`. A compile failure on a missing symbol counts as failing. Stop for review at each `Review stop`.

## 1. Design

- [x] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal, design and spec delta pass pre-owner review and are accepted
  by the owner. Owner rulings of 2026-10-16 on questions 1–5 are recorded in `design.md` and on #153 (comment
  6214880970) and #189 (comment 6214881340); the reviewer passed that revision at
  `b679ee4`. Gate: `task spec:check` and `task docs:check`.

## 2. Failing tests, first batch (#189)

- [x] 2.1 `TestValidateFactoryConfigRefusesDataAfterTheFirstValue`, with a `CreateComponent` row whose factory is never
  called (design D1). Gate: fails on the base commit.
- [x] 2.2 `TestSafeUnmarshalRefusesANilTarget` (D2), with a valid-input row and an empty-input row, and
  `TestSafeUnmarshalRefusesAnUnknownKey`, which requires the error to name the key. Gate: both fail on the base commit
  (the first row panics, the second returns nil, the unknown key is accepted).
- [x] 2.3 `TestGeneratedSchemaUsesOneWordPerType` (D3). Gate: fails on the base commit.

## 3. Implementation, first batch

- [ ] 3.1 The config check refuses data after the first value (D1). Gate: 2.1 passes; `task test:unit`.
- [ ] 3.2 `SafeUnmarshal` checks the target first, then decodes strictly: unknown keys and data after the first value
  are refused; `component/README.md` states its contract (D2). Gate: 2.2 passes.
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

## 5. Interface, test code and prose (#197, #153)

- [ ] 5.1 `TestDiscoverableNeedsOnlyPortsHealthAndFlow` (D6). Gate: does not compile on the base commit.
- [ ] 5.2 `ConfigSchema()` and `Meta()` leave `Discoverable`, graph-ingest and the test components; `Metadata` is
  deleted; graph-ingest names its consumer with `c.name`; its schema test reads the registration (D6). Gate: 5.1
  passes; `task test:unit`; `go test -tags integration -race ./component/ ./processor/graph-ingest/`.
- [ ] 5.3 `SimpleMockComponent` moves into a `_test.go` file; `createSimpleMockComponent`, `attack_test.go`,
  `ValidateConfig`, `ValidationError` and their tests are deleted (D7, D9). Gate: `task build`, `task lint`,
  `task cover:check`.
- [ ] 5.4 The `LifecycleComponent` doc lists ADR-108 Decision 3's rules as outcomes; the `component.PlatformMeta` alias
  is removed and `Dependencies.Platform` is `types.PlatformMeta` (D8). Gate: review for the doc; `task build` and
  `task test:unit` for the alias.

Review stop C (tasks 5.1–5.4).

## 6. Documentation and follow-ups

- [ ] 6.1 `component/README.md` and `doc.go` describe one `Register` call, the registration as the one home of a
  factory's schema, description and version, a four-method `Discoverable`, and `Snapshots`
  returning declarations, and `Dependencies.Platform` as `types.PlatformMeta`; the `docs/admission-ledger.yaml`
  `component` row's `contract` lists these adaptations and its `proving_tests` drops `attack_test.go`; the `types`
  row's `contract` names `ComponentType.Validate`. Gate: `task ledger:check`, `task docs:check`.
- [ ] 6.2 Route the rest of #153 (owner ruling 2): post the two comments in `notes/issue-153-remainder.md` on #196 and
  #272; file its issue text, replace the placeholder there with the number, and link it from the PR body without a
  closing keyword. Gate: both comments and the issue are read back, and the PR body's `closingIssuesReferences` is
  read back as #153, #189, #197.

Review stop D (tasks 6.1–6.2).

## 7. Verify, review and close

- [ ] 7.1 `task verify` passes on a clean tree at the implementation head.
- [ ] 7.2 Cross-agent review of record on the implementation commit; `reviewed-by:` in the PR body.
- [ ] 7.3 Archive the change (`openspec archive simplify-04a-09-component`) as the last content commit. Gate:
  `task spec:check`.
