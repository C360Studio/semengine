# Inventory: simplify-04a-13-natsclient-storage

base: 1c6b34e76cebf9b8711a75a1a250f8d172cae972 (claim head 0cf27a19fb6bc5382f5dc978c7408fe769336b87, no content)

Question: what moving natsclient's storage inventory, pressure and report files into their own package touches, and
what #85's fix changes. Repositories read (docs/inventory-scope.md): this one; SemStreams at the pin `8b99efe9`
(tarball from `gh api .../tarball/8b99efe9…`); semsource, semconnect, semteams, semboids at symbol level (local
checkouts, file grep only).

## 1. The claimed gap

- Files (measured; #222 names files that do not exist): `natsclient/storage_account.go` 347, `storage_growth.go`
  203, `storage_inventory.go` 606, `storage_pressure.go` 652, `storage_report.go` 791,
  `storage_report_consumer.go` 578, `storage_resource.go` 238 = 3,415 non-test lines; ten `storage_*_test.go` files
  5,555 lines and `reviewer_storage_snapshot_probe_test.go` 57. All seven non-test files exist at the pin.
- Exported symbols: 118 names (types, functions, methods, constants), listed by a go/ast walk of the seven files.
- #85: `natsclient/storage_report_consumer.go:64` — `type StorageReportObserver interface {` has
  `ObserveResource`, `ForgetResource`, `ObserveAccount`; no account retraction. `:375` —
  `accountRetracted = true` (resync path); `:463` — `func (c *StorageReportConsumer) removeAccount() {` (live path);
  both only log through `:403` — `func (c *StorageReportConsumer) logAccountRetracted() {`.

## 2. Every spelling of the fact

- What the storage files need from the rest of natsclient (compile of the seven files as a separate package with
  `go build -gcflags=-e -overlay`): `ClassifyBackingStream`, `ResourceKind`, `ResourceKeyValue`
  (`backing_stream_prefix.go:35,:43,:64`), `ValidateKVLiteralToken`, `EncodeKVOpaqueToken`, `DecodeKVOpaqueToken`
  (`kv_key_contract.go:89,:200,:215`), `Client` (receiver of `storage_inventory.go:112` —
  `func (m *Client) AccountStreamLister() (StreamLister, error) {`), and unexported `request.go:29` —
  `func nilContextErrorOf(component, method string) error {`, called at `storage_inventory.go:268` and
  `storage_report.go:227`.
- Reverse: natsclient builds with the seven files replaced by an empty file. Its tests break only at
  `nil_context_siblings_test.go:207-208` (`accountTestCollector`, `accountAwareLister`, `newTestPublisher`,
  `newFakeReportStore`, `defaultSource`) and `:297` (`StorageInventory{}`).
- The moved tests need unexported natsclient test helpers from `test_helpers_integration_test.go`: `:59` `startFixture`,
  `:138` `failureBound`, `:144` `stopBound`, `:150` `closeClient`, used by `storage_inventory_integration_test.go`
  `:43,:84,:225,:239,:250`. Each stays used by other natsclient tests (16, 13, 20, 2 files).
- Account retraction is one fact with two homes in the consumer (`:375` resync, `:463` live); both feed `:403`.

## 3. Adjacent claims

- In-tree callers outside natsclient (`git grep -E 'natsclient\.<Name>([^A-Za-z0-9_]|$)'` per name; known positive
  `NewStorageReportPublisher` in `storage_report_test.go:253`): production none; comments only at
  `graph/kvcatalog/kvcatalog.go:88` and `:91`. Tests: `graph/kvcatalog/kvcatalog_attribution_test.go`,
  `kvcatalog_storage_report_test.go:51,:57`, `storage_report_integration_test.go` (37 references, 31 names).
- Spec: `openspec/specs/transport-client/spec.md:233` — `### Requirement: The storage report view changes at a sync
  marker` (declares the #85 gap at `:239-241`); Purpose `:8`. No other spec names these types
  (`git grep` for StorageReport|StorageInventory|storage_report|STORAGE_REPORT|AccountReport over openspec/specs).
- Ledger: natsclient row `docs/admission-ledger.yaml:820`, `disposition: adapt` (`:1718`), so `ledger:check` does not
  compare it with the pin; `:824-825` purpose names "storage inventory, pressure and reports"; `:1167-1169` the #85
  sentence. No ledger rule needs a row per package directory (`internal/harness/contract/ledger_test.go`).
- `docs/contract.md:139` — `| storage report | the storage observability endpoint |` (operator surface, served by
  `service`).
- Coverage: `scripts/cover-check.sh:29` — `"natsclient merged"`; `internal/harness/contract/cover_test.go:42`
  `d11Targets` mirrors the script.
- Open PRs (`gh pr list --state open`): #278 (ledger hunk `@@ -2025,13`, pkg/dispatch row; graph-ingest,
  internal/dispatch) — no shared hunk; #279 (pkg/security, `specs/security-config`) — no shared file; #242–#248
  Dependabot — none. #283 is this claim.
- Open issues (`gh issue list --state open --search natsclient`, plus storage_report, StorageReport, "storage report",
  STORAGE_REPORT, "storage inventory", pressure; bodies and comments grepped for file/type names): #85 (the
  observer); #222 (part a); #281 names `natsclient/storage_pressure.go:82`; #9 names #85 only as a word.
  Name the word, not the code: #223, #195, #169, #75, #74, #83, #141, #133, #132, #135, #134, #139, #162, #196, #216,
  #174, #280, #68, #24, #60, #103, #20, #15, #198, #94, #95.

## 4. Who it is for

- In-repo production callers: none. Pin production readers outside natsclient (`service`, admitted, change 3):
  44 names, e.g. `service/storage_observability.go:203` (`NewStorageReportConsumer`), `:218`
  (`NewStorageInventoryCollector`), `:219` (`deps.NATSClient.AccountStreamLister`), `:451`
  (`NewStorageReportPublisher`); `service/storage_observability_metrics.go:339,:398,:413` implement
  `ObserveResource`, `ForgetResource`, `ObserveAccount`; `:479` `deleteAccountTierSeries`. Pin
  `graph/kvcatalog.go:86,:89` name two constants in comments.
- Consumers (semsource, semconnect, semteams, semboids, local checkouts): no file names `StorageReportObserver`,
  `StorageReportConsumer`, `StorageInventoryCollector`, `StorageReportPublisher`, `AccountStreamLister` or
  `natsclient.{AccountReport,ResourceReport,StorageReportSnapshot,PressureState,StorageTier}`.

## 5. Problem shape

Moving a subsystem out of a package into a child package that imports its parent. Closest instance: none that moves
code. A child that imports its parent: `graph/kvcatalog` imports `graph`. Interface retraction mirrors
`ForgetResource` (`storage_report_consumer.go:66`).

## 6. Pin decision records

Fetched all 85 pin `docs/adr/*.md`; grep for storage_report|storage report|STORAGE_REPORT|StorageInventory|storage
inventory|storage observability|storage pressure|AccountStreamLister|StorageReport: none. ADR-068 (cited by pin
`docs/operations/35-storage-pressure-runbook.md:206`) governs the bucket's no-TTL rule in kvcatalog, untouched. The
pin's design record for this code is `openspec/specs/storage-observability/spec.md` (7 requirements), not ported.

## Shape sweep (moved code, at base)

- Stutter under a `storagereport` name: pinned revive flags 7 (`StorageReportConfig`, `StorageReportPublisher`,
  `StorageReportKey`, `StorageReportObserver`, `StorageReportSnapshot`, `StorageReportConsumerConfig`,
  `StorageReportConsumer`); a name not starting `storagereport` gets 0.
- One implementation: `storage_inventory.go:156` `InventoryPublisher`, only `StorageReportPublisher` in production
  (plus test fakes). Optional; stays.
- No classification by error text; no `context.Background`/`TODO`/`WithoutCancel` in the seven files.

## Adopter seam

The importer (today the pin's `service`; later a product) must know: the new import path and the 9 new names
(compile error); `ForgetAccount` (compile error on its observer). Default if they do nothing: the build fails. Nothing
reaches "doc" or below.
