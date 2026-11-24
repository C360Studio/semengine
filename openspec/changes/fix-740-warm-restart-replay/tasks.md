# Tasks: fix-740-warm-restart-replay

Each code task ends at its own tests passing; one clean-tree `task verify` runs before the push. One pull request
(#747): `Closes #740`, `Closes #741`.

## 1. Proposal

- [x] 1.1 Inventory, probes P1 to P8, Fable read, owner ruling (#740 comments 7146176783, 7146268745).
- [x] 1.2 The owner re-fills `Approved-by:` for this proposal.

## 2. Code and test

- [x] 2.1 `TestSmoke_WarmRestartOverALargeStore` (132 MB) shown red at the default `max_pending` on Linux CI
  (`notes/2.1.md`).
- [x] 2.2 Revert `natsclient.Client.NewSibling` (e1e6da25) with `git revert`. Accept: `go build ./...`.
- [x] 2.3 `natsfixture.WithMaxPending(n)`: writes `max_pending` to the broker config file beside `max_payload`,
  refusing n below 1 before any Docker call. Test: a broker started with it reports that value in `/varz`, or the
  server's own config read the fixture already uses. Accept: the natsfixture tests pass.
- [ ] 2.4 `TestSmoke_WarmRestartOverALargeStore` runs on its own broker with `WithMaxPending(536870912)`
  (`// Own server: the documented max_pending`), and passes locally and on Linux CI. Record the CI URL in
  `notes/2.1.md`.

## 3. Docs and spec

- [x] 3.1 `docs/boot.md`: the required `max_pending: 536870912`, its reason (whole-bucket readers replay at once at
  boot, each up to about one flow-control window ahead), what happens without it (`Start` fails with
  `not connected to NATS` after a slow-consumer close), and that the number grows with the reader count, not the
  store. Accept: `task docs:check`.
- [x] 3.2 #741: `openspec/specs/composed-stack/spec.md:8` without the issue reference, and `graph/follower/follower.go:23`
  naming nats.go v1.55.0.
- [x] 3.3 The delta under `specs/` matches the test. Accept: `task spec:check`.

## 4. Close

- [ ] 4.1 `task verify` on a clean tree, push, CI green, cross-agent review of record, `task merge:check -- 747`.
- [ ] 4.2 Archive the change and sync the specs as the last content commit. Accept: `task spec:check`.
