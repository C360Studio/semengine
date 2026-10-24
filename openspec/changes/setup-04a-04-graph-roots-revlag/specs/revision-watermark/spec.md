# revision-watermark

## ADDED Requirements

### Requirement: The floor is the low-water mark of in-flight revisions

`internal/revlag.Watermark` SHALL report, as the first result of `IndexedAt`, a floor equal to the highest revision
passed to `Observe` when no observed revision is in flight, and otherwise one less than the lowest in-flight revision,
whatever order the revisions were observed in. A revision is in flight from its `Observe` until a `Complete` drains
it. A new `Watermark` reports floor 0.

#### Scenario: A sparse build reports below its lowest pending revision

- **WHEN** revisions 50, 72 and 100 are observed for three keys and only 72 completes
- **THEN** the floor is 49

#### Scenario: Caught up skips revisions never delivered

- **WHEN** revisions 50, 72 and 100 are observed and all three complete
- **THEN** the floor is 100

#### Scenario: A new watermark

- **WHEN** nothing has been observed
- **THEN** `IndexedAt` returns floor 0 and the zero time

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

### Requirement: Observed is the highest revision delivered

`Observed` SHALL return the highest revision passed to `Observe`, whether or not it has completed. `Observe` SHALL
ignore revision 0, which changes no result.

#### Scenario: Observed runs ahead of the floor

- **WHEN** revisions 50 and 72 are observed and neither completes
- **THEN** `Observed` is 72 and the floor is 49

#### Scenario: Revision zero is ignored

- **WHEN** `Observe` is called with revision 0
- **THEN** the floor and `Observed` stay 0 and the time `IndexedAt` returns stays zero

### Requirement: The floor never falls under ascending observation

When every `Observe` call passes a revision higher than any observed before, the floor SHALL never decrease from one
call to the next. Keeping `Observe` calls in ascending revision order is the caller's obligation: the watermark does
not check it, and makes no promise about the commit time for a caller that breaks it.

#### Scenario: A new delivery after catch-up

- **WHEN** revision 10 is observed and completed, and then revision 50 is observed
- **THEN** the floor rises from 10 to 49, and the time `IndexedAt` returns stays revision 10's

### Requirement: IndexedAt pairs the floor with a covered commit time

Under ascending observation, `IndexedAt` SHALL return, read together with the floor, the commit time passed to
`Observe` for a completed observed revision at or below the floor, never one above it. When no revision is in flight
it SHALL be the highest observed revision's time. It SHALL be the zero time when no observed revision is at or below
the floor or that revision's time was zero; zero means unknown.

#### Scenario: Exact when caught up

- **WHEN** revisions 50, 72 and 100 are observed and all complete
- **THEN** `IndexedAt` returns floor 100 and revision 100's commit time

#### Scenario: Older, never newer, while behind

- **WHEN** revisions 50, 72 and 100 are observed, 72 completes, then 50 completes
- **THEN** `IndexedAt` returns floor 99 and revision 50's commit time

#### Scenario: A drain at the floor is covered

- **WHEN** revision 10 is observed for key A and 11 for key B, and `Complete(A, 10)` is called
- **THEN** `IndexedAt` returns floor 10 and revision 10's commit time

#### Scenario: No covered revision

- **WHEN** revisions 50 and 72 are observed and none completes
- **THEN** the time `IndexedAt` returns is zero

### Requirement: Retained state tracks in-flight work

The `Watermark` SHALL keep per-revision state only for revisions in flight: once every observed revision has
completed it SHALL hold none, however many revisions it has seen, and a revision stuck in flight SHALL be the only
per-revision state it keeps while others complete above it.

#### Scenario: Completions do not accumulate

- **WHEN** 20,000 revisions are observed and completed one after another
- **THEN** the watermark holds no per-revision state, and `IndexedAt` returns floor 20,000

#### Scenario: One stuck revision

- **WHEN** revision 1 stays in flight while revisions 2 to 20,000 are observed and completed
- **THEN** the watermark holds per-revision state for revision 1 only, and the floor is 0

### Requirement: The Watermark is safe for concurrent use

Every method of `Watermark` SHALL be safe to call from many goroutines at once. After concurrent `Complete` calls drain
every observed revision, `IndexedAt` SHALL return the highest observed revision and its commit time, whatever order
the completions ran in.

#### Scenario: Concurrent completion

- **WHEN** 2,000 revisions are observed and then completed from 2,000 goroutines
- **THEN** after all have returned, `IndexedAt` returns floor 2,000 and revision 2,000's commit time
