# Inventory: readiness-nil-context

base: `35fd3e33f42db83d5daafcce2b168fbed0108fb7` (origin/main, after #289 / 8e; the claim commit `00ada23` is empty;
draft PR #298)

Question: where `graph/readiness` lets a nil context through an exported, error-returning call, and what already
refuses one. Scope: issue #297 (owner: "Take #297: refuse a nil context in Publish and WaitForFirst"). This change
carries no package from the pin: `graph/readiness` is already ported (`docs/admission-ledger.yaml:2007` —
`- source_path: graph/readiness`, disposition `adapt`), so the pin probe runs, shape sweep and ADR listing are not
owed; the one pin statement below cites the pin file read with `gh api .../contents/...?ref=8b99efe9`.

## 1. The claimed gap — CONFIRMED

- `graph/readiness/publisher.go:71` —
  `func (p *Publisher) Publish(ctx context.Context, status graph.IndexStatusResponse) error {` takes no nil check;
  `:80` — `putCtx, cancel := context.WithTimeout(ctx, publishTimeout)` panics on nil
  ("cannot create context from nil parent", #297's probe). `:82` (`Put`) is not reached.
- `graph/readiness/watcher.go:252` — `func (w *Watcher) WaitForFirst(ctx context.Context) error {`; `:256` —
  `case <-ctx.Done():` is evaluated before the select chooses, so nil panics even when an envelope has arrived.
- Pin: the same, `graph/readiness/publisher.go:100` — `putCtx, cancel := context.WithTimeout(ctx, p.timeout)` and
  `watcher.go:317` — `case <-ctx.Done():` at `8b99efe9`. Behaviour change from the pin, so the ledger row's
  `contract` list grows by one item.
- Exported context-taking functions in the package (`git grep -n 'func .*context\.Context' -- 'graph/readiness/*.go'
  ':!*_test.go'`): `Publish`, `Run`, `WaitForFirst`, and private `watchOnce` (`watcher.go:264`, caller invariant,
  not owed a check). Only `Run` refuses.

## 2. Every current spelling of the fact (a nil context is refused)

- `graph/readiness/watcher.go:196-197` — `if ctx == nil {` / `return errors.New("readiness: nil context")`; doc at
  `:192` — `// Misconfiguration is refused at the call instead: a nil context, a nil bucket source,`.
- `natsclient/storagereport/storage_report.go:227-228` — `if ctx == nil {` / `return PublishResult{},
  nilContextErrorOf("Publisher", "Publish")` (classified; spec `storage-report`, `spec.md:50`).
- `pkg/retry/retry.go:63` — `return errors.New("retry: nil context")`.
- Rule homes: `.agents/contracts/semengine-architect.md:407-408` and `semengine-reviewer.md:246` — `Exported
  context-taking boundaries reject nil when able to return an error`; `openspec/specs/background-work/spec.md:64`
  covers calls that start background work (neither method does).

## 3. Adjacent claims

- Spec: `openspec/specs/graph-readiness/spec.md:36-40`, "Readiness wiring is refused, not ignored" — names `Run`'s nil
  context only.
- Ledger: `docs/admission-ledger.yaml:2017-2029`, the `graph/readiness` `contract` adaptations (1)-(9).
- Open PRs (`gh pr list --state open --json number,title,changedFiles,files`): #292 (8d, draft) edits
  `docs/admission-ledger.yaml` (the `graph/readiness` row's dependency list, its task 3.2, the line after `:2029`) and
  plans `graph/readiness/watcher.go` (its task 3.1: the `IsKVTombstone` call in `apply`, `:303-310`); not yet in its
  file list. #296 (6d) touches `natsclient/` only: no overlap. Dependabot PRs #242-#248: no overlap.
- Open issues (`gh issue list --state open --search readiness`, plus `WaitForFirst`, `nil context`): #297 (this);
  #110 and #183 name `graph/readiness` (envelope fields, `published_at`), not these checks; #295, #139, #223, #133
  name the word or other files; the rest name the word, not the package.

## 4. Who it is for

No new exported symbol, subject, bucket or config field. The two methods are framework API: contract line
`docs/admission-ledger.yaml:2016` (`Publisher, gauges, Watcher, ...`), product-style tests in `publisher_test.go`
and `watcher_test.go`. Callers (`gopls references`): `Publish` — `processor/graph-ingest/readiness.go:329` (ctx from
the status tick, non-nil) and `publisher_test.go:101,147,224`; `WaitForFirst` — `watcher_test.go:503,569` only.
Consumers: semsource `4093d3c` `processor/code-context/scope_integration_test.go:53` builds the pin's publisher in a
test; semconnect `dff1265` has no reader (`git grep -n -e WaitForFirst -e readiness.NewPublisher`).

## 5. Problem shape

Admit-or-refuse at an exported call. Closest instance: `watcher.go:196-197` (same package, same wording), with its
test row `watcher_test.go:671` — `{name: "nil context", nilCtx: true, ...`. Adopted. Not establishing a pattern, so
no adoption sweep; no tree-wide sweep of other packages was run (scoped to the deliverable).

## 6. Pin decision records

None carried: no package from the pin. ADR-083 governs the envelope, not argument checks.

## Adopter seam

1. Must know: nothing new (do not pass nil was already the rule).
2. Doing nothing (passing nil): today a panic in the caller's goroutine; for graph-ingest's tick, the process.
3. Finds out: after the change, a returned error at the call (typed runtime error rank).
4. Should know: nothing; the gap closes.
