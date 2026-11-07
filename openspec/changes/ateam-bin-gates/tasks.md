# Tasks: ateam-bin-gates

Each step ends when `task verify` passes on a clean tree at its commit (#171). One PR.

## 1. Gates on .bin/ateam

- [x] 1.1 `ateam:doctor`, `spec:check` and `spec:queue` depend on `ateam:bin` and run `.bin/ateam`;
  `TestUnitInvocationsPinned` and `TestSpecTaskWiring` hold the new lines and the `deps` key.
- [x] 1.2 `TestAteamTemplatesUnchanged` runs `.bin/ateam` and fails naming `task ateam:bin` when it is missing;
  `docs/testing.md` drops the `PATH` workaround.

## 2. Spec

- [ ] 2.1 The spec deltas applied to `openspec/specs/` and the change archived.
