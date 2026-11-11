# Review fixes for #589

Codex's review of PR #587 at `05d7aebfc20d076032a1af369c5da51c5bb79f7a` found two defects, tracked in #589.

## H1: `ReadBatch` did not count reads in the metric

`ReadBatch` added each removed item to `Statistics.Reads` but only set the size and utilization gauges, so
`semengine_buffer_reads_total` stayed at 0 for a consumer that drains in batches (SemSource's entity publisher).
`ReadBatch` now calls `recordReads(readCount, size, capacity)` (`metrics.go`), which adds the number of items
removed to the read counter and sets the gauges once, from the final size. `Read` is unchanged.

Did the pin have it? Yes. At `8b99efe9`, `pkg/buffer/circular.go` `ReadBatch` calls `cb.stats.Read()` per item and
then only `cb.metrics.updateSize(cb.size, cb.capacity)`, and `updateSize` in `pkg/buffer/metrics.go` sets the two
gauges and touches no counter (`git -C ../semstreams show 8b99efe9:pkg/buffer/circular.go`). `MIGRATION.md` lists
it under the silent changes, and the ledger row's contract gains item (10).

Test: `TestReadBatchCountsReadMetric` (`metrics_test.go`). Two buffers on one prefix, each: four writes, `Read`,
`ReadBatch(0)`, `ReadBatch(2)`, `ReadBatch(10)` with one item left, and `ReadBatch(5)` on an empty buffer. The
hand-counted oracle is 4 reads per buffer, 8 on the shared series, and size and utilization 0.

Failing first, before the fix, `go test -count=1 -run TestReadBatchCountsReadMetric ./pkg/buffer/` (the 2 is the
two `Read` calls):

```text
--- FAIL: TestReadBatchCountsReadMetric (0.00s)
    metrics_test.go:149: 
            Error Trace:    pkg/buffer/metrics_test.go:149
            Error:          Not equal: 
                            expected: 8
                            actual  : 2
            Test:           TestReadBatchCountsReadMetric
            Messages:       items removed by Read and ReadBatch
FAIL
FAIL    github.com/c360studio/semengine/pkg/buffer  0.317s
FAIL
```

After the fix, `go test -count=1 -run 'TestReadBatchCountsReadMetric|TestMetricsOnInjectedRegistry' -v
./pkg/buffer/`:

```text
=== RUN   TestMetricsOnInjectedRegistry
=== RUN   TestMetricsOnInjectedRegistry/another_collector_type_under_a_key_refuses_construction
--- PASS: TestMetricsOnInjectedRegistry (0.00s)
    --- PASS: TestMetricsOnInjectedRegistry/another_collector_type_under_a_key_refuses_construction (0.00s)
=== RUN   TestReadBatchCountsReadMetric
--- PASS: TestReadBatchCountsReadMetric (0.00s)
PASS
ok      github.com/c360studio/semengine/pkg/buffer  0.229s
```

## M1: drop and overflow ratios have no upper bound

Under DropNewest a rejected write counts a drop and an overflow but not a write, so `DropRate` and `OverflowRate`
can be above 1 (capacity 1, four writes: 3). The pin's arithmetic is kept. The doc comments on `Writes`, `DropRate`
and `OverflowRate`, and the README's statistics list, now say that `Writes` counts accepted items and that both
ratios are to accepted writes with no upper bound. The pin's comments promised 0 to 1; the ledger row's contract
gains item (11). `MIGRATION.md` does not mention the ratios, so it gains no line for M1.

Test: `TestDropNewestRatesExceedOne` (`circular_test.go`) pins writes 1, drops 3, overflows 3 and both ratios 3.
It passes on the unchanged arithmetic, as intended. Mutation check: a copy of `circular.go` whose DropNewest branch
also calls `cb.stats.Write()`, run with `task mutate:check -- -pkg ./pkg/buffer -test TestDropNewestRatesExceedOne
-file pkg/buffer/circular.go -mutant <copy> -expect-text "only the accepted write counts"`: `verdict: detection`.

Not covered: no generated checks; the read counts follow from a fixed script with an exact oracle, and named
examples cover each batch case the finding names (empty, zero, oversized, shared prefix).
