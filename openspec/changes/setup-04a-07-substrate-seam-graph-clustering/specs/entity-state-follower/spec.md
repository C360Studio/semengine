# entity-state-follower

## ADDED Requirements

### Requirement: An entry carries the delivered value

`Entry.Value` SHALL be the value the watch delivered for `Entry.Revision`, and nil for a delete or purge marker. It
SHALL be valid only for the duration of `Deliver`; a plug-in that keeps it copies it. A plug-in that re-reads current
truth at apply time SHALL NOT apply `Value` (ADR-108 D4); a plug-in that applies `Value` SHALL ignore a delivery at or
below the revision it last applied for that key. Delivery order is what the watch delivers: nats.go does not promise
ascending revisions across keys, so the follower checks each revision against the highest observed and declares the
feed lost on a revision at or below it, still delivering the entry with its value.

#### Scenario: Two writes to one key

- **WHEN** a key is written twice and both revisions are delivered
- **THEN** each entry's `Value` is the bytes written at its revision (`TestEntryCarriesDeliveredValue`)

#### Scenario: A revision out of order

- **WHEN** the watch delivers revision 5 and then revision 3
- **THEN** `Stuck` names the lost feed, and the second entry is delivered with revision 3's value
  (`TestFollowerDeclaresLostFeedOnOutOfOrderDelivery`, extended)

### Requirement: Stuck names unresolved failures without assuming their kind

When `Failed` returns a count above zero, `Stuck` SHALL name the count as unresolved failures and, when `Failed`
returns reasons, each reason with its count.

#### Scenario: A plug-in with reasons

- **WHEN** `Failed` returns 2 with reasons `{"refused_entity": 2}`
- **THEN** `Stuck` holds one sentence naming 2 unresolved failures and `refused_entity` 2 (`TestStuckNamesReasons`)
