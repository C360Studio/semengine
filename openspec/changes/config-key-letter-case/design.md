# Design: config-key-letter-case

base: 3a8611f plus the claim commit 8e88ca5. This is not a port: nothing is carried from the SemStreams pin, so there
is no pin probe, shape sweep or pin ADR list. The owner ruled on 2026-10-20 (#325 comment 6294481629): "Refuse it,
naming the right spelling. Yes, go."

## Premises, measured

- On main, a key in another letter case is dropped or applied silently. A probe through `LoadFromBytes` with the
  platform pair set loads each of these with no error:
  - `"NATS":{"urls":[..]}`, `"nats":{"URLs":[..]}` and `"nats":{"JetStream":{"enabled":false}}`: the defaults are
    kept;
  - `"Components":{"x":{..}}`: 0 components;
  - `"Streams":{"S":{..}}`: 1 stream, so it is applied;
  - `"services":{"heartbeat":{"Enabled":false}}`: heartbeat stays enabled.
- `types.ServiceConfig` decodes strictly (`types/service.go:22-38`), but `encoding/json` matches a case variant
  (`Enabled`) to the field, so the strict decode does not catch it (last probe above).
- The loader decodes into `Config` (`config/config.go:42-64`) and the types it reaches: `platform.Config`,
  `security.Config` and its TLS structs, `NATSConfig`, `JetStreamConfig`, `types.ServiceConfig`,
  `types.ComponentConfig`, `StreamConfig`, `*model.Registry` with `CapabilityConfig`, `EndpointConfig` and
  `DefaultsConfig`, `StreamMigrationOverride` and `ArchivalStream`. None of them has an embedded struct. None
  reaches a slice or array of structs or maps. The only custom decoder reached is `ServiceConfig`'s, which decodes
  into its own fields.
- Both loaders parse through `loadRawJSONFromBytes` (`config/config.go:402`, called at `:372` and `:479`).
- `rejectUnappliedNATSKeys` is reused on stored bucket values (`config/manager.go:1526`), wrapped under an exact
  `"nats"` key, so its case folding still matters there.

## D1. Which keys are checked, and how they stay covered

- A key is checked where the loader decodes it into a struct field. A key exactly equal to a declared name is
  accepted first, as `encoding/json` prefers an exact match. A key that differs from a declared name only in letter
  case (`strings.EqualFold`, the matching `encoding/json` uses) is refused. A key that matches no declared name at
  all is left alone, and so is a value that is not an object where a struct or map is expected (the decoder reports
  it).
- Not checked, and covered by tests:
  - user-chosen map keys: component, service, stream, override, archival, and model capability and endpoint names
    (map values are still walked);
  - `json.RawMessage` payloads: a component's or service's `config`, which its owner decodes;
  - `map[string]any`: `EndpointConfig.Options`.
- How it is built: `rejectCaseVariantKeys(raw map[string]any) error` walks the raw document beside
  `reflect.TypeOf(Config{})`.
  - It follows pointers and walks map values with the map's element type.
  - It stops at anything that is neither a struct nor a map.
  - It walks each struct's fields with `NumField()` and names them as `encoding/json` does: the tag name, else the
    Go field name; `-` and unexported fields are skipped. `Config` reaches no embedded struct, so promotion is not
    handled; the guard test below fails if one appears.
  - It visits keys in sorted order, so the first error is the same on every run.
- A new field is covered with no edit, because the names come from its tag. A guard test fails if `Config` ever
  reaches a shape the walker does not handle: a slice or array of structs or maps, an embedded struct, or a type
  with its own `json.Unmarshaler` other than `json.RawMessage` and `types.ServiceConfig` (whose decoder uses its own
  tagged fields).
- Rejected alternatives:
  - a table of key paths: it drifts, and only a reflection test could keep it honest;
  - folding keys during the merge: the owner ruled refuse;
  - doing nothing: the owner ruled against it;
  - `encoding/json/v2`, which matches names case-sensitively: at go1.26.9 it is still behind `GOEXPERIMENT=jsonv2`.
- The `config.Manager` bucket path is out of scope (the ruling).

## D2. The error, both loaders, and which error wins

- Error text: `config key <path as written> must be written <path as declared>`. For example,
  `config key nats.URLs must be written nats.urls`. User-chosen map keys show as written in both paths.
- The check runs in `loadRawJSONFromBytes` after `rejectRemovedPlatformFields` and `rejectUnappliedNATSKeys`. A
  named refusal therefore wins: `{"NATS":{"token":"x"}}` still says `nats.token ... not applied`. That is the more
  useful message, since fixing the letter case alone would still fail, and the existing tests at
  `config/load_test.go:62-63` and `:227-229` stay unchanged.
- The two named refusals keep their folding. They are not simplified, because the bucket path relies on it
  (premises).
- A document holding both `nats` and `NATS` is refused on `NATS`. A case-variant key is refused whatever its value,
  `null` included: the fault is the key.
- Keys that are not case variants stay accepted, as part 1 ruled (semteams' `tier`). Inside a `services` entry they
  stay refused by the strict decode.

## D3. Proving tests and sensitivity

- Each test runs through both loaders (`loadPaths`) and fails on main first; record how in `notes/2.1.md`.
- Refused, with the error holding both paths: `NATS`, `nats.URLs`, `nats.JetStream`, `Components`, `Streams`,
  `services.heartbeat.Enabled`, `components.x.Config`, both `nats` and `NATS` in one document, and `"NATS":null`.
- Accepted (controls):
  - unknown keys `tier` and `nats.extra`;
  - a component named `Config`, a service named `Enabled` and a stream named `Subjects`;
  - a component `config` payload holding `"Port"`, carried byte for byte.
- Coverage test: for every declared key path reachable from `Config`, with each map level filled by a placeholder
  name, the same path with that key upper-cased is refused. The paths come from the keys `json.Marshal` writes for a
  fully populated `Config` (every pointer, map and struct filled), never from the walker's naming code, so a naming
  bug in the walker cannot hide itself. The hand-written #325 cases are the second independent oracle.
- Mutants for `task mutate:check`, each detected:
  - (a) map values not walked;
  - (b) only the top level checked;
  - (c) pointers not followed (`model_registry`);
  - (d) the check run before the named refusals.

## D4. Docs

- In `config/README.md` (`:31-34`, `:121-122`) and `config/doc.go` (`:24-28`, `:44-45`), "not reliably applied" and
  "accepted and ignored" become a refusal: a key differing from a declared key only in letter case fails the load,
  naming the right spelling.
- Component and service `config` payloads belong to their owners and are not checked here.
- The README paragraph moves out of the numbered list it currently breaks.

## D5. Consumer cost

- None. The walker ran over the 28 consumer config files (top-level `platform` and `components`: semteams 25,
  semconnect 2, semboids 1, from default-branch tarballs fetched 2026-10-19 17:02, after every head's last commit).
  It refused 0, nested keys included.
- semsource does not call the loader: no `LoadFile`/`LoadFromBytes` in its non-test Go. The callers are
  `semteams/cmd/semteams/main.go:951` and `semboids/cmd/semboids/main.go:246`.

## Open issues and merge order

- This PR retires #325 (`Closes #325`, tasks 2.1-3.1).
- #340 (`config.Manager`) is unrelated.
- No other open PR touches `config/` or `config-loading`.
- New exported surface: none.

## Inventory notes

- Problem shape, admit-or-refuse at a seam: the closest existing instances are `rejectRemovedPlatformFields` and
  `rejectUnappliedNATSKeys` (`config/config.go:680-718`) and the strict decode at `types/service.go:22`. This design
  adopts their seam and their `config key ...` error style.
- Not a primitive offered for reuse: component factories decode their `config` payload strictly, and a case variant
  there is applied consistently, not dropped, because no defaults are merged in.
- Searches run: `gh pr list --state open` (only #341); `gh issue list --search config` and `--search "letter case"`
  (#325 and #340); `git grep` for the loader functions (callers `config.go:372`, `:479`, `manager.go:1526`); consumer
  `grep` for `LoadFile`/`LoadFromBytes` and capitalised key literals in Go (none besides the two callers above).
