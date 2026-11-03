# Probe evidence for the graph-query port

These files back the probe results in `inventory.md` section 1. They are evidence, not tests of this repository: a
`.go.txt` file stays out of the build and the guards, and runs only through a Go build overlay (`-overlay`, which
compiles a file into a package without writing it to disk there).

## Behaviour probes P0-P7

`pin-behaviour-probe.go.txt` is the source of `TestInventoryProbe`; `pin-behaviour-probe.log` is its output. It runs
against `processor/graph-query` in SemStreams at the frozen pin `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`. It needs
`gh`, Go, and no Docker.

From this folder:

```bash
probes="$PWD"
pin="$(mktemp -d)"
gh api repos/c360studio/semstreams/tarball/8b99efe9c66a4faa4fa509f9f62cc6bad8392128 > "$pin.tar.gz"
gzip -dc "$pin.tar.gz" | git get-tar-commit-id   # prints the full pin sha
tar -xzf "$pin.tar.gz" -C "$pin" --strip-components=1
printf '{"Replace":{"%s/processor/graph-query/zz_inventory_probe_test.go":"%s/pin-behaviour-probe.go.txt"}}\n' \
  "$pin" "$probes" > "$pin.overlay.json"
cd "$pin"
go test -overlay "$pin.overlay.json" -run '^TestInventoryProbe$' -v -count=1 ./processor/graph-query/
```

The test prints results and asserts nothing about them: compare its `RESULT` lines to the table. Lines marked `FAKE`
say what the probe's stand-in responders answer. Those answers are chosen by the probe, not real responder behaviour.
In particular real graph-ingest refuses the non-ID in P5 as `invalid_request`, where the fake answers `not_found`.

## Prefix verb cost

These ran on this repository at `2bc62e5`, not on the pin, through graph-ingest on NATS in Docker:

- `prefix-cost-full-walks.log`: the first run, with full walks of each group. An earlier form of the source below,
  not kept, produced it.
- `prefix-cost-bounded-walks.log`: the reviewer's re-run, which adds walks stopped at 23 pages.
- `prefix-cost-bounded-walks.go.txt`: the source of that re-run, `TestPrefixCostProbe`, overlaid as
  `processor/graph-ingest/zz_prefixcost_integration_test.go` and run with `-tags integration`. It takes about three
  minutes and seeds a million entities.
