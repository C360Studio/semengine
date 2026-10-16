# storage-report

## ADDED Requirements

### Requirement: The storage report view changes at a sync marker

The storage report consumer (`natsclient/storagereport.Consumer`) SHALL collect a watch's initial values until the
watch's sync marker and then make them its view in one step, so a row or account row the bucket no longer holds is gone
even when no delete marker for it was replayed. Until that marker a `Snapshot` caller SHALL see the previous view
whole; before the first watch's marker the view is empty and `Synced` is false. `Snapshot` SHALL share no mutable memory
with the consumer. When the consumer holds an account row and the account key is deleted or purged, or a replacement
watch's initial values lack it, the consumer SHALL clear `AccountKnown` and call the observer's `ForgetAccount` once
with the account row it held (changed behaviour: the observer was not told, #85). It SHALL make no `ForgetAccount` call
for an account it does not hold, nor for an account value it cannot decode, which keeps the held row.

#### Scenario: A replacement watch over a bucket that lost rows

- **WHEN** a replacement watch's initial values lack a row the previous view held, and no delete marker for it is
  replayed
- **THEN** `Snapshot` returns the previous view until the sync marker, and from the marker on the row is gone

#### Scenario: The account key is deleted

- **WHEN** the consumer holds an account row and the account key is deleted or purged
- **THEN** `Snapshot` reports `AccountKnown` false and the observer's `ForgetAccount` is called once with that row

#### Scenario: A replacement watch without the account

- **WHEN** the consumer holds an account row and a replacement watch's initial values lack the account key
- **THEN** `ForgetAccount` is called once, at the sync marker, with the held row

#### Scenario: A delete of an account never held

- **WHEN** the account key is deleted and the consumer holds no account row
- **THEN** no observer method is called

#### Scenario: Editing a snapshot

- **WHEN** a caller edits any slice or pointer field of a returned snapshot
- **THEN** the next `Snapshot` is unchanged

### Requirement: Collect and Publish refuse a nil context

`StorageInventoryCollector.Collect` and `Publisher.Publish` SHALL refuse a nil context with an invalid-data error.

#### Scenario: Nil context

- **WHEN** `Collect` or `Publish` is given a nil context
- **THEN** it returns an invalid-data error
