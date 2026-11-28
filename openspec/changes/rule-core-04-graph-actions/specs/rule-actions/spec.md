# rule-actions Specification

## ADDED Requirements

### Requirement: The rule package reaches no agentic or governance package

`processor/rule` and its subpackages SHALL NOT import, directly or through their dependencies, a package whose import
path has an element named `agentic`, beginning `agentic-`, or named `governance`. A sensitivity test SHALL require the
contract test's failure for a temporary module whose rule package reaches such a path.

#### Scenario: Dependency listing

- **WHEN** a contract test lists the dependencies of `./processor/rule/...`
- **THEN** no listed path has an `agentic`, `agentic-*` or `governance` element
- **AND** the tests that verify this are `TestRuleReachesNoAgenticOrGovernancePackage` and
  `TestRuleReachesNoAgenticOrGovernancePackageSensitivity`

### Requirement: An action runs through its type's handler

The rule processor SHALL run an action by calling `Run(ctx, action, execution context, outputs)` on the handler its
type has in the handler set the executor was built from, built-in or a product's, with the action's `Body` as decoded
and the outputs of "A handler changes the world through its outputs". A type the set does not hold, a nil context and
a nil execution context SHALL each be refused, classified invalid, naming the type and the rule, with no handler
called.

#### Scenario: A product's handler runs at fire

- **WHEN** a set built by `NewHandlers` holds a product handler for `test_family`, a rule using it loads, and the
  executor runs that rule's action
- **THEN** the handler's `Run` receives the action whose `Body` equals the rule file's bytes, the execution context and
  the framework's outputs
- **AND** the test that verifies this is `TestRunDispatch`

### Requirement: A handler's error reaches the caller intact

An error a handler's `Run` returns SHALL reach the executor's caller wrapped, naming the rule and the type, so that
`errors.As` and `errors.Is` still find what the handler returned.

#### Scenario: A handler's typed error passes through

- **WHEN** a handler's `Run` returns a `*DenyVerdict`, or an error classified transient
- **THEN** the executor's error yields that `*DenyVerdict` through `errors.As`, or keeps the transient class
- **AND** the test that verifies this is `TestRunDispatch`

### Requirement: The executor is built once with its dependencies

The executor SHALL be built by one constructor that takes the handler set, the publisher and, when the set was built
with triple writes on, the triple writer, and SHALL have no method that changes a dependency afterwards. The
constructor SHALL refuse, classified invalid, a handler set `NewHandlers` did not build, a missing publisher, a set
built with triple writes on and no writer, and a writer given with triple writes off. The executor SHALL be safe for
concurrent runs.

#### Scenario: The set and the writer disagree

- **WHEN** the executor is built from a set with triple writes on and no triple writer
- **THEN** construction fails, classified invalid, and no executor is returned
- **AND** the test that verifies this is `TestNewActionExecutorRefuses`

### Requirement: A handler changes the world through its outputs

The outputs handed to every handler SHALL publish and add one triple; the built-ins SHALL also remove and replace
through them. Each statement SHALL carry source `rule_engine`, its write time and confidence 1, never a handler's
choice, and go to graph-ingest's typed mutation operations. A built-in handed outputs the executor did not build SHALL
refuse, classified invalid, sending nothing: an in-package guard, not a product-facing contract.

#### Scenario: A handler's triple carries the rule source

- **WHEN** a product handler adds a triple through its outputs
- **THEN** the stored statement's source is `rule_engine` and its timestamp is set
- **AND** the test that verifies this is `TestTripleWriter`

### Requirement: A product's triple add fails when triple writes are off

With the handler set built with triple writes off, a product handler's triple add through its outputs SHALL fail at
fire, classified invalid, saying the rule processor writes no triples, with nothing sent.

#### Scenario: A product handler adds a triple with writes off

- **WHEN** the set was built with triple writes off and a product handler adds a triple through its outputs
- **THEN** the add fails, classified invalid, and no request is sent
- **AND** the test that verifies this is `TestOutputsAddTripleWritesOff`

### Requirement: A triple write is checked before it is sent

A triple write SHALL pass `ValidateEntityID` on its subject and `RequireDeclaredPredicate` on its predicate before
any request, and otherwise SHALL be refused, classified invalid, with nothing sent.

#### Scenario: A bad subject is refused before sending

- **WHEN** a handler adds a triple whose subject is not a six-part entity ID
- **THEN** the add fails, classified invalid, and no request is sent
- **AND** the test that verifies this is `TestTripleWriterRefusesBeforeSending`

### Requirement: Every publish refuses the reserved reply space

The outputs SHALL refuse to publish to a subject in `user.response.>`, classified invalid and naming the subject,
before the publisher is called, whatever handler asks and whatever substitution produced the subject.

#### Scenario: A product handler publishes to the reserved space

- **WHEN** a product handler's `Run` publishes through its outputs to `user.response.x`
- **THEN** the publish fails, classified invalid, and the publisher is not called
- **AND** the test that verifies this is `TestOutputsPublish`

### Requirement: A publish action sends the rule envelope

A `publish` action SHALL publish to its filled subject one JSON object with exactly the keys `entity_id` (the
execution context's entity ID), `subject` (the filled subject), `timestamp` (RFC 3339 with nanoseconds, in UTC),
`source` (`rule_engine`) and `properties`: the action's properties with each top-level string value filled as text and
every other value as written, or `null` when the action has none.

#### Scenario: semboids' steering envelope

- **WHEN** semboids' `food-attract` action runs on a message whose `boid_id` is 7 and `zone_id` is `food-1`
- **THEN** one message goes to `boids.steering` whose `properties` are `{"boid_id": "7", "zone_id": "food-1", "kind":
  "attract", "ttl_ticks": 90}` and whose `source` is `rule_engine`
- **AND** the test that verifies this is `TestPublishAction`

### Requirement: An add appends the rule's statement

`add_triple` SHALL append its statement to an existing entity. An append that adds nothing SHALL succeed. An entity
that does not exist SHALL be refused classified invalid with code `entity_not_found`; a subject the reply reports
failed SHALL be refused with the reply's class and code.

#### Scenario: An add to an entity that does not exist

- **WHEN** an `add_triple` action names a subject no entity has
- **THEN** it fails with code `entity_not_found`, and no entity is created
- **AND** the test that verifies this is `TestTripleWriter`

### Requirement: A remove clears only what rules wrote

`remove_triple` SHALL read its subject exactly and send a conditional replace that removes every statement of its
predicate whose source is `rule_engine`, and SHALL leave the predicate's statements from every other source. A remove
with no `rule_engine` statement to clear SHALL succeed; one whose entity does not exist SHALL fail with the read's
`entity_not_found`, classified invalid.

#### Scenario: Another writer's value stays

- **WHEN** a predicate holds one statement from `rule_engine` and one from another source, and a `remove_triple` action
  names it
- **THEN** the predicate holds only the other source's statement
- **AND** the test that verifies this is `TestTripleWriter`

### Requirement: An update replaces in one write

`update_triple` SHALL read its subject exactly and send a conditional replace that makes the `rule_engine` statements
of its predicate exactly its one statement, committing at most one write, and SHALL leave the predicate's statements
from every other source. An update whose entity does not exist SHALL fail with the read's `entity_not_found`,
classified invalid.

#### Scenario: An update over a held value

- **WHEN** a predicate holds a `rule_engine` statement and an `update_triple` action gives it another value
- **THEN** the entity's revision rises by exactly one and the predicate's only `rule_engine` statement is the new value
- **AND** the test that verifies this is `TestTripleWriter`

### Requirement: A graph write retries only what committed nothing

A remove or update whose conditional replace meets a revision mismatch SHALL read the entity again and retry, at most
five attempts in all, and then SHALL return the last mismatch with its classification. A triple write whose commit is
unknown SHALL return that failure at once and SHALL NOT be retried. An add SHALL be sent once.

#### Scenario: A mismatch, then success

- **WHEN** a remove's first conditional replace meets a revision mismatch and its second is applied
- **THEN** the action succeeds after two reads and two replaces
- **AND** the test that verifies this is `TestTripleWriterRetries`

#### Scenario: Mismatches every time

- **WHEN** every conditional replace of a remove meets a revision mismatch
- **THEN** the action fails after exactly five replaces and `errors.Is(err, errs.ErrRevisionMismatch)` holds
- **AND** the test that verifies this is `TestTripleWriterRetries`

#### Scenario: Commit unknown

- **WHEN** a remove's conditional replace fails with its commit unknown
- **THEN** the action fails with that classification after exactly one replace
- **AND** the test that verifies this is `TestTripleWriterRetries`

### Requirement: A triple action targets the firing entity unless it names one

A triple action without a `subject`, or with an empty one, SHALL write to the execution context's entity ID; one with a
`subject` SHALL write to its filled value. An action with neither SHALL be refused, classified invalid. Its `predicate`
SHALL be used as written. A whole `object` that is one token SHALL keep its value's type; any other `object` SHALL be
filled as text.

#### Scenario: A stamp on another entity

- **WHEN** an `add_triple` action's subject is `$entity.triple.agent.run.entity-id` and the firing entity holds that
  predicate
- **THEN** the statement is written to the entity that predicate names
- **AND** the test that verifies this is `TestTripleActionFields`

### Requirement: An unfilled name stops the action

A built-in action SHALL fill each template it reads (a triple's `subject` and `object`, a publish's `subject` and each
top-level string property) in one read, and when a token is left unfilled SHALL fail, classified invalid, naming the
field and the token, with nothing written or published and nothing logged by the action.

#### Scenario: A value no condition guarantees

- **WHEN** an `add_triple` action's object is `$entity.triple.coordinator.decision.reason` and the firing entity has no
  such triple
- **THEN** the action fails naming `object` and the token, and no statement is written
- **AND** the test that verifies this is `TestUnfilledNameStopsAction`

### Requirement: A deny verdict is one typed error

`DenyVerdict` SHALL carry the rule ID and the reason; `errors.Is(err, ErrDenyVerdict)` SHALL hold for any error that
wraps a `*DenyVerdict`, and `errors.As` SHALL yield it with its rule ID and reason.

#### Scenario: A wrapped verdict

- **WHEN** an error wraps `&DenyVerdict{RuleID: "r", Reason: "x"}`
- **THEN** `errors.Is(err, ErrDenyVerdict)` holds and `errors.As` yields rule ID `r` and reason `x`
- **AND** the test that verifies this is `TestDenyVerdict`
