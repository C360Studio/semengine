# Tasks: setup-04c-01-http-embedder

Tests come first. Each code task ends at a clean-tree local `task verify`. "Shown to fail" is failing-first output
in `notes/<task>.md` or `task mutate:check`. Premises cite `inventory.md` and `design.md`. Each code task names its
pull request (design D8): A, then B.

## 1. Design

- [ ] 1.1 Architect inventory and design (`inventory.md`, `design.md`, `notes/probes/pin-http-embedder.md`). INVENTORY
  PASS, then pre-owner design review passes; the owner fills `Approved-by:` and rules on design questions 1 to 6.
  Hold: OPEN-Q proposal awaits owner approval

## 2. PR A: the client

- [ ] 2.1 Tests first: design D3's table, against a fake shaped like semembed at 7ceb5281 (`main.rs:18-60`); the S1-S4
  cases are shown failing against the pin's behaviour (`notes/2.1.md`). The two prefix tests and three `Dimensions`
  tests are carried, adapted.
- [ ] 2.2 The client (design D1-D3): plain `net/http`; `TestEmbeddingImportsNoProviderModule` and
  `TestTreeImportsNoProviderPlumbing` pass unchanged; `go.mod` is unchanged.
- [ ] 2.3 `TestProviderContract` and `task test:provider` (design D5), with toolcheck updated if it pins Taskfile
  invocations. Run against semembed 7ceb5281 (default model); output, image build and model in `notes/2.3.md`.
- [ ] 2.4 `TestHungEmbedderFreesWorkerLane` first, then the Worker's 30 s embed bound (design D4); graph-embedding
  README "Known limits" updated.
- [ ] 2.5 Ledger row `graph/embedding/http_embedder.go` → `adapt` (destination, contract, proving tests);
  `task ledger:diff`. Docs: `graph/embedding/README.md:114`, `doc.go:2-3`, `docs/repository-map.md:74`,
  `processor/graph-embedding/README.md:58` (the example uses the client), MIGRATION.md (SemSource's constructor).

## 3. PR B: fallback and degrade test

- [ ] 3.1 Tests first: design D6's four tests and D7's `TestLosingTheProviderKeepsWordSearch`, shown failing on main
  (`notes/3.1.md`).
- [ ] 3.2 `graph.GlobalSearchData.Fallback` and searchGraph's fallback (design D6); graph-query README and MIGRATION.md
  (SemSource's `disclosure.go:31`).

## 4. Close-out

- [ ] 4.1 Spec sync and archive as the last content commit; PR B body carries `Closes #489` and `Closes #563`.
