# transport-client

## ADDED Requirements

### Requirement: A retried update passes the revision it read

The callback of `natsclient.KVStore.UpdateWithRetryRead` SHALL get the stored bytes and the revision read, 0 for an
absent key. A callback error matching `ErrKVSkipWrite` SHALL make that run write nothing, and the call SHALL return 0
and no error. The returned revision SHALL be the call's own commit, or 0.

#### Scenario: A skip writes nothing

- **WHEN** a key is at revision R and the update callback returns a value with an error wrapping `ErrKVSkipWrite`
- **THEN** the call returns 0 and no error after one run, and the key keeps its value at revision R

#### Scenario: A conflict passes the newer revision

- **WHEN** the callback's first run reads revision R, and the key is written at revision E before that run's value
  commits
- **THEN** the second run gets the bytes written at E and the revision E

### Requirement: An unconfirmed write ends a retried update

`natsclient.KVStore.UpdateWithRetryRead` SHALL run its callback again only after the bucket refused the previous run's
write as a revision conflict, after the write returned `jetstream.ErrNoStreamResponse`, or after its read failed. Any
other error from the create or the update SHALL end the call with that error and revision 0. `UpdateWithRetryRev`,
`UpdateWithRetry` and `UpdateJSON` SHALL behave the same.

#### Scenario: A write that stood but returned an error

- **WHEN** the callback returns a value, the bucket stores it, and the write returns an error that is neither a
  conflict nor `ErrNoStreamResponse`
- **THEN** the call returns that error (`errors.Is`) and revision 0 after one run, and the key holds the value written

#### Scenario: A write no stream answered is retried

- **WHEN** the first run's write returns `ErrNoStreamResponse` and the second run's write is stored
- **THEN** the call returns that write's revision and no error, and the key was written once
