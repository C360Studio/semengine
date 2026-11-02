# Tasks: flake-gates-own-deadline

Each step ends when `task verify` passes on a clean tree at its commit (#171). The guard (task 3) is the failing-first
test for the sweep (task 2): before the sweep it names 148 sites; after it, none. One PR.

## 1. Design

- [x] 1.1 Design track (`design.md`); owner approval on #500 (comment 6577539980), measurements on #500 (comment
  6577478437).

## 2. Sweep: no test fails on its own deadline

At each site: delete the `case <-time.After(...)` arm that fails the test and wait directly on the ordering event.
Keep a timer arm that passes. Gate per group: `go test -race -count=1 -cpu 1 <pkgs>` and
`go test -count=5 -cpu 1 -shuffle=on <pkgs>`; every kept deadline listed in the PR with its reason.

- [x] 2.1 `natsclient` (65 sites), `natsclient/storagereport` (7).
- [x] 2.2 `config` (25), `internal/cache` (17), `service` (7), `graph/embedding` (7).
- [x] 2.3 `model` (5), `graph/follower` (5), `metric` (3), `processor/graph-ingest` (2), `pkg/lifecycle` (2),
  `processor/graph-index` (1), `pkg/graphview` (1), `graph/clustering` (1).

## 3. Guard and the one deadline

- [x] 3.1 `TestNoOwnDeadlineInTests` and `TestNoOwnDeadlineInTestsSensitivity` in
  `internal/harness/contract/testtext_test.go`: a `case <-time.After(...)` arm whose body fails the test
  (`t.Fatal`, `t.Fatalf`, `t.Error`, `t.Errorf`, `t.FailNow`, `require.`) without `// Own deadline: <reason>` on the
  line above; a passing timer arm is not a violation; a scan of no test file fails. Spec delta: `harness-boundaries`.
  Gate: `go test -race -count=1 -cpu 1 ./internal/harness/contract`.
- [x] 3.2 `-timeout 5m` on `test:unit` and `test:repeat` in `Taskfile.yml`; toolcheck's pin test and the
  `merge-gate` spec say so. Gate: `task test:tools`, `task spec:check`.

## 4. Rule text

- [x] 4.1 `docs/testing.md` "Concurrency and cleanup", developer and reviewer contracts, `AGENTS.md` section 3 row
  for #392 (gate `verify+review`), `.agents/rules-index.md` row 55; no new row. Gate: `task docs:check`,
  `task ateam:doctor`.

## 5. Land

- [x] 5.1 Implementation verified and independently reviewed; specs synced and change archived. Verification and the
  narrow review of the archive commit go on the PR's landing record, after archive.
