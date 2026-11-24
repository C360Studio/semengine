# Tasks: design-570-product-boot-contract

Tests come first. Each code task ends at a clean-tree local `task verify`; docs tasks run the preflight docs gates.
"Shown to fail" means the failing-first output in `notes/<task>.md`. One pull request (#739), `Closes #570`.

## 1. Design

- [ ] 1.1 Inventory and design: INVENTORY PASS (done, round 2), then the pre-owner design review passes; the owner
  fills `Approved-by:` and rules on open questions 1 and 2.
  Hold: OPEN-Q proposal awaits owner approval

## 2. service: refuse a platform the configuration manager did not establish

- [ ] 2.1 Test first, shown failing (`notes/2.1.md`): `TestComponentManagerRefusesAPlatformOtherThanTheEstablishedOne`.
  - On a fresh bucket, start a `config.Manager` declaring org `o` and stem `s`, so it mints `s-<hex>`. Register one
    component factory that counts its calls.
  - It is a table test over `deps.Platform`, with each field empty, equal to the effective value or different:
    nine cases, covered exhaustively, so no generated check (design D3).
    - Wholly empty: not refused for this reason.
    - Both fields equal (`{o, s-<hex>}`): constructed.
    - Each of the other seven cases is refused with an error that `errs.IsInvalid` reports and that names both
      pairs. These include the stem (`{o, s}`), an org-only mismatch (`{other, s-<hex>}`), and the two half-empty
      pairs (`{o, ""}` and `{"", s-<hex>}`).
  - For every refused case: the factory's count is 0, and `ConfigureFromServices` returns the error with
    `GetService` finding nothing.

  Premises: `service/component_manager.go:186-192` (snapshot read), `:217` (copied as given);
  `config/config.go:123-133` (`Mutate` swaps a clone, so the caller's `cfg` keeps the stem).
  Accept: the test fails on the base commit for the right reason (no error).
- [ ] 2.2 Implement design D3 (B), plus a doc comment on `NewComponentManager` and the `Platform` row at
  `service/README.md:240`. Fix the fixtures that pass the stem to a manager that minted:
  - `service/component_manager_boot_only_integration_test.go:65`
  - `service/config_boot_process_restart_integration_test.go:175`
  - `service/component_manager_boot_findings_integration_test.go:112`

  Each must read the effective pair.
  Accept: 2.1 passes, `task test:integration` for `./service/` passes, `task verify`.

## 3. Specs

- [ ] 3.1 Deltas `specs/service-lifecycle/spec.md` and `specs/config-manager/spec.md` as drafted. The
  `config-manager` Requirement names an existing test (`config/manager_identity_integration_test.go:160` —
  `require.ElementsMatch(t, []string{"org", "stem", "id"}, mapKeys(fields))`, in
  `TestConfigManagerFirstBootMintsPlatformIdentity`). Accept: `task spec:check`.

## 4. Docs

- [ ] 4.1 Write `docs/boot.md` per design D1. Every "if skipped" line cites the code that produces it:
  - item 1: `processor/graph-ingest/consume.go:218`
  - item 2: `processor/graph-ingest/component.go:272`
  - item 3: `service/service_manager.go:165-166`, `service/service_configs.go:90`
  - item 6: `processor/graph-ingest/authority_gate.go:52`
  - item 7: `config/manager.go:492-496`
  - F1: `graph/entitytext/text.go:38-41`
  - F3: `service/component_manager.go:186-189`
  - F4: `config/config.go:707-713`
  - F5: `service/component_manager.go:261-262`
  - F6: smoke `:254,269,319`

  No reconnect time is stated. Plain words; no port history.
  Accept: a reviewer checks each step against `internal/smoke/stack_integration_test.go:242-320`;
  `task docs:check`.
- [ ] 4.2 Correct the examples:
  - `service/doc.go:80-84`
  - `service/README.md:97-101,348` (use `effective.Services` and `deps.Platform` from `effective`)
  - `docs/contract.md:146-191` (drop the stale note at `:150-152`; point to `docs/boot.md`)
  - Add links from `service/README.md:82` and `config/README.md:170`

  Accept: `task docs:check`; `task build` (doc.go compiles).
- [ ] 4.3 ADR-104: a port-note correction under `:183`, saying the record's shape is normative in `config-manager`
  ("The platform identity record is a read contract"). Accept: `task docs:check`.

## 5. Close

- [ ] 5.1 Archive the change and sync the specs as the last content commit. Accept: `task spec:check`,
  `task merge:check -- 739`.
