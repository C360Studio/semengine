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
adapter whose `Observe` lists each open watcher, each running goroutine and each open subscriber channel, and counts
watcher stops) unless another test is named:

- `Start(ctx)` does its startup work under `ctx`: acquire, identity, arbitration, opening the watches. It refuses
  nil, an ended context and a second call (`errs.ErrAlreadyStarted`). A refused or failed Start holds nothing and
  arms no writer (`TestRefusedStartDisarmsEveryExportedWriter`).
- Ending Start's context after Start returned stops nothing: watches keep delivering (the suite's Start-context
  check; P2 is the failing case).
- `Stop(ctx)` returns nil only after every watch is stopped, every Manager goroutine has exited and every subscriber
  channel is closed. If `ctx` ends first it returns that error, and a later Stop tries again.
- A live `WatchModelRegistry` subscriber does not hold Stop: `TestStopWithAModelRegistryWatcherReturnsPromptly`
  starts `model.Watch` on the Manager, then requires Stop to return nil under a 1 s context and the watch channel to
  be closed (P1 is the failing case).
- `onChange` and `WatchModelRegistry` after Stop return a closed channel and start nothing.
- A key-family handler that blocks makes Stop return its context's error; Stop succeeds once the handler returns.

The mechanism is the developer's. The closest instance is graph-ingest (`lifecycleguard.Guard` plus a run context
created in Start and cancelled in Stop). Recommended: compose `internal/lifecycleguard` and drop
`shutdownCh`/`stopped`. Alternative: keep the pin's fields and add the checks by hand, the copied shell M5 names.

**Q1 (owner): confirm.** Consumer cost: semsource `run.go:163`, semteams `main.go:149`, semboids `main.go:155` change
`Stop(5*time.Second)` to `Stop(ctx)`. The compile error tells them. Ending the Start context no longer stops the
Manager; all three call Stop on exit already.

## D3. A deleted component stays deleted (M8, M9, #17; Q2)

- **Recommended:** when arbitration selects the bucket (equal or older file version, or an unreadable or
  unparseable version), the effective `Components` are the bucket's `components.*` entries, as `Services` already
  are at the pin (`manager.go:1327`). This is the smallest fix that mirrors what the code already does.
- `pushToKV`, `PutComponentToKV` and `DeleteComponentFromKV` refuse a component name that is not one NATS KV literal
  token after the space mapping (`natsclient.ValidateKVLiteralToken`, as `KeyFamily.key` uses), naming the
  component. A dotted name would otherwise be written as a key the watches and the sync skip, and would vanish at
  the next KV-selected boot.
- **Proof:**
  - `TestDeletedFileComponentStaysDeletedAtNextBoot` is P4 as a test: fails at the base, passes after.
  - `TestConfigManagerKVSelectionReplacesOnlyServices` becomes `TestKVSelectionReplacesServicesAndComponents`.
  - `TestComponentNameThatIsNotOneKeyTokenIsRefused`.
- **Cost:**
  - A component added to the file without a version bump no longer appears while the bucket holds the same
    version, the rule services follow today.
  - semsource's `Enabled:false` workaround keeps working.
  - 0 of 227 consumer component names are refused.
- **Alternatives:**
  - Replay tombstones: the bucket keeps History 5, but a purge or compaction loses them, and it is more code.
  - Keep the pin: #17 stays open as a required correction, since its own body makes it an admission gate.
- Retires #17 (`Closes #17`, task 2.3) if the owner says yes.

## D4. Start fails closed (M6, M7; Q3)

- **Recommended:** Start returns the error, holds nothing and arms no writer when:
  - the first-boot push fails;
  - arbitration cannot list or read the bucket;
  - a stored `services.*`, `components.*`, `nats` or `model_registry` entry does not decode;
  - the stored `version` read fails for any reason other than not-found (a missing key stays `0.0.0`, the pin's
    rule for old buckets);
  - a stored `nats` value sets a key `config-loading` refuses: `rejectUnappliedNATSKeys` (`config/config.go:704`)
    runs on the value wrapped as `{"nats": ...}`, letter case folded.

  The error names the key.
- An unparseable stored `version` keeps the pin's rule (select the bucket, Warn), since nothing is lost by it.
- **Proof:**
  - `TestMalformedStoredServiceFailsStart` (P5);
  - `TestStoredNATSTokenFailsStart`;
  - `TestFirstBootPushFailureFailsStart`, driven by a component whose name D3 refuses, so no fault injection is
    needed;
  - `TestVersionReadErrorFailsStart` is not written: it needs a mid-operation fault (#185), and the branch is one
    `errors.Is`.
- **Cost:** a bucket holding a broken entry stops boot until an operator fixes or deletes the key. The message names
  the key. Runtime writes after Start keep logging and skipping (`handleUpdate`, optional).
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

- **Q1 (D2).** Should `Stop` take a context, `Start` refuse a second call, and the Manager keep running when the
  context passed to `Start` ends, as ADR-113 says for components? Cost: one line in each of semsource, semteams and
  semboids. Recommended: yes.
- **Q2 (D3).** When the bucket's copy wins at boot, should its components replace the file's, as its services
  already do, so a deleted component stays deleted (#17)? Component names NATS cannot hold as one key would be
  refused. Cost: a component added to the file needs a version bump to appear, as a service does today. No consumer
  name is refused. Recommended: yes.
- **Q3 (D4).** Should Start fail with the real error when it cannot write the first-boot configuration, read the
  bucket, or decode a stored entry, including a stored `nats` setting that loading refuses, instead of logging and
  continuing? Cost: a broken stored entry stops boot until it is fixed. Today it silently disappears. Recommended:
  yes.
