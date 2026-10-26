# Proposal: setup-04a-04-graph-roots-graph-ingest-split

## Why

`processor/graph-ingest/component.go` is 2,558 lines (`wc -l` at `c1caa45`): factory, lifecycle, metrics,
configuration, the boot sweep, the stream consumer, and the decode, ingest and write paths in one file. #300 section G
asks for the split and section D for "the file's own seams"; the owner routed both to change 4 (#300, comment
6250544834) and ruled on 2026-10-26 that the split goes "in its own pull request right after PR #409, not inside it,
so #409's behaviour change stays readable", with the file map of PR #409's design D8 and "moves only, no edits" (#300,
comment 6416204673). It is change 4's next pull request after #409.

## What Changes

- Whole top-level declarations move, unchanged and in their order, out of `component.go` into four new files
  (`metrics.go`, `config.go`, `consume.go`, `ingest.go`) and onto the end of two existing ones (`poison_inventory.go`,
  `query.go`). The file map is design D1. `component.go` keeps the factory, ports, lifecycle and storage set-up, about
  700 lines.
- The only new text in the six files is a package clause and an import block in each new file, and the added import
  lines in `query.go` and `poison_inventory.go`.
- One test outside the package names the file that holds graph-ingest's stream consumer
  (`natsclient/consumer_policy_callsite_test.go:323`, `:331`); its two expected names become `consume.go`.
- Three Go comments and one admission-ledger row that name `component.go` for code that moves name the new file.
- Comments on the open issues whose line citations point into moved code.

No behaviour changes: no declaration is added, removed, renamed or edited, and graph-ingest's exported surface is the
same. Design D3 gives the commands that prove it.

## Capabilities

None. No requirement changes, so the change carries no spec delta and sets `skip_specs: true` in `.openspec.yaml`, as
`openspec/changes/archive/2026-10-01-setup-03b-contract-boundary/.openspec.yaml` did. `openspec validate --strict`
refuses a change with no delta unless that key is set (measured with openspec 1.14.1).

## Issues

Addresses #300 (sections G and D's "the file's own seams"); the rest of section D, and sections C, F and H, stay open
there. Addresses #109 (where #300's G and D seams were routed). Retires no issue: the pull request body says
"addresses", never a closing keyword.

## Non-goals

Any edit to moved code, including #300 section D's other items (fact-lane re-validation, the `pkg/retry` loop, the
repeated shapes, the fixed inner `WithTimeout`s, `marshalFittingPrefixPage`'s double marshal); the extra package doc
comments in `component.go:1` and `query.go:1`; the test-only fields of #301; ADR text.
