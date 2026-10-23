# Design: fix-262-stale-days-operand

Base: main 46d3cd5. Rests on `inventory.md`. Issue #262, claim PR #381.

## Options

1. **Do nothing.** The hang and the silent misreads stay. Rejected: the script must fail closed (AGENTS.md section 1,
   "Failure paths fail closed").
2. **Default a missing operand to 7 and shift once.** This keeps the intent of `${2:-7}`. Rejected: it guesses, and
   `--stale-days --strict` would still be ambiguous.
3. **Refuse the operand unless it is all digits (#262's text, semsparql 289d791).** Recommended. It reuses the
   script's existing refusal shape, `unknown argument: $1` with exit 2 at `:65`.
4. **Option 3 plus a range check** (`[ "$2" -ge 0 ] 2>/dev/null`), which also refuses an operand that bash's
   integer test cannot read (20 digits or more). This is owner question Q1. Without it, such an operand is accepted
   and drops every stale note, and the PR files that as a defect.

## Observable behaviour

- `--stale-days N` is accepted when N is one or more ASCII digits. `0` and leading zeros are accepted, and `08` is
  read as 8. The bash test at `:237` reads decimal; this was probed on bash 3.2.57.
- Otherwise the script prints `--stale-days needs a whole number of days` as a line on standard error, prints
  nothing on standard output, and exits 2. "Otherwise" covers no operand, an empty operand, a sign, a suffix, or
  another option. The refusal comes before the CLI check and before the change list is read. It holds wherever
  `--strict` or `--check` sits in the argument list. An option that follows `--stale-days` is never read as that
  option.
- An unknown argument still prints `unknown argument: <arg>` and exits 2, as it does today.
- Every run ends. Invariant: for every argument list the script terminates, and an operand is accepted exactly when
  it matches `^[0-9]+$` (subject to Q1). Spec home: the ADDED requirements.
- The header keeps its line count, so `--help` (`sed -n '2,51p'`, `:64`) still prints the whole header. If a line is
  added anyway, the range moves with it in the same commit.

## Proving tests

`TestSpecQueueArguments` (`internal/harness/contract/specqueue_test.go`) uses `runSpecQueue` with the
`listOneChange` stand-in and a clean `tasks.md`. Expected values are written by hand.

| Case | Args | Want | On the base |
| --- | --- | --- | --- |
| A1 | `--stale-days` | exit 2, stderr line, empty stdout | does not end; fails at the bound |
| A2 | `--stale-days abc`, `-1`, `''`, `7d` | same | exit 0 |
| A3 | `--stale-days --strict`; `--strict --stale-days abc` | same | exit 0 |
| A4 | `--stale-days 0`, `--stale-days 08` | exit 0, `ok` line, empty stderr | passes (guards against over-refusal) |
| A5 | `--bogus` | `unknown argument: --bogus`, exit 2 | passes |

The bound: every `runSpecQueue` run gets a deadline derived from `t.Context()`, as in
`message/generic_json_shape_test.go:166-168`. A run that has not ended by then is killed, and the test fails with a
message saying it did not end. No sleep, no port. The developer picks the bound value. Examples are enough here,
because the operand grammar is one shell pattern and every input class has a row. Sensitivity is checked by hand
(`docs/testing.md`, "Run it by hand", because `task mutate:check` takes only `.go` files) with three mutants:

- M1: the base line.
- M2: the pattern without `''`.
- M3: the refusal's `exit 2` changed to `exit 0`.

Each must be detected.

## Merge order

The only overlapping open PR is #376 (draft), and the only shared file is `docs/repository-map.md`. Neither PR
depends on the other. Whichever lands second merges `origin/main` and resolves the map rows.
