# harness-boundaries

## ADDED Requirements

### Requirement: No own deadlines in tests

No `*_test.go` file SHALL contain a `select` arm `case <-time.After(...)` whose body fails the test unless the line
above the arm is a comment `// Own deadline: <reason>`. `go test -timeout` is the one deadline for work that must
finish. The check SHALL have no baseline and no allowlist, and SHALL fail when it scanned no test file.

#### Scenario: What counts as failing the test

- **WHEN** the arm's body calls `t.Fatal`, `t.Fatalf`, `t.Error`, `t.Errorf`, `t.FailNow` or a `require.` function
- **THEN** the arm is a violation; a body that only ends the select (a negative check: nothing arrived) is not

#### Scenario: Shapes outside the check

- **WHEN** a test stores the timer in a variable, uses `time.NewTimer`, or a `context.WithTimeout` deadline
- **THEN** the check does not see it: it matches the literal text `case <-time.After(`, and those shapes stay review
  work

#### Scenario: A test fails on its own deadline

- **WHEN** a test file gains `case <-time.After(10 * time.Second):` followed by `t.Fatal("timed out")`
- **THEN** the contract test fails naming the file and line

#### Scenario: A negative check keeps its timer

- **WHEN** a test file gains `case <-time.After(200 * time.Millisecond):` whose body only ends the select, with the
  failing call in the other arm
- **THEN** the contract test passes

#### Scenario: A deadline a test must own

- **WHEN** the line above the arm is `// Own deadline: Stop must return before the fixture's own 30 s cleanup budget`
- **THEN** the contract test passes

#### Scenario: Nothing scanned

- **WHEN** the check is given a tree with no `*_test.go` file
- **THEN** the contract test fails saying it scanned no test file
