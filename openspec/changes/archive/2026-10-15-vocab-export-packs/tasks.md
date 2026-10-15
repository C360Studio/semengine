# Tasks: vocab-export-packs

Read `design.md`, `proposal.md`, `inventory.md`, and `specs/vocabulary-packs/spec.md`. Run `task test:unit` after each
code task and `task verify` on a clean tree before requesting cross-agent review. Record mutation checks, Rapid seeds,
and fuzz runs in `notes/<task>.md`.

## 1. Claim and design (orchestrator / architect)

- [x] 1.1 File issue #234, move to worktree `gemini/vocab-export-packs`, author `inventory.md`, `proposal.md`,
  `design.md`, `tasks.md`, and `specs/vocabulary-packs/spec.md`, and open the claiming draft PR.

## 2. Port `vocabulary/export` and `ADR-107` (developer)

- [x] 2.1 Port `vocabulary/export` from the SemStreams pin (`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`) and
  `docs/adr/107-semantic-web-vocabulary-lives-at-the-export-edge.md` (design D1):
  - adapt `vocabulary.SemStreamsBase` to `vocabulary.SemEngineBase` in `export.go`, `prefix.go`, and the 5 test files;
  - add `defer vocabulary.SnapshotRegistry()()` before `vocabulary.ClearRegistry()` in `export_test.go`,
    `jsonld_test.go`, and `turtle_test.go`;
  - add `vocabulary/export`, `docs/adr/107-semantic-web-vocabulary-lives-at-the-export-edge.md`, `vocabulary/bfo`, and
    `vocabulary/cco` rows to `docs/admission-ledger.yaml`.
  Checks: `go test -race -count=1 ./vocabulary/export/...`, `task ledger:check`.

## 3. Extend `vocabulary` for IRI lookup, classes, projection, and packs (developer)

- [x] 3.1 Write failing unit, property, and fuzz tests in `vocabulary/` for `LookupByIRI`, `ParseEntityTypeKey` /
  `RegisterEntityType` / `LookupEntityTypesByClassIRI`, `WithProjection`, and `LoadPackJSON` / `LoadPackYAML` /
  `RegisterPack` (design D2–D6), then implement `entity_type.go`, `projection.go`, `pack.go`, and the registry updates
  in `predicates.go` and `registry.go`.
  Checks:
  - `task test:unit`;
  - `task test:repeat`;
  - `go test -fuzz=FuzzParseEntityTypeKey -fuzztime=5s ./vocabulary/` (and seed replay);
  - `go test -fuzz=FuzzLoadPackJSON -fuzztime=5s ./vocabulary/` (and seed replay);
  - `go test -fuzz=FuzzLoadPackYAML -fuzztime=5s ./vocabulary/` (and seed replay);
  - `task mutate:check` on the new `vocabulary` logic.
  Record commands and outputs in `notes/3.1.md`.

## 4. Update documentation and run full verification (technical writer / developer)

- [x] 4.1 Update `docs/inventory-scope.md`, `docs/tier1-cross-check.md`, `docs/repository-map.md`,
  `docs/adr/102-entity-id-segment-semantics.md`, `docs/admission-ledger.yaml` (`vocabulary` row), `vocabulary/doc.go`,
  and `vocabulary/README.md`. Run `task verify` on a clean tree.

## 5. Review and archive

- [x] 5.1 Cross-agent review of record on the implementation commit.
- [x] 5.2 Archive `vocab-export-packs` and sync `openspec/specs/vocabulary-packs/spec.md` and
  `openspec/specs/harness-boundaries/spec.md` as the final content commit.
