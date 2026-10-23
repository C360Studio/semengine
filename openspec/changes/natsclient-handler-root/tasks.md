# Tasks: natsclient-handler-root

Each task names its outcome and the gate that proves it. Tests come first. Record in `notes/<task>.md` how each test
fails on the base commit, or which mutant it detects. Review stops (Codex) come after the design and after the code.
Each task ends only when `task verify` passes on a clean tree at its commit (#171).

## 1. Design

- [ ] 1.1 Inventory and design (`design.md`) pass review, with a pin-version nats.go probe for every library-timing
  statement (ADR-112); the `transport-client` delta is written under `specs/`, `skip_specs` is dropped, and
  `task spec:check` is green. Gate: `task spec:check` and `task docs:check`.

## 2. Code

- [ ] 2.1 Tests first for the request-handler context (#311) and the closed-connection refusal (#317). Gate: the tests
  fail on the base (`notes/2.1.md`).
- [ ] 2.2 `SubscribeForRequests` handler root, its callers, and the `Connect` doc and refusal text. Gate:
  `task verify` green.
- [ ] 2.3 Sensitivity: the mutants the design names, each detected or reported as a survivor (`notes/2.3.md`).

## 3. Docs and archive

- [ ] 3.1 `natsclient` README/doc and ADR-112 cites updated.
- [ ] 3.2 Spec deltas synced into `openspec/specs/` and `skip_specs` removed; archive as the last content commit.
