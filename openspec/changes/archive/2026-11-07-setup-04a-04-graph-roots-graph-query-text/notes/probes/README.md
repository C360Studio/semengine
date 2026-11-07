# Step 5 pin probe

Pure functions of the pin's `processor/graph-query` on literal inputs; no fakes, no broker.

```bash
dir=$(mktemp -d) && cd "$dir"
gh api 'repos/C360Studio/semstreams/tarball/8b99efe9' > pin.tgz && mkdir pin && tar -xzf pin.tgz -C pin --strip-components=1
cp <this dir>/step5-pin-probe_test.go.txt pin/processor/graph-query/zz_step5_probe_test.go
cd pin && go test -count=1 -run TestStep5Probe -v ./processor/graph-query/ | grep -E 'RESULT|^ok'
```

Recorded with go1.26.4 darwin/arm64 on 2026-11-07: `step5-pin-probe.log`. P2 prints order only; the scores behind it
are read from `pin:graphrag.go:2416-2470`.
