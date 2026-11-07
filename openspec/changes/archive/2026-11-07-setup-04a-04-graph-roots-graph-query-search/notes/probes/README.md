# Step 4 probes

Both run on 2026-11-06, Go 1.26.4, darwin/arm64, one run each, added by `go test -overlay`, so neither tree changed.

- **Pin** (`step4-pin-probe.go.txt`, `.log`): SemStreams `8b99efe9`, local copy `/tmp/setup-04a-04-graph-query-stage/pin`
  (local only; blobs of the read files match `gh api contents?ref=8b99efe9`). The file is placed as
  `processor/graph-query/zz_step4_probe_test.go`. Replay:
  `go test -overlay overlay.json -count=1 -run TestStep4PinProbe -v ./processor/graph-query/`, where `overlay.json` is
  `{"Replace":{"<pin>/processor/graph-query/zz_step4_probe_test.go":"<this file>"}}`.
- **Tree** (`step4-tree-probe.go.txt`, `.log`): this repository at `f5ffe3b`, placed as
  `graph/query/zz_probe_test.go`, same command with `-run TestStep4TreeProbe ./graph/query/`.

The pin's keyword tier still emitted `path_predicates` for zone questions; the tree's does not (PR #475).
