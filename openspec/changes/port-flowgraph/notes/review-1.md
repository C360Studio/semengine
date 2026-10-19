# Pre-Codex review, round 1: LOW fixes

Four LOW findings from the review of PR #320 at `e479478`, fixed under the owner's word of 2026-10-19 on simple
leftovers ("let’s fix them so we do not create debt on simople fixes as we port").

- **Duplicate publisher name (carried defect).** `ValidateStreamRequirements` listed one component twice when two of
  its plain NATS output ports fed the same JetStream input (`publishers=[alpha alpha]`). The merge now appends a name
  only when the warning does not already carry it. Proof: `TestValidateStreamRequirementsFlagsCoreNATSPublishers`,
  case `one publisher with two feeding ports is named once`, red before the fix (`[alpha alpha]`), green after.
- **Sort order.** Warnings sorted on the joined string `"comp:port"`, so `graph-index-spatial` came before
  `graph-index` (`-` sorts before `:`), against the README's "sorted by subscriber component and port". The dedup map
  is now keyed on a `struct{ comp, port string }` and the result sorted with `slices.SortFunc` on component, then
  port; the `fmt.Sprintf` key and its `:` collision are gone. Proof: case `warnings sort by subscriber component, then
  port`, red before the fix, green after.
- **Unprobed NATS claim (#170).** "A JetStream consumer reads from a stream, and a plain NATS publisher does not create
  one" is dropped from `README.md`, `doc.go` and the function's doc comment; each now states only what the function
  returns.
- **Misplaced comment.** The doc comment on `validateKVWriters` read `connectWatchPorts ...`; it now names the
  function.

The ledger row for `internal/flowgraph` now says each publisher is named once and lists `cmp` among its stdlib
imports. `design.md` keeps its draft of the row as written at acceptance; the ledger is the current record.
