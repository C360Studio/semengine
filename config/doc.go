// Package config reads a SemEngine process's configuration and declares the
// JetStream streams it needs, and its Manager keeps that configuration in a
// NATS key-value bucket across restarts.
//
// # Loading
//
// A Loader reads one JSON document, lays it over built-in defaults, applies
// the environment overrides, and checks that platform.org and platform.id leave
// room for the entity IDs the deployment will create (ADR-104). It does not
// call Validate; the caller does:
//
//	cfg, err := config.NewLoader().LoadFile("config.json")
//	if err != nil {
//		return err
//	}
//	if err := cfg.Validate(); err != nil {
//		return err
//	}
//
// LoadFromBytes does the same for a document already in memory. A key the
// document sets replaces the default, including false, 0 or an empty list; a
// key it leaves out keeps the default; objects merge key by key.
//
// Write every key in lowercase, as the README shows it. A key in another
// letter case is not reliably applied: where the defaults carry that key it is
// silently ignored, so "Components", nats "URLs" or nats "JetStream" leave the
// defaults in place (#325). The refused keys below are the exception: they are
// refused in any letter case.
//
// Loading fails, returning no Config, when the document:
//   - is not exactly one JSON object (null is refused too), is larger than
//     10 MB, or nests deeper than 100 levels;
//   - has a value that does not fit its field, such as a string where a list
//     is expected, or an unknown key inside a services entry (the error
//     carries the decoder's message);
//   - sets platform.instance_id, which was removed (ADR-102);
//   - sets a NATS setting nothing in SemEngine applies yet: nats.username,
//     nats.password, nats.token, nats.tls, nats.max_reconnects,
//     nats.reconnect_wait, nats.jetstream.domain,
//     nats.jetstream.retention_policy or nats.jetstream.replication_factor.
//     These are refused when present, whatever their value and in any letter
//     case, so a setting an operator wrote is never silently ignored (#74).
//
// Other unknown keys are accepted and ignored, as are keys in another letter
// case where the defaults carry that key (#325).
//
// # Environment overrides
//
// When set and not empty, SEMENGINE_PLATFORM_ID, SEMENGINE_PLATFORM_TYPE,
// SEMENGINE_PLATFORM_REGION and SEMENGINE_NATS_URLS replace the document's
// platform.id, platform.type, platform.region and nats.urls. The URL list is
// split at commas and spaces are not trimmed, so write a,b, not a, b.
// Loading fails when SEMENGINE_NATS_USERNAME, SEMENGINE_NATS_PASSWORD or
// SEMENGINE_NATS_TOKEN is set and not empty, naming the variable: those
// credentials are not applied yet.
//
// # Which files LoadFile reads
//
// The path must end in .json or .json5; the content is read as plain JSON
// either way. A relative path must stay inside the current working directory
// once . and .. are resolved. An absolute path is not confined to any
// directory, but is refused if a name in it contains "..". Symbolic links are
// followed, so a link to a regular file is read wherever that file is. A
// directory or device is refused.
//
// # Sharing a configuration
//
// SafeConfig wraps a Config for concurrent use. Get returns a deep copy.
// Mutate holds the write lock from the read to the swap, so use it, not Get
// followed by Update, to change part of a configuration another goroutine may
// also change.
//
// # The configuration manager
//
// Manager keeps a deployment's configuration in a NATS key-value bucket, the
// configuration bucket, so it outlives the process and another process can
// change what the next boot runs. BucketName names the bucket from the
// document's platform.org and declared platform.id:
// semengine_config_<org>_<stem>.
//
// Start(ctx) does its startup work under ctx: it acquires the bucket,
// establishes the deployment's platform identity from the bucket's identity
// record (ADR-104), decides whether the file or the bucket wins, and opens the
// watches. It refuses a nil or ended ctx, and refuses every Start after the
// first with errs.ErrAlreadyStarted. Ending ctx after Start returned stops
// nothing (ADR-113): the watches run until Stop.
//
// Stop(ctx) closes every channel the Manager handed out, stops every watch and
// waits for the Manager's goroutines. It returns nil only once all of that is
// done. If ctx ends first it returns ctx's error, and a later Stop tries again.
// A ctx that has already ended stops nothing, so a caller on its shutdown path
// passes a fresh bounded context, not its ended signal context:
//
//	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
//	defer cancel()
//	err := mgr.Stop(stopCtx)
//
// Which copy wins at Start: on a first boot, and when the file's version is
// newer than the bucket's version key (CompareVersions), Start writes the file
// to the bucket. Otherwise (equal versions, an older file, or either version
// not a semantic version, a file with no version among them) the bucket wins:
// its services and components replace the file's, so one deleted at run time
// stays deleted. A missing version key, and a stored version that fails to
// decode into a string, such as a number (logged as a warning), compare as
// 0.0.0, so a file at 1.0.0 wins; a stored string that is not a semantic
// version selects the bucket, and so does a stored null, which decodes as an
// empty string. A stored model_registry replaces the file's; a stored nats is
// decoded over the file's nats section, so a key it leaves out keeps the
// file's value. The stored platform key is a copy for readers and is never
// applied. Start writes the version key last, so a write cut short leaves the
// old version, or none; the next boot writes again only when the file's
// version is newer than that. A file with no version, or 0.0.0, is not, and
// selects the partly written bucket.
//
// Each service and component name must be one NATS KV key token once its
// spaces are written as '_': no '.', '*' or '>'. A name with a space is kept
// under its key's name from Start on, so GetConfig reports a file component
// "a b" as "a_b", and two names that map to one key are refused.
//
// Start fails closed. It returns an error and leaves every writer refusing
// when a name or value cannot be written, the bucket cannot
// be listed or read, a stored services, components, nats or model_registry
// entry does not decode, a stored nats entry sets a key loading refuses, the
// write to the bucket fails, or a watch does not open. The error names the key.
// A refused name or value is found before the bucket is touched, so nothing is
// written, not even the identity record. A failed Start rolls back what it
// acquired before returning; if that rollback fails, its error is joined into
// Start's and the rest of the cleanup is kept for Stop, so a caller installs
// its bounded Stop before calling Start.
//
// PutComponentToKV and DeleteComponentFromKV change the components the next
// boot runs. They update GetConfig before they return; a put whose key the
// watcher has already applied at a newer revision keeps that newer value, and
// no watcher event is applied between a delete reaching the bucket and its
// removal from GetConfig, so a delete never erases a later put. They
// never start, stop or restart a running component. WatchModelRegistry delivers the model
// registry each time its key changes.
//
// A key family (NewKeyFamily, WithKeyFamily) hands another owner the keys
// "<prefix>.<name>" in the same bucket: its handler receives the entries
// present when Start opened its watch, then each put and delete, and its Get,
// Put, Create, Delete and Names reach only those keys. The Manager never
// decodes a family's values. A prefix the Manager uses itself (services,
// components, platform, nats, model_registry, platform_identity) is refused.
//
// # Streams
//
// StreamsManager.EnsureStreams creates or repairs the JetStream streams a
// Config needs: the framework's own, those in its streams section, and those
// its components' JetStream output ports imply. Every stream must declare a
// finite max_age, a max_bytes above zero and a discard policy, unless it is
// declared archival, or it already exists and has an unexpired migration
// override. Config.Validate checks those declarations without a server
// (ValidateStreamDeclarations).
//
// The package README gives an example document and the rules in full.
package config
