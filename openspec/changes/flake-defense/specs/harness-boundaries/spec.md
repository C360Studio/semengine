# harness-boundaries

## ADDED Requirements

### Requirement: No sleeps in tests

No `*_test.go` file and no Go file under `internal/harness/` SHALL contain `time.Sleep`. The check SHALL have no
baseline, no allowlist and no inline exemption, and SHALL fail when it scanned no test file. The check matches the
text `time.Sleep`: a renamed `time` import and a wait built from `time.After` or a timer are outside its scope by
construction.

#### Scenario: Sleep added to a test

- **WHEN** a test file gains `time.Sleep(10 * time.Millisecond)`
- **THEN** the contract test fails naming the file and line

#### Scenario: Nothing scanned

- **WHEN** the check is given a tree with no `*_test.go` file
- **THEN** the contract test fails saying it scanned no test file

### Requirement: No skipped or hidden tests

No `*_test.go` file SHALL call `Skip`, `Skipf` or `SkipNow`, and no `*_test.go` file SHALL carry a build constraint
other than `//go:build integration`. The check SHALL have no baseline, no allowlist and no inline exemption, and
SHALL fail when it scanned no test file. The check matches text: a skip reached through a helper and a platform
suffix in a file name are outside its scope by construction.

#### Scenario: Skip added to a test

- **WHEN** a test file gains `t.Skip("flaky")`
- **THEN** the contract test fails naming the file and line

#### Scenario: Test hidden behind a build tag

- **WHEN** a test file begins with `//go:build flaky`
- **THEN** the contract test fails naming the file and the constraint

## MODIFIED Requirements

### Requirement: No fixed addresses in tests

No `*_test.go` file SHALL contain a fixed broker address (`nats://localhost:`, `nats://127.0.0.1:`, `:4222"`,
`:8222"`) or bind a fixed TCP port (scripts/lint-test-ports.sh SHALL pass). `scripts/lint-test-ports.sh` SHALL
honour no inline exemption, and its failure output SHALL name the offending line and tell the author to bind port 0
and hand the listener itself to the code under test, naming no file outside this repository.

#### Scenario: Fixed dial

- **WHEN** a test dials nats://localhost:4222
- **THEN** the contract test fails naming the line

#### Scenario: Marked fixed port

- **WHEN** a test binds a fixed port on a line that ends with the comment `// gh#220:allow-fixed-port`
- **THEN** scripts/lint-test-ports.sh exits 1 naming the line

#### Scenario: Guidance keeps the listener

- **WHEN** scripts/lint-test-ports.sh fails on a fixed port
- **THEN** its output tells the author to bind port 0 and pass the listener on, and names no SemStreams file
