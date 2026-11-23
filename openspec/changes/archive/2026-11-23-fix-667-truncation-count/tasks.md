# Tasks: fix-667-truncation-count

Tests come first. Each code task ends at a clean-tree local `task verify`. Single PR (#728), `Closes #667`.

## 1. Proposal

- [x] 1.1 Proposal (`proposal.md`); `task spec:check` and `task docs:check` pass.

## 2. Code

- [x] 2.1 Tests that `httpembedder` returns each text's `truncated` mark, and that graph-embedding's
  `truncated_total` counts a provider-marked cut under `by="provider"` and a cut at `max_text_len` under
  `by="text_cap"`; then `TruncationMarker`, `IncTruncated(by)`, the httpembedder field and the collector. Every
  existing test passes with its fakes updated to the new `IncTruncated` signature.
- [x] 2.2 Count SemSource's cut entities with the new counter on one full index of the SemSource repository,
  cross-checked against semembed's own count (`notes/probes/semsource-truncation.md`).

## 3. Close-out

- [x] 3.1 Archive: `openspec archive fix-667-truncation-count --yes`; `task spec:check` and `task docs:check` pass.
