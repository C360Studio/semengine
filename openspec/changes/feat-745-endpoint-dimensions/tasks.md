# Tasks: feat-745-endpoint-dimensions

Tests come first. Each code task ends at a clean-tree local `task verify`. "Shown to fail" means failing-first output
in `notes/<task>.md`. One pull request (#746), `Closes #745`.

## 1. Proposal

- [x] 1.1 The owner fills `Approved-by:`.

## 2. model: an embedding endpoint's width

- [ ] 2.1 Tests first, shown failing (`notes/2.1.md`), in `model/registry_test.go`:
  - `TestValidate` (`:59`) gains the case "endpoint negative dimensions", refused with
    `endpoint "bad": dimensions must not be negative`.
  - A registry loaded from JSON with `"dimensions": 384` on an embedding endpoint yields `Dimensions` 384 from
    `ResolveEndpointWithConfig(reg, CapabilityEmbedding)` (`model/registry.go:115`). One without the key yields 0
    and validates.

  Premises: `model/registry.go:239-290` (`EndpointConfig`), `:517-580` (`validateEndpoint`).
  Accept: the new cases fail on the base commit for the right reason.
- [ ] 2.2 Implement the field and its doc comment, saying it is used only by the embedding capability and what 0
  means, and the negative check. Accept: 2.1 passes; `task verify`.

## 3. Docs

- [ ] 3.1 `processor/graph-embedding/MIGRATION.md:31-32`: pass the endpoint's `Dimensions` as
  `httpembedder.Config.Dimensions`. Accept: `task docs:check`.

## 4. Close

- [ ] 4.1 Archive the change as the last content commit. Accept: `task spec:check`, `task merge:check -- 746`.
