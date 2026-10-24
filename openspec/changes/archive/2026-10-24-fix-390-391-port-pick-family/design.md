# Design: fix-390-391-port-pick-family

Base: main 6b23fdf. Rests on `inventory.md`. Issues #390, #391; claim PR #404.

## Options

1. **Do nothing.** The #345 retry hides #390's cause; every ported test can bring the pick back. Rejected: six flakes.
2. **Bash: extend `lint-test-ports.sh`** with a regex and an exception list in the script (the issue's first shape).
   Not chosen: a file-path list cannot say which call in a file is excepted, so it either goes stale or excepts too
   much, and recording the reason needs a line-above check in awk.
3. **Go contract test with a reason comment**, copying `TestOwnServerReason` (`testtext_test.go:365-410`). Chosen: same
   shape, existing helpers (`repoFiles` reads tracked and unignored files, `writeTree`, `requireViolation`,
   `codeBeforeLineComment`), paired sensitivity test by the same pattern.

## Decisions

- **D1, the pick (#390).** `freePort` (`fixture.go:322-329`) listens with `net.Listen("tcp4", ":0")`: every IPv4
  address, the address Docker's bind was seen using (#345's error text; Docker's choice is not promised). Go documents
  that `"tcp4"` listens on IPv4 only (`go doc net.Listen`, go1.26.10). That a `tcp4` pick skips a port an IPv4 loopback
  socket holds is measured by task 1.3's walk, not promised; the doc comments at `fixture.go:36-37` and `:313-321` say
  so. `deadPort` (`fixture_integration_test.go:181-192`) returns `freePort()`, so the package has one pick.
- **D2, the retry stays.** The gap between `freePort` closing its listener and Docker's bind is still open to any
  process, on Linux too. The retry (`startAttempts = 2`, PR #351) is what covers it, and `docs/test-infrastructure.md`
  § "Retiring harness code" removes harness code only when its failure can no longer happen.
  `TestStartRetriesOnceOnPortInUse` is unchanged. For the `nats-fixture` scenario at `:77` (ports held on every
  address), task 1.3's walk (PR #404 comment 6386556253, go1.26.10, macOS 26.5.2) handed a `tcp4` pick the port a
  `tcp ":0"` listener holds 0 times in 33,000 picks (25 beside it), and an explicit `tcp4` bind of the held port
  failed "address already in use"; measured on one Mac, not promised. The retry covers it either way.
- **D3, the guard (#391).** In every `*_test.go` file and every Go file under `internal/harness/`, a `net.Listen` call
  with the literal network `"tcp"` and the literal address `""`, or a literal address whose host is empty, `0.0.0.0`
  or `[::]` and whose port is empty or `0`, is refused unless the line above begins `// Every address:` followed by a
  reason. The check reads code before a line comment. The failure names `file:line` and says: pick in the family that
  is bound (`tcp4` for a port handed to Docker, `127.0.0.1:0` or the listener itself for a test server); a port held
  to make an every-address bind fail says why with `// Every address: <reason>`. Review only: an address in a
  variable, a raw (backquoted) string, a call split over lines (Codex code review on #404), `ListenConfig.Listen`, a
  production listener a test configures with port 0, and whether the reason holds.
  D7 of `flake-defense` removed a marker that exempted a line *with no reason*; this one requires a reason, as
  `// Own server:` does.
- **D4, the seven exceptions** get a reason comment directly above the call (inventory § 1): the three in
  `start_port_integration_test.go` (the #209 holder; Docker's bind must fail), `metric/lifecycle_test.go:70`,
  `service/metrics_owner_test.go:52` and `:112`, `service/pprof_test.go:93`. Their behaviour does not change: each
  held port was measured as it is (#343 H1, #351, #210).
- **D5, release checks: review only.** The tree has one release check, already correct (`metrics_owner_test.go:73-86`).
  A grep for `DialTimeout("tcp", addr` matches nothing today and misses a renamed variable, so it would prove
  nothing. `docs/testing.md` gains the line: a check that a listener released its port dials the loopback address of
  the listener's own family and counts only a refused connection. The guard's row in `AGENTS.md` and
  `.agents/rules-index.md` carries this and D3's review-only remainder (one row, for `AGENTS.md`'s byte budget).
  `.agents/contracts/semengine-reviewer.md:359` gains one line: a port-0 pick is in the family that is bound, an
  every-address pick's `// Every address:` reason holds, and a release check dials the listener's own loopback and
  counts only a refused connection. The adapters `.claude/agents/semengine-reviewer.md` and
  `.codex/agents/semengine-reviewer.toml` link the contract by path only, so the adapter-match check
  (`agentsetup_test.go`) needs no adapter edit.
- **D6, the script.** Header `lint-test-ports.sh:4-11`: the guard refuses fixed ports; port-0 picks are refused or
  allowed by `TestEveryAddressPortPickSaysWhy`; it runs from `Taskfile.yml`'s `lint` task (run by `task verify`, which
  CI runs) and from `TestLintTestPortsPasses` (`addresses_test.go:51-56`, `task test:unit`). `:21-32`: `freePort`
  picks with `tcp4`. Scan `:54-55`: `grep -rnE ... --include='*_test.go' .` becomes
  `git grep --no-index --exclude-standard -nE ... -- '*_test.go'`, so git-ignored paths such as `.worktrees/` are not
  read (inventory § 1: 144 such files today from the main checkout), and `\s*` becomes `[[:space:]]*` in both
  patterns (`:46`, `:51`), which git grep's ERE needs (inventory § 2). `--no-index` keeps the fixture test's and
  `copyScript`'s temp directories, which are not repositories, working. What is refused is unchanged. Only git
  grep's exit 1 (no match) counts as a clean scan: any other failure (for example exit 128 on a bad pattern) fails the
  guard with git's error shown, where the scan now discards stderr and passes (`2>/dev/null || true`).

## Observable behaviour

- `task verify` fails on a planted `net.Listen("tcp", ":0")` in any test file, naming the line; passes on `tcp4`,
  `127.0.0.1:0`, or a reasoned exception.
- `natsfixture` hands Docker a port picked on every IPv4 address; on Docker's refusal it retries once, as today.
- `scripts/lint-test-ports.sh` run from the main checkout reads no file under `.worktrees/`.

## Proving tests

- **New `TestEveryAddressPortPickSaysWhy`** in `internal/harness/contract/addresses_test.go`, beside
  `TestNoFixedAddressesInTests` (same requirement family), over `repoFiles`. On the base it fails naming nine sites
  (inventory § 1); recorded in `notes/2.1.md`. The planted call text is assembled at run time so the file does not
  trip itself.
- **New `TestEveryAddressPortPickSaysWhySensitivity`.** Clean tree: `tcp4` `":0"`, `"127.0.0.1:0"`, a reasoned call, a
  comment naming the call, the call in a non-test file outside `internal/harness/`. Each of these is a violation naming
  `file:line`: `""`, `":0"`, `":"`, `"0.0.0.0:0"`, `"0.0.0.0:"`, `"[::]:0"`, `"[::]:"`, another comment above, an
  empty reason, and the call in a non-test file under `internal/harness/`. A tree with no test file reports "scanned
  no test file".
- **D1 has no in-tree test.** The failure needs another process's IPv4 loopback socket on the exact port the kernel's
  counter reaches next; showing it in a test means walking the counter past a held socket, which is macOS-only, slow
  and clever. Rejected. D1's evidence is task 1.3's posted walk; a revert of the spelling fails
  `TestEveryAddressPortPickSaysWhy` naming `fixture.go`.
- **D6's scan: new `TestLintTestPortsSkipsIgnoredFiles`** in `addresses_test.go`, beside
  `TestLintTestPortsHonoursNoMarker` and using its `copyScript` temp root: a `.gitignore` naming `ign/` and a fixed
  port in `ign/x_test.go` exits 0; the same fixed port at an unignored path exits 1. It fails on the base (the
  `grep -r` scan reads `ign/`). `notes/2.5.md` also records a by-hand run from the main checkout, before and after
  (files read under `.worktrees/`); the fixture test and `TestLintTestPortsHonoursNoMarker` stay green.
- Unchanged and still green: `TestStartRetriesOnceOnPortInUse`, `TestStartPastHeldPorts`, `TestS1_1PartialStartupCleanup`
  (`task test:integration`), and the `service`/`metric` tests at the exception sites.
- Examples suffice: a fixed set of literal spellings, no interacting inputs.

## Merge order

No shared file with #401 or #403. #403 edits `portstatus_test.go` in the same Go package as `addresses_test.go`; the
new code declares no name `portstatus_test.go` uses. Neither waits. Change 4 ports no hit (inventory § 3); a later
port with a `":0"` pick adds `tcp4` or a reason.
