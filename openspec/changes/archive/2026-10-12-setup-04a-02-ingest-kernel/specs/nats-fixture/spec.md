# nats-fixture

## ADDED Requirements

### Requirement: Connected value for a package's tests

`natsfixture` SHALL provide one helper that, given a test, a started fixture and an open function supplied by the
caller (called with a context and the fixture's URL, and returning the opened value and a close function that takes a
context), calls the open function under a bounded context and returns the value. When the open function returns an
error, the helper SHALL fail the test with a message naming the fixture's URL and that error. On success it SHALL
register the close function on the test's cleanup so that it runs before the fixture's own Stop, under a context with
a deadline, and SHALL report a close error as a test error. The helper SHALL import no package of this module outside
`internal/harness`, SHALL start no goroutine, and SHALL hold nothing after it returns beyond the cleanup it
registered.

#### Scenario: Opening fails

- **WHEN** the open function returns an error
- **THEN** the test fails with a message naming the fixture's URL and the error, and no close is registered

#### Scenario: Close runs first, bounded

- **WHEN** a test that opened a value through the helper ends
- **THEN** the close function runs before the fixture stops, with a context that has a deadline, and a close error is
  reported as a test error

#### Scenario: Import bound

- **WHEN** the harness import check lists the helper's package imports
- **THEN** no package of this module outside `internal/harness` is among them
