# revision-watermark Specification

## Purpose

`internal/revlag.Watermark` tracks how far a pipeline fed by a NATS KV watch has caught up: the floor at or below
which every delivered revision has completed, the highest revision delivered, and the commit time of the oldest
delivered write not yet applied (or of the newest delivered write when none is in flight). It accepts revisions only
in ascending order and refuses any other. graph-index and graph-embedding feed their readiness envelopes from it
(pin ADR-066 section 1, ADR-083 decision 3 as amended for #408); the envelope is `graph`'s.

## Requirements

### Requirement: Completion is key-scoped and drains lower revisions

`Complete(key, rev)` SHALL drain every in-flight revision observed for `key` at or below `rev`, and SHALL leave every
other key's revisions in flight, even ones at or below `rev`. Completing at a revision never observed SHALL still
drain the key's lower in-flight revisions and SHALL NOT change `Observed`. A `Complete` that drains nothing SHALL
change no result.

#### Scenario: One completion drains collapsed revisions

- **WHEN** revisions 10, 20 and 30 are observed for one key and `Complete(key, 30)` is called
- **THEN** the floor is 30

#### Scenario: Another key's revision stays in flight

- **WHEN** revision 10 is observed for key A and 11 for key B, and `Complete(B, 11)` is called
- **THEN** the floor is 9

#### Scenario: Completing ahead of delivery

- **WHEN** revision 10 is observed for a key and `Complete(key, 40)` is called
- **THEN** the floor is 10 and `Observed` is 10

### Requirement: Observe accepts only a revision above Observed

`Observe` SHALL accept a revision only when it is above `Observed`. It SHALL refuse any other, revision 0 included,
with an error, and the refusal SHALL change no result: the revision keeps the key and commit time it had, or stays
unobserved. The floor SHALL never decrease from one call to the next.

#### Scenario: A revision observed again is refused

- **WHEN** revision 10 is observed for key A, and `Observe` is called again with revision 10 for key B
- **THEN** the second call returns an error, the floor is 9 and `Observed` is 10
- **AND** after `Complete(A, 10)` the floor is 10

#### Scenario: A lower revision is refused

- **WHEN** revision 50 is observed, and `Observe` is called with revision 20
- **THEN** the call returns an error, and `IndexedAt` returns floor 49 and revision 50's commit time

#### Scenario: Revision zero is refused

- **WHEN** `Observe` is called with revision 0 on a new watermark
- **THEN** the call returns an error, the floor and `Observed` stay 0, and the time `IndexedAt` returns stays zero

#### Scenario: A new delivery after catch-up

- **WHEN** revision 10 is observed and completed, and then revision 50 is observed
- **THEN** the floor rises from 10 to 49, and the time `IndexedAt` returns becomes revision 50's

### Requirement: Retained state tracks in-flight work

The `Watermark` SHALL keep per-revision state only for revisions in flight: once every observed revision has
completed it SHALL hold none, however many revisions it has seen, and a revision stuck in flight SHALL be the only
per-revision state it keeps while others complete above it.

#### Scenario: Completions do not accumulate

- **WHEN** 1,000,000 revisions are observed and completed one after another
- **THEN** the live heap grows by less than 1 MiB, and `IndexedAt` returns floor 1,000,000 and its commit time
- **AND** the test that verifies this is `TestWatermark_RetentionTracksInFlight`, which reads no field of the watermark

#### Scenario: One stuck revision

- **WHEN** revision 1 stays in flight while revisions 2 to 1,000,001 are observed and completed
- **THEN** the live heap grows by less than 1 MiB, the floor is 0 and the time is revision 1's
- **AND** the test that verifies this is `TestWatermark_RetentionTracksInFlight`

### Requirement: The Watermark is safe for concurrent use

Every method of `Watermark` SHALL be safe to call from many goroutines at once. A read SHALL reflect every `Observe`
and `Complete` that returned before it began, and the floor a reader sees SHALL never be lower than one it saw before.
After concurrent `Complete` calls drain every observed revision, `IndexedAt` SHALL return `Observed` and its commit
time, whatever order the completions ran in.

#### Scenario: Reads alongside observation and completion

- **WHEN** one goroutine observes revisions 1 to 1,000 and workers complete each, a reader reads at checkpoint A once
  all have returned, then revisions 1,001 to 2,000 are observed, 1,500 and 2,000 each under a key no other revision
  uses, and all but 1,500 and 2,000 complete, the reader reads at checkpoint B, then 1,500 and 2,000 complete, and a
  reader also calls `IndexedAt` and `Observed` throughout
- **THEN** checkpoint A reads floor 1,000 with revision 1,000's commit time, checkpoint B reads floor 1,499 with
  revision 1,500's commit time, no floor read is lower than an earlier one, and after all have returned `IndexedAt`
  returns floor 2,000 and revision 2,000's commit time

#### Scenario: Concurrent completion

- **WHEN** 2,000 revisions are observed and then completed from 2,000 goroutines
- **THEN** after all have returned, `IndexedAt` returns floor 2,000 and revision 2,000's commit time

### Requirement: The floor is one below the lowest in-flight revision

`internal/revlag.Watermark` SHALL report, as the first result of `IndexedAt`, a floor equal to `Observed` when no
observed revision is in flight, and otherwise one less than the lowest in-flight revision. A revision is in flight from
the `Observe` that accepts it until a `Complete` drains it. A new `Watermark` reports floor 0.

#### Scenario: A sparse build reports below its lowest pending revision

- **WHEN** revisions 50, 72 and 100 are observed for three keys and only 72 completes
- **THEN** the floor is 49

#### Scenario: Caught up skips revisions never delivered

- **WHEN** revisions 50, 72 and 100 are observed and all three complete
- **THEN** the floor is 100

#### Scenario: A new watermark

- **WHEN** nothing has been observed
- **THEN** `IndexedAt` returns floor 0 and the zero time

### Requirement: Observed is the highest revision accepted

`Observed` SHALL return the highest revision an `Observe` call accepted, whether or not it has completed, and 0 before
any.

#### Scenario: Observed runs ahead of the floor

- **WHEN** revisions 50 and 72 are observed and neither completes
- **THEN** `Observed` is 72 and the floor is 49

### Requirement: IndexedAt pairs the floor with the oldest unapplied commit time

`IndexedAt` SHALL return with the floor, read together, the commit time `Observe` accepted for the lowest in-flight
revision, or, when none is in flight, for the revision `Observed` returns. The time SHALL be zero when nothing has been
observed or that revision was observed with the zero time; zero means unknown. A completion that leaves the lowest
in-flight revision in flight SHALL NOT change the time.

#### Scenario: Exact when caught up

- **WHEN** revisions 50, 72 and 100 are observed and all complete
- **THEN** `IndexedAt` returns floor 100 and revision 100's commit time

#### Scenario: The oldest in-flight write while behind

- **WHEN** revisions 50, 72 and 100 are observed and none completes
- **THEN** `IndexedAt` returns floor 49 and revision 50's commit time

#### Scenario: A completion above an in-flight revision

- **WHEN** revisions 50, 72 and 100 are observed for three distinct keys and 72 completes
- **THEN** `IndexedAt` returns floor 49 and revision 50's commit time

#### Scenario: The time moves when the lowest drains

- **WHEN** revisions 50, 72 and 100 are observed, 72 completes, then 50 completes
- **THEN** `IndexedAt` returns floor 99 and revision 100's commit time

#### Scenario: A drain at the floor

- **WHEN** revision 10 is observed for key A and 11 for key B, and `Complete(A, 10)` is called
- **THEN** `IndexedAt` returns floor 10 and revision 11's commit time

#### Scenario: An unknown oldest time

- **WHEN** revision 7 is observed with the zero time and revision 8 with a commit time, and neither completes
- **THEN** `IndexedAt` returns floor 6 and the zero time
