# AGENTS.md rules-table changes (test-infra-audit)

Rows land with the pull request whose check enforces them: the replaced row and rows 1 and 2 with pull request 1
(task 2.9), rows 3 to 7 with pull request 3 (task 4.5).

## Replace

Old row:

| Rule | Canonical home | Enforced by |
| --- | --- | --- |
| One NATS image pin; Docker cleanup touches only SemEngine-assigned names | `harness-boundaries` spec | `TestOneImagePin`, `TestSemEngineAssignedNames`, `TestNoBroadDockerCleanup` |

New row:

| Rule | Canonical home | Enforced by |
| --- | --- | --- |
| One NATS image pin, a digest reference; no broad Docker cleanup; a Docker name a script or Compose file assigns starts with `semengine-` | `harness-boundaries` spec | `TestOneImagePin`, `TestSemEngineAssignedNames`, `TestNoBroadDockerCleanup` (`task test:unit`); natsfixture refuses a pin without a digest at run time |

## Add

| Rule | Canonical home | Enforced by |
| --- | --- | --- |
| 1. Integration tests run under plain `go test -tags integration`; `task test:integration` is that command with pinned flags; nothing between them takes a lock or refuses a run (#205 rulings 6132580576, 6132995614) | `integration-test-runner` spec; `docs/test-infrastructure.md` | `TestUnitInvocationsPinned`, extended to `test:integration` (`task test:unit`), for the command; that no code refuses a plain `go test` run is review only |
| 2. natsfixture turns Ryuk off in its process; every container it starts carries the label `org.c360studio.semengine.test`; leftovers older than 30 minutes are removed by the next test process; nothing is removed by name or by a check of whether a process is alive | `nats-fixture` spec, "Ryuk off, labelled containers and the sweep"; `docs/test-infrastructure.md` | natsfixture's tests "Ryuk is off" and "Only old labelled containers are swept" (`task test:integration`); `TestNoBroadDockerCleanup` for scripts; a container started outside natsfixture, and so without the label, is review only |
| 3. An integration test reaches its package's shared NATS server only through the reset | `nats-fixture` spec, "Shared server per package"; `docs/test-infrastructure.md` | review only: the server returns its URL only from the reset, but a URL kept in a package variable escapes that |
| 4. A test starts on a shared server with no stream, consumer, stored byte or other client connection, and closes every connection it opens before it ends | `nats-fixture` spec, "Reset to a verified-empty server" | the reset's checks and its cleanup's connection check at run time, with natsfixture's tests of them (`task test:integration`); a test on its own server is review only |
| 5. No `t.Parallel` in an integration-tagged Go file | `harness-boundaries` spec, "Integration tests in a package run one at a time" | `TestNoParallelIntegrationTests` and its sensitivity test (`task test:unit`); a call in an untagged helper, or on a receiver not named `t`, is review only |
| 6. A test that starts its own NATS server says why, with `// Own server: <reason>` on the line above `natsfixture.New` | `harness-boundaries` spec, "A test with its own server says why"; `docs/test-infrastructure.md` | `TestOwnServerReason` and its sensitivity test (`task test:unit`) at each call site; a package helper that calls `New` for many tests, a renamed import, a function value, and whether the reason holds are review only |
| 7. Harness code exists only for a failure that happens on the tree today and that testcontainers, `go test`, `t.Cleanup` or Task does not handle; its doc comment names that failure and its issue; it is removed once the failure cannot happen | `docs/test-infrastructure.md`, "When we write harness code" and "Retiring harness code" | review only |
