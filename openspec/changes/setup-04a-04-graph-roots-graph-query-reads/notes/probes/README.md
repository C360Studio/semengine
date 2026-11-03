# Probe evidence for graph-query step 2

Evidence, not tests of this repository: a `.go.txt` file stays out of the build and the guards. The earlier probes
P0-P7 and the prefix costs are in `openspec/changes/archive/2026-11-03-setup-04a-04-graph-roots-graph-query/notes/probes/`.

## P8-P12, path search at the pin

`pin-path-probe.go.txt` is `TestPathProbe`; `pin-path-probe.log` is its output. It runs in a pin copy, beside the pin's
own `mockNATSClient`, and needs `gh` and Go, no Docker. Its answers from graph-ingest and graph-index are its own fakes.
The results describe what the pin does with those bytes.

```bash
probes="$PWD"
pin="$(mktemp -d)"
gh api repos/c360studio/semstreams/tarball/8b99efe9c66a4faa4fa509f9f62cc6bad8392128 > "$pin.tar.gz"
tar -xzf "$pin.tar.gz" -C "$pin" --strip-components=1
printf '{"Replace":{"%s/processor/graph-query/zz_path_probe_test.go":"%s/pin-path-probe.go.txt"}}\n' \
  "$pin" "$probes" > "$pin.overlay.json"
cd "$pin"
go test -overlay "$pin.overlay.json" -run '^TestPathProbe$' -v -count=1 ./processor/graph-query/
```

## What one hop costs

`hop-cost-probe.go.txt` is `TestHopCostProbe`, run on an archive of this tree at `b17886a`, placed as
`processor/graph-query/zz_hopcost_integration_test.go`; `hop-cost-probe.log` is its output. It asserts nothing and
polls with a sleep, which a test of this tree may not. Real graph-ingest and graph-index, NATS in Docker, 3,000 small
entities, one client, one run, an otherwise idle laptop. It says nothing about load or real entity shapes.

```bash
tree="$(mktemp -d)"; git archive b17886a | tar -x -C "$tree"
cp hop-cost-probe.go.txt "$tree/processor/graph-query/zz_hopcost_integration_test.go"
cd "$tree"
go test -tags integration -run '^TestHopCostProbe$' -v -count=1 -timeout 8m ./processor/graph-query/
```

| Read | Mean | p50 | p99 |
| --- | --- | --- | --- |
| `outgoing`, one edge (n=2,000) | 3.7 ms | 2.9 ms | 14.8 ms |
| `incoming`, one or two edges (n=1,500) | 8.6 ms | 7.2 ms | 32.8 ms |
| `incoming`, 1,000 edges (n=200) | 11.5 ms | 11.2 ms | 21.0 ms |
| `predicateList`, 3,000 entities (n=3) | 45 ms | | |
| One prefix page, 1,000 rows, 964,335 bytes, empty prefix then a five-part one | 156 ms, 40 ms | | |

Side reading: the index applied about 33 entities a second during this run (revision 192 at 0 s, 1,188 at 30 s, 2,160
at 60 s), while the writes took 4.6 s. An earlier run with 10,000 entities had not caught up after three minutes.
