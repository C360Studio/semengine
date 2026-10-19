// Package config reads a SemEngine process's configuration and declares the
// JetStream streams it needs.
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
