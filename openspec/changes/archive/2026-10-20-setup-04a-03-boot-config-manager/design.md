# Design: setup-04a-03-boot-config-manager

This design depends on `INVENTORY PASS` for `inventory.md`. Its spec delta is `specs/config-manager/spec.md`. Pin
paths are at `8b99efe9`; repository lines are at `c0cabb7`. Size: 1,697 carried pin lines, under the ~3,000 cap; no
waiver. It ports `manager.go`, `key_family.go` and `bucket_name.go` into `config` with their tests, completing the
package (#159). `service` ports after it.

**Decision records.**

- ADR-104 and ADR-102 decision 7 bind the mint, adopt and refuse paths. Kept unchanged. Task 3.1 rewrites ADR-104's
  port note (`docs/adr/104-unique-platform-authority.md:12-13`): the mechanism now lives in this repo's
  `config.Manager.Start`, and the bucket here is `semengine_config_<org>_<stem>`.
- ADR-094's next-boot rule binds `PutComponentToKV`/`DeleteComponentFromKV`. Kept. Its "Terminal shutdown remains
  Start-owned" is superseded by ADR-113 (D2). ADR-094 is not copied: its subject is `service`.
- ADR-095's one Start per instance is kept, and enforced for the first time (D2).
- ADR-113 governs D2. The Manager has `Start(ctx)`/`Stop(ctx)` after this change, which makes it a lifecycle owner
  in ADR-113's sense. Q1 asks the owner to confirm.

## D1. Port and drop (required)

- **Ported:** the three files with the module path and the four renames (inventory, slice). No new import or
  dependency.
- **Unexported (surface audit a, M10):** `OnChange` becomes `onChange`, `Update` becomes `update`, and `PushToKV` is
  dropped (Start keeps `pushToKV`). There is no reader outside `config` at the pin or in the four consumers.
- **Kept, with contract lines:** `NewConfigManager`, `ManagerOption`, `WithKeyFamily`, `Start`, `Stop`, `GetConfig`,
  `WatchModelRegistry` (`model.Watcher`), `PutComponentToKV`, `DeleteComponentFromKV`, `BucketName`, `KeyFamily`
  and its methods, `KeyFamilyEntry`, `KeyFamilyOperation`. Each has a requirement in the spec delta and a test that
  calls it as semsource or the rule manager does.
- **Options:**
  - (a) Chosen.
  - (b) Keep `OnChange`/`PushToKV` exported: surface with no reader and no contract line (#186, #190).
  - (c) Do nothing: `service` cannot build.

## D2. Lifecycle: Start bounds Start, Stop(ctx) stops (M1-M5; ADR-113; Q1)

Observable behaviour, each proven by `TestManagerPassesTheLifecycleSuite` (`lifecycletest.Run` through a test-side
adapter) unless another test is named. As in graph-ingest's adapter
(`processor/graph-ingest/lifecycle_adapter_test.go:61-105`), `Observe` reads only state production code uses:

- the Manager's watcher slice;
- the subscriber map;
- each done channel `Stop` waits on, listed while it is open.

`Calls` is left empty: the suite compares whole observations around a call (`lifecycletest.go:243-294`), so a
count the call itself bumps would always differ (task 2.1 finding). Watcher stops are counted on the server,
through the KV backing stream's consumer count, never through a counter, hook or field added for tests
(#186, #190). This rests on nats.go v1.54.0 behaviour (ADR-112, #170):

- a KV watch is an ordered consumer created to be deleted on unsubscribe (`js.go:2026`);
- `KeyWatcher.Stop` unsubscribes (`jetstream/kv.go:1239`);
- a successful `Unsubscribe` deletes that consumer before it returns (`nats.go:5409-5411`).

Two caveats:

- An ordered-consumer reset deletes the old consumer in the background (`js.go:2390`, `:2425`). The adapter
  therefore polls the count with `probe.Await` under a bound, and never reads it once.
- The pin ignores a watcher's stop error (`manager.go:509`). The port keeps that and logs the error at WARN; `Stop`
  does not return it (owner ruling Q4, #9 comment 6278986482). Returning it would make every later `Stop` fail after
  one failed unsubscribe during a broker outage, because a second `Unsubscribe` returns `ErrBadSubscription`
  (`nats.go:5400-5404`). A watcher whose unsubscribe failed leaves its consumer on the server, so the count still
  shows it.

- `Start(ctx)` does its startup work under `ctx`: acquire, identity, arbitration, opening the watches. It refuses
  nil, an ended context and a second call (`errs.ErrAlreadyStarted`). A refused or failed Start holds nothing and
  arms no writer (`TestRefusedStartDisarmsEveryExportedWriter`).
- Ending Start's context after Start returned stops nothing. The suite's Start-context check proves the handles are
  still held. `TestStartContextEndingKeepsDelivering` cancels Start's context, then puts `services.x`, and requires
  the subscriber to receive it (P2 is the failing case).
- `Stop(ctx)` returns nil only after every watch is stopped, every Manager goroutine has exited and every subscriber
  channel is closed. If `ctx` ends first it returns that error, and a later Stop tries again. A `ctx` that has
  already ended stops nothing: Stop returns its error at once (`internal/lifecycleguard/guard.go:108-110`). A caller
  on its shutdown path therefore passes a fresh bounded context, for example
  `context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)`, not its ended signal context.
- A live `WatchModelRegistry` subscriber does not hold Stop: `TestStopWithAModelRegistryWatcherReturnsPromptly`
  starts `model.Watch` on the Manager, then requires Stop to return nil under a 1 s context and the watch channel to
  be closed (P1 is the failing case).
- `onChange` and `WatchModelRegistry` after Stop return a closed channel and start nothing.
- A key-family handler that blocks makes Stop return its context's error; Stop succeeds once the handler returns.

The mechanism is the developer's. The closest instance is graph-ingest (`lifecycleguard.Guard` plus a run context
created in Start and cancelled in Stop). Recommended: compose `internal/lifecycleguard` and drop
`shutdownCh`/`stopped`. Alternative: keep the pin's fields and add the checks by hand, the copied shell M5 names.

**Q1 (owner): confirm.** Consumer cost: semsource `run.go:163`, semteams `main.go:149` and semboids `main.go:155`
change `Stop(5*time.Second)` to `Stop` with a fresh bounded context. The compile error tells them to change it, but
not how. Passing the signal context they already hold, which has ended by shutdown, makes every Stop return
`context.Canceled` and release nothing. semsource joins that error into `runErr`, so it would report a failure on
every shutdown. Doc comment and README say so, with the example above. Ending the Start context no longer stops the
Manager; all three call Stop on exit already.

## D3. A deleted component stays deleted (M8, M9, #17; Q2)

- **Recommended:** when arbitration selects the bucket (equal or older file version, or an unparseable stored
  version), the effective `Components` are the bucket's `components.*` entries, as `Services` already
  are at the pin (`manager.go:1327`). This is the smallest fix that mirrors what the code already does.
- Service and component names must each be one NATS KV literal token after the space mapping
  (`natsclient.ValidateKVLiteralToken`, as `KeyFamily.key` uses). Otherwise a name is written as a key the watches
  and the sync skip (pin `:1336-1339`), and vanishes at the next bucket-selected boot. Services already do this at
  the pin, since `syncFromKV` replaces their set (`:1327`). `Start` checks every name in the file configuration
  before it acquires the bucket. A refused name fails Start naming the service or component, and nothing is written,
  not even the identity record. `PutComponentToKV` and `DeleteComponentFromKV` check their name before any write.
- `Start` encodes every value the push would write before it acquires the bucket, with the name check, so a value
  that cannot be encoded writes nothing, not even the identity record.
- `pushToKV` writes `version` last; the pin writes it first (`manager.go:882-894`). A push interrupted after its
  first write leaves the bucket's old `version`, or none (`0.0.0`). The next boot's newer file then wins and pushes
  again, instead of selecting a partial component set. Limit, retained: a file with no `version`, or `0.0.0`,
  compares equal and is not protected.
- **Proof:**
  - `TestDeletedFileComponentStaysDeletedAtNextBoot` is P4 as a test: fails at the base, passes after.
  - `TestConfigManagerKVSelectionReplacesOnlyServices` becomes `TestKVSelectionReplacesServicesAndComponents`.
  - `TestNameThatIsNotOneKeyTokenIsRefused`: a service `a.b` and a component `a.b` in the file each fail `Start`
    with the bucket left as it was (no identity record), and `PutComponentToKV("a.b", ...)` fails with no write.
  - `TestRecoveryFromAnInterruptedPush` proves recovery from the leftover state such a push can leave. It does not
    drive the failure itself, which would need fault injection. It seeds the bucket through the KV API with the
    identity record and some `services.*`/`components.*` keys, in two cases: no `version`, and an older `version`.
    It then starts with the file and requires the effective services and components to be the file's full set, and
    the bucket to hold the full push with the file's `version`.
  - `TestPushWritesVersionLast`: after a successful push, the revision of `version` is greater than the revision of
    every other key the push wrote.
  - #17 asks for its reproduction under `internal/harness`. It lives in `config/` beside the code it proves, because
    it needs the package's shared NATS fixture and its unexported test helpers.
- **Cost:**
  - A component added to the file without a version bump no longer appears while the bucket holds the same
    version, the rule services follow today.
  - semsource's `Enabled:false` workaround keeps working.
  - 0 of 156 service names in the 28 consumer config files, and none of semsource's four built-in service names
    (`cmd/semsource/run.go:1104-1122`: `service-manager`, `component-manager`, `metrics`, `heartbeat`), is refused.
  - 0 of 227 component names in the 28 consumer config files are refused. semsource's runtime names all pass
    `entityid.SystemSlug` (`internal/sourcespawn/components.go:83-103`), which yields one token.
- **Alternatives:**
  - Replay tombstones: the bucket keeps History 5, but a purge or compaction loses them, and it is more code.
  - Keep the pin: #17 stays open as a required correction, since its own body makes it an admission gate.
- Retires #17 (`Closes #17`, task 2.3) if the owner says yes.

## D4. Start fails closed (M6, M7; Q3)

- **Recommended:** Start returns the error, holds nothing and arms no writer when:
  - the first-boot push fails, or the push when the file's version is newer fails (pin `manager.go:385-387` only
    logs it);
  - arbitration cannot list or read the bucket;
  - a stored `services.*`, `components.*`, `nats` or `model_registry` entry does not decode;
  - the stored `version` read fails for any reason other than not-found (a missing key stays `0.0.0`, the pin's
    rule for old buckets);
  - a stored `nats` value sets a key `config-loading` refuses: `rejectUnappliedNATSKeys` (`config/config.go:704`)
    runs on the value wrapped as `{"nats": ...}`, letter case folded.
  - any one of the Manager's watches does not open (the pin failed only when none opened, and skipped a failed one
    with a Debug log, so its updates never arrived; recorded under the repo's fail-closed rule, task 2.2 finding).

  The error names the key.
- An unparseable stored `version` keeps the pin's rule (select the bucket, Warn), since nothing is lost by it.
- **Proof:**
  - `TestMalformedStoredServiceFailsStart` (P5);
  - `TestStoredNATSTokenFailsStart`;
  - `TestPushEncodingFailureFailsStart`, in a first-boot case and a newer-file case. It is driven by a service
    whose `config` is `json.RawMessage("{")`, which `encoding/json` refuses to marshal; the test is the probe. (A
    component with that config is refused earlier, when Start validates the configuration for the identity, so it
    never reaches the push; task 2.1 finding.) Start fails naming the service, and the bucket holds no
    `services.*`, `components.*` or `version` key the push
    would have written. A push failing between two writes, from a broker fault, has no deterministic driver without
    fault injection or a test-only hook. It is not tested; its recovery is `TestRecoveryFromAnInterruptedPush` (D3);
  - `TestVersionReadErrorFailsStart` is not written: it needs a mid-operation fault (#185), and the branch is one
    `errors.Is`.
- **Cost:** a bucket holding a broken entry, or a push that fails, stops boot until an operator fixes the key or the
  file. The message names the key. A push that fails after its first write leaves those keys. The next boot
  recovers them because `version` is written last (D3). Runtime writes after Start keep logging and skipping
  (`handleUpdate`, optional).
- **Alternative:** keep the pin, which leaves P5's silent service loss.

## D5. Tests

- **Carried and adapted:** all eight pin test files. Changes:
  - `fixtureClient` replaces the 32 `NewTestClient` calls (#195 for `config`);
  - `semengine_config` literals;
  - `Stop(ctx)` under a bounded context (cleanup-roots guard);
  - the four `Skip` calls removed, since tagged files need none;
  - the four sleeps replaced by waits on the subscriber channel or `probe.Await`;
  - `nats://localhost:4222` replaced by a host the guard accepts (part 1's `nats://a.invalid:14222`);
  - the two-boot test split into subtests;
  - the #17 test flipped (D3);
  - the nil-context test checks the guard, not `shutdownCh`.
- **Not carried:** `example_test.go`'s Manager examples (no oracle, names that do not exist).
- **New:** D2's suite adapter and the model-watcher Stop test; D3's three tests; D4's three tests.
- **Sensitivity:** `task mutate:check` cannot run integration tests (#129). Task 2.4 applies each mutant by hand,
  runs the named test with `-tags integration`, and records the failure and the revert in `notes/2.4.md`. The
  mutants:
  - run context derived from Start's context;
  - forwarder not selecting on the stop signal;
  - `Components` not reset in `syncFromKV`;
  - the token check removed;
  - `syncFromKV` skipping a decode error again;
  - the second-Start refusal removed.
- Examples suffice for these checks: each rule is one branch or a fixed list. The identity paths keep the pin's
  property test (`TestBucketNameIsLegalForEveryValidPair`, `FuzzBucketName`).
- **Coverage:** add `"config merged"` to `scripts/cover-check.sh`. The scratch copy measured 83.7% before the
  changes.

## D6. Ledger, text, guidance

- **Ledger:** no new rows. The three files are `adapt`: renames everywhere, and D2 touches `processFamily` in
  `key_family.go`. The `config` row is edited in place (no end-of-file conflict):
  - `consumer_purpose` adds "and config.Manager (Start/Stop, BucketName, PutComponentToKV, DeleteComponentFromKV,
    key families) read by service, boot, processor/rule and semsource, semteams, semboids".
  - `contract` adds "the config-manager spec; ADR-104 and ADR-102 decision 7 as at the pin; ADR-113".
  - `dependencies_and_side_effects` adds graph and graph/kvcatalog, the configuration bucket, and "watch goroutines
    joined in Stop".
  - `known_risks` adds: OnChange/Update unexported and PushToKV dropped; Stop takes a context and Start is one-shot
    (Q1); KV selection replaces components and refuses names that are not one key token (Q2, #17); Start fails
    closed on bucket I/O and decode, and a stored nats value is refused like loading (Q3); retained: wildcard
    matching unlike NATS, runtime writes log and skip, unprobed nats.go comments. The "part 2" sentence goes.
  - `owner` becomes this change's developer.
- **Text:** `config/doc.go` and `config/README.md` gain a Manager section that compiles against this surface: Start,
  then Stop with a context, the version rule, and next-boot writes. `docs/repository-map.md`'s `config/` row drops
  "follows in part 2". The ADR-104 port note is updated (Decision records). The kvcatalog and graph comments naming
  `config.BucketName` become true and are left as they are.
- **Guidance:** no pin contract section or skill names the Manager (`grep` over the pin's `.agents`, `.claude`,
  `AGENTS.md`). `kv-or-stream` and `entity-or-bucket` are already here. No `GenericJSON`.

**Issues:**

- #17: retired if Q2 is yes.
- #195: stays open for `service` and the other packages. This PR finishes `config`.
- #325: not touched. The KV path matches keys exactly and decodes values typed (inventory § 3). It stays open for its
  own loader PR.
- #196: no change. The Manager adopts the guard without being on its list. A comment on #196 records it.

**Merge order:** independent of #327; whichever merges second keeps both ledger and map edits. `service`
(`ComponentManager`) follows this PR.

## Owner questions

All answered on 2026-10-19 (#9 comment 6278986482): Q1, Q2 and Q3 yes; the Stop-error question (Q4, the open
MEDIUM after three review rounds) kept as at the pin. The design is accepted.

- **Q1 (D2).** Should `Stop` take a context, `Start` refuse a second call, and the Manager keep running when the
  context passed to `Start` ends, as ADR-113 says for components? Cost: one line in each of semsource, semteams and
  semboids, which must hand `Stop` a fresh bounded context (an already-ended one stops nothing). Recommended: yes.
- **Q2 (D3).** When the bucket's copy wins at boot, should its components replace the file's, as its services
  already do, so a deleted component stays deleted (#17)? Service and component names that NATS cannot hold as one
  key (a dot, `*` or `>`) would be refused when `Start` begins and by the component writes, because such a name is
  written as a key the boot never reads back, and a service named that way already vanishes at the pin today. Cost:
  a component added to the file needs a version bump to appear, as a service does today. No name in the consumer
  config files (227 components, 156 services) and no semsource runtime name is refused. Recommended: yes.
- **Q3 (D4).** Should Start fail with the real error when it cannot write the file's configuration to the bucket (on
  a first boot or when the file is newer), read the bucket, or decode a stored entry, including a stored `nats`
  setting that loading refuses, instead of logging and continuing? Cost: a broken stored entry or a failed push
  stops boot until it is fixed. Today the entry silently disappears or the push is lost. Recommended: yes.
