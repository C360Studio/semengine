# Probe evidence for graph-query step 3

Evidence, not tests of this repository: a `.go.txt` file stays out of the build and the guards. The earlier probes are
in the archived step 1 and step 2 changes' `notes/probes/`.

## Local search at the pin

`step3-pin-probe.go.txt` is `TestZZStep3Probe`; `step3-pin-probe.log` is its output, one run, Go 1.26.4 on darwin/arm64.
It runs in a pin copy beside the pin's own code and calls three unexported functions directly: `synthesizeAnswer`,
`filterEntitiesByQuery` and `scoreEntityQuery` (`processor/graph-query/graphrag.go`). It needs `gh` and Go, no Docker.
It asserts nothing; it prints what the pin returns.

```bash
probes="$PWD"
pin="$(mktemp -d)"
gh api repos/c360studio/semstreams/tarball/8b99efe9c66a4faa4fa509f9f62cc6bad8392128 > "$pin.tar.gz"
tar -xzf "$pin.tar.gz" -C "$pin" --strip-components=1
printf '{"Replace":{"%s/processor/graph-query/zz_step3_probe_test.go":"%s/step3-pin-probe.go.txt"}}\n' \
  "$pin" "$probes" > "$pin.overlay.json"
cd "$pin"
go test -overlay "$pin.overlay.json" -run '^TestZZStep3Probe$' -v -count=1 ./processor/graph-query/
```

| Line | What it shows |
| --- | --- |
| `ANSWER` | The answer for one group of 7 members, 3 matches: `(7 entities, 0% match)`, a relevance local search never sets |
| `TIES_FIRST8` | 40 entities with equal scores came back in input order in this run; `sort.Slice` does not promise it |
| `TWO_TERMS_ONE_TEXT` | `alpha zzz` against a title `alpha` scores 0.25, under the 0.3 threshold, so it is dropped |
| `DUP_TERMS` | `alpha alpha zzz` scores 0.33 and is kept: a repeated term changes who matches |
| `TYPE_TERM` | `note`, the entity's type position, scores 1.0 |
| `PRED_TERM` | `title` scores 0.5 by matching the predicate name `dc.terms.title`, not a value |
