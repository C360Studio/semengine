# transport-client

## REMOVED Requirements

### Requirement: The storage report view changes at a sync marker

**Reason**: The storage report consumer moves out of `natsclient` into `natsclient/storagereport` (#222 part a). The
requirement is kept, with the account retraction now told to observers (#85), as the ADDED requirement of the same name
in the `storage-report` capability.

**Migration**: Import `github.com/c360studio/semengine/natsclient/storagereport`; `StorageReportConsumer` is
`storagereport.Consumer` and `StorageReportObserver` is `storagereport.Observer`, which adds `ForgetAccount`.
