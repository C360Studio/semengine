# transport-client Delta

## MODIFIED Requirements

### Requirement: Only Close changes the status once Close begins

While a connection is installed and Close has not begun, a Connect that does not install its own connection SHALL NOT
change `Status()`. Once Close has begun, no writer but Close SHALL change `Status()`. It keeps its value through the
drain and is `Disconnected` for good once Close's cleanup has finished.

#### Scenario: Only Close changes the status once Close begins

- **WHEN** Close has begun and a connection handler or a Connect would change the status
- **THEN** Status keeps the value it had when Close began until Close's cleanup has finished, and reports
  Disconnected from then on

#### Scenario: A losing Connect leaves the winner's status

- **WHEN** two Connect calls overlap, one installs its connection and the other fails or is cancelled
- **THEN** Status is that of the installed connection

### Requirement: A handler error never arrives as reply data

`RequestClassified` and `RequestWithRetryClassified` SHALL return a reply that carries `X-Status: error` as an error
and no data: a `*errs.ClassifiedError` with the reply's class, code, message and detail.

#### Scenario: A classified call gets the handler's error

- **WHEN** a `SubscribeForRequests` handler returns an error with class invalid and a code, and the caller uses
  `RequestClassified`
- **THEN** the call returns no data and an error for which `errs.IsInvalid` reports true and whose `Code` is the
  handler's code

### Requirement: A failed call refuses no other call

No natsclient call SHALL refuse because other calls failed. A call that fails SHALL return its own error and SHALL leave
`Status()` as `Connect`, the connection's events and `Close` set it; no periodic check of its own sets it.

#### Scenario: Failures on one subject leave other calls working

- **WHEN** a connected client makes twenty `PublishToStream` calls to a subject no stream captures, and each fails
- **THEN** each error is that publish's own failure, a `PublishToStream` to a subject a stream captures then
  succeeds, and `Status()` reports `Connected`
