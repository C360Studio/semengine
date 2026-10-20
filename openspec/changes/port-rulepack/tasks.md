# Tasks: port-rulepack

Each task names its outcome and the gate that proves it. Tests come first. Each stop's task ends only when
`task verify` passes on a clean tree at its commit (#171).

## 1. Design

- [x] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal and design pass pre-owner review and are accepted by the
  owner (2026-10-20: Q1 yes, Q2 yes). Gate: `task spec:check` and `task docs:check`.

## 2. Port

- [ ] 2.1 Tests first: `pkg/rulepack/identity_test.go` with the pin's `TestValidateID` unchanged plus
  `TestValidateIDEveryByte`, `TestValidateIDBounds` and `TestValidateIDKVTokenRefusal` (design, "Generated checks" and
  "Proving tests and mutants"); then `identity.go` as Decision B (keep `PackIDCharset`, `MaxPackIDBytes`, `ValidateID`;
  one pattern from `PackIDCharset`; true text; `ValidateConfig`, `ValidateRuntimeUpdate`, `packIdentity`,
  `decodePackID` dropped); the `adapt` ledger row; a `docs/repository-map.md` row. Gate:
  `go test -race ./pkg/rulepack`, `go list -deps ./pkg/rulepack` naming no `config` or `types`, `task ledger:check`.
- [ ] 2.2 `task mutate:check` for M1-M10 (detections) and S1 (survivor recorded) in `notes/2.2.md`; `task verify`
  passes on a clean tree.
- [ ] 2.3 Cross-agent review stop (Codex); `reviewed-by:` in the PR body.

## 3. Close

- [ ] 3.1 Archive (`openspec archive port-rulepack`) as the last content commit, syncing `rule-pack-identity` into
  `openspec/specs/`. Gate: `task spec:check` and `task docs:check`.
