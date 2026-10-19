# Design: one-exclusive-address-check

Accepted by the owner on 2026-10-19, with a yes on the question in "One component, one address twice" (#324
comment 6278113073). Line numbers are at base `a5c0fe4`.

## Terms

- **Exclusive resource:** an address only one port may hold. `PortFacts.IsExclusive` says which ports claim one;
  the kind set is closed (`component/port_resolver.go:27`) and only `NetworkPort` (`component/port_network.go:18`)
  is exclusive, so in practice this is a network address, spelled as its `ResourceID` (`tcp:0.0.0.0:8080`).
- **Admission:** `Registry.CreateComponent` adding a built component's `Declaration` to the registry's set.

## What exists

- **The registry's check.** `component/registry.go:571` `checkResourceConflictsLocked`, called once, at `:402`
  inside `admitPrepared`, after the duplicate-name refusal (`:396-401`). It reads `Declaration.ExclusiveResources`
  (`:69`), which `exclusiveResources` (`:611-620`) builds, sorts, and then folds duplicates out of with
  `compactStrings` (`:636`, whose only caller is `:619`). It refuses a second component on a held address and names
  the holder. No test drives it: `grep 'already used by' component/*_test.go` is empty.
- **Flowgraph's copy.** `internal/flowgraph/flowgraph.go:197-199` calls `validateNetworkPorts` (`:639-677`), which
  uses `exclusivePortRefs` (`:690-709`); nothing else calls either. `portInfo.exclusive` (`:37`, set at `:165`) has
  no other reader. `describePorts` (`:682`) stays: `validateKVWriters` (`:613`) uses it. `PortFacts.exclusive`
  (`component/port_facts.go:49`) stays: the registry reads it.
- **Readers.** Nothing imports `internal/flowgraph`; `composition` and `service` are not ported yet. At the pin
  (`8b99efe9`), flowgraph never sees a conflict between two components: `composition/validate.go:58-61` screens
  with `exclusiveConflict` (`:139-155`) before analysis, and `service/component_manager.go:397-402` passes only
  admitted declarations.

## What deleting flowgraph's check loses

- **Which component is refused:** nothing that matters. The registry refuses whichever component is admitted second
  and names the holder; flowgraph refused every pair and named all of them. Only the wording differs.
- **A component replacing itself:** cannot happen. The registry has no replacement call
  (`component/registry_boot_admission_test.go:88`), and the duplicate-name refusal at `:396` runs first, so the
  check's skip of its own name (`:573`) never runs.
- **One component, one address on two of its own ports:** lost. Flowgraph refuses it (`:648-651`, `:661-672`); the
  registry accepts it because `compactStrings` folds the duplicate away.

## One component, one address twice (ruled yes)

Refuse it. Flowgraph refuses it today, so deleting flowgraph's check without this would remove a refusal that exists
now. A socket that both reads and writes is one port, not two, and no production declaration at the pin or in the
consumers declares one address twice.

The refusal goes in `exclusiveResources`, which both `Declare` (`:310`) and `CreateComponent` (through
`captureComponentDeclaration`, `:607`) call, so both refuse it. The error is of the invalid class and reads:

`resource conflict: tcp:0.0.0.0:8080 already used by another port of component 'gateway'`

It names no port: `exclusiveResources` sees port facts, not port names. That is accepted as is.

## Change

1. Test that the registry refuses a second component on a taken address,
   `TestRegistryRefusesASecondComponentOnATakenAddress`. It passes today; mutant M1 shows it can fail. Oracle:
   literals in the test (`tcp:127.0.0.1:8080`, holder `first`), `errs.IsInvalid`, and `Snapshots()` holding only
   `first`. Sensitivity pair: two different addresses, and two `FilePort`s on one path, are both admitted. M1, in a
   copy of `registry.go`: `if resource == existingResource {` becomes
   `if resource == existingResource && resource == "" {`.
2. Failing test for one component claiming one address twice,
   `TestRegistryRefusesOneComponentClaimingAnAddressTwice`: an input and an output, two inputs, and two outputs,
   each through `CreateComponent` and `Declare`. Oracle: the exact text above.
3. Implement: `exclusiveResources(instanceName, inputFacts, outputFacts) ([]string, error)` sorts the resources and
   refuses two equal neighbours; its two callers return the error. Delete `compactStrings`, the self-skip and the
   check's `instanceName` parameter. M2, against the new code: the duplicate check becomes
   `if false && resources[i] == resources[i-1]`, run against the test from step 2.
4. Delete flowgraph's copy: `validateNetworkPorts`, `exclusivePortRefs`, the call, `portInfo.exclusive`; and its
   tests: `TestBuildFromDeclarationsRefusesNetworkBindConflicts`, the "network bind conflict" case of
   `TestRefusalsNameNoInternalFunction`, the `networkPort` helper, and the subtests "Network pattern detects port
   conflicts" and "shared file paths are not exclusive network binds" in `flowgraph_patterns_test.go`. The "two
   conflicts at once" case is rewritten with two KV buckets, each with two writers, so it still checks how two
   refusals are joined. No new flowgraph test: `flowgraph_validation_test.go:13` already uses network ports.
5. Spec delta: ADDED to `component-registration`, "Admission refuses a second claim on an exclusive resource".
6. Docs, then `task verify`.

## Docs

- `internal/flowgraph/doc.go` and `README.md`: drop the network-bind refusal; network ports are never joined.
- `docs/repository-map.md`, the `internal/flowgraph/` row: drop "two binds on one network address".
- `component/README.md`, `CreateComponent`'s refusals: add "or one address on two of its own ports".
- `docs/admission-ledger.yaml`, flowgraph row: the contract, the known risks and the proving tests drop the
  network-bind refusal and `TestBuildFromDeclarationsRefusesNetworkBindConflicts`. `component` row: adaptation (12)
  (admission refuses one component claiming an exclusive resource on two of its ports, through `Declare` and
  `CreateComponent`; `compactStrings` and the unreachable self-skip removed), and the two new tests in
  `proving_tests`.

## Merge order

- #322 (config) also edits `docs/admission-ledger.yaml` and `docs/repository-map.md`. Neither waits for the other;
  whichever lands second merges `origin/main` and re-checks both files.
- #323 also edits `flowgraph.go` and `TestRefusalsNameNoInternalFunction`; it has no PR yet. Whichever lands second
  merges `origin/main`.
- #280 and #321 are not touched.

## Not in scope

- The composition-port note: it lands with `composition`.
- #328, the protocol-name defect found in this design.
