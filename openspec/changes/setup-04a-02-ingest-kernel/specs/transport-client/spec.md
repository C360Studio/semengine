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
