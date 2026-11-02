# harness-boundaries

## ADDED Requirements

### Requirement: No reach check after a generated run

In a `*_test.go` file, a function body that calls `rapid.Check` SHALL NOT, after that call, contain a failing call
(the set "No own deadlines in tests" names) unless the line directly above the statement is a comment
`// After check: <reason>`. A case the test must cover runs as a named witness before the generated run. The check
SHALL have no baseline and no allowlist, and SHALL fail when it scanned no test file.

#### Scenario: A reach check after the run

- **WHEN** a test calls `rapid.Check` and then `t.Fatalf(...)` when a counter the closure filled is zero
- **THEN** the contract test fails naming the file and line

#### Scenario: Witnesses before the run

- **WHEN** a test runs named inputs through its assertions, failing on any, and then calls `rapid.Check`, and after it
  only logs
- **THEN** the contract test passes

#### Scenario: Assertions inside the property

- **WHEN** the closure passed to `rapid.Check` calls `t.Fatalf` on its drawn input
- **THEN** the contract test passes: that is the property's own assertion

#### Scenario: A post-run assertion with a reason

- **WHEN** the line above a failing call after the run is `// After check: the run must leave no goroutine behind`
- **THEN** the contract test passes

#### Scenario: Shapes outside the check

- **WHEN** a test runs a property through `rapid.MakeCheck` or another entry point, imports `rapid` under an alias,
  keeps a reach check in a helper the test calls after the run or in a `defer` or `t.Cleanup` registered before it, or
  runs the check inside a helper and fails after calling the helper
- **THEN** the check does not see it: it recognises `rapid.Check` calls (including inside a `t.Run` closure) and failing
  calls in the same function body, and those shapes stay review work

#### Scenario: Nothing scanned

- **WHEN** the check is given a tree with no `*_test.go` file
- **THEN** the contract test fails saying it scanned no test file
