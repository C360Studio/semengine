# AGENTS.md rules-table changes (test-infra-audit)

Rows land with the pull request whose check enforces them: rows 1 and 2 with pull request 1, rows 3 to 7 with pull
request 3.

## Replace

Old row:

| Rule | Canonical home | Enforced by |
| --- | --- | --- |
| One NATS image pin; Docker cleanup touches only SemEngine-assigned names | `harness-boundaries` spec | `TestOneImagePin`, `TestSemEngineAssignedNames`, `TestNoBroadDockerCleanup` |

New row:

| Rule | Canonical home | Enforced by |
| --- | --- | --- |
| One NATS image pin; no broad Docker cleanup; a Docker name a script or Compose file assigns starts with `semengine-` | `harness-boundaries` spec | `TestOneImagePin`, `TestSemEngineAssignedNames`, `TestNoBroadDockerCleanup` |

## Add

| Rule | Canonical home | Enforced by |
| --- | --- | --- |
| Integration tests run under plain `go test -tags integration`; `task test:integration` is that command with pinned flags and Ryuk off; nothing between them takes a lock or refuses a run (#205 rulings 6132580576, 6132995614) | `integration-test-runner` spec; `docs/test-infrastructure.md` | `TestUnitInvocationsPinned`, extended to `test:integration` (`task test:unit`), for the command and its environment; that no code refuses a plain `go test` run is review only |
| Every container the harness starts carries the label `org.c360studio.semengine.test`; leftovers older than 30 minutes are removed by the next test process; nothing is removed by name or by a check of whether a process is alive | `nats-fixture` spec, "Labelled containers and the sweep"; `docs/test-infrastructure.md` | the sweep's integration test (`task test:integration`) for selection by label and age; `TestNoBroadDockerCleanup` for scripts; a container started outside natsfixture without the label is review only |
| An integration test reaches its package's shared NATS server only through the reset, which fails the test unless the server holds no stream, no consumer and no client connection | `nats-fixture` spec, "Reset to a verified-empty server"; `docs/test-infrastructure.md` | the shared server returns its URL only from the reset (compile time); the reset's checks at run time, with natsfixture's tests of them (`task test:integration`) |
| A test closes every NATS connection it opens before it ends | `nats-fixture` spec, "Reset to a verified-empty server" and "Shared server per package" | the reset's cleanup fails the test, and the shared server's `Stop` fails the package, while the server reports a client connection; a test on its own server is review only |
| No `t.Parallel` in an integration-tagged test file | `harness-boundaries` spec, "Integration tests in a package run one at a time" | `TestNoParallelIntegrationTests` and its sensitivity test (`task test:unit`) |
| A test that starts its own NATS server says why on the line above `natsfixture.New`, with `// Own server: <reason>` | `harness-boundaries` spec, "A test with its own server says why"; `docs/test-infrastructure.md` | `TestOwnServerReason` and its sensitivity test (`task test:unit`); whether the reason holds is review only |
| Harness code exists only for a failure that happens on the tree today and that testcontainers, `go test`, `t.Cleanup` or Task does not handle; its doc comment names that failure and its issue; it is removed once the failure cannot happen | `docs/test-infrastructure.md`, "When we write harness code" and "Retiring harness code" | review only |
