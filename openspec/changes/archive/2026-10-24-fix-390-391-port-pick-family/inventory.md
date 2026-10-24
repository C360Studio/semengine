# Inventory: fix-390-391-port-pick-family

Base: main 6b23fdf (claim commit 5b10a01). Issues #390, #391; claim PR #404. Not a porting change: `natsfixture`,
`metric`, `service` and `scripts/lint-test-ports.sh` are in the tree, so no pin probe applies. go1.26.10
darwin/arm64, macOS 26.5.2, Apple Git 2.50.1.

## 1. The claimed gap

Search: `git grep -nE 'Listen(TCP|Packet)?\(|ListenConfig' -- '*.go'`, then `git grep -nE '"(0\.0\.0\.0)?:0"|\[::\]:0'`.
Every port-0 pick on every address with network `tcp`, by purpose read from the code:

| Site | Purpose | Class |
| --- | --- | --- |
| `internal/harness/natsfixture/fixture.go:323` — `l, err := net.Listen("tcp", ":0")` | `freePort`: the number is closed and handed to Docker (`:227` `HostPort: hostPort`) | convert |
| `internal/harness/natsfixture/fixture_integration_test.go:183` — `l, err := net.Listen("tcp", ":0")` | `deadPort`: a port "nothing listens on", returned as the mapped port so the real connect fails (`:203`) | convert |
| `internal/harness/natsfixture/start_port_integration_test.go:37` — `held, err := net.Listen("tcp", ":0")` | `TestStartPastHeldPorts`: plays the #209 Mac program "on all addresses"; spec `nats-fixture/spec.md:77` | exception |
| `internal/harness/natsfixture/start_port_integration_test.go:57` and `:64` — `held1`/`held2, err := net.Listen("tcp", ":0")` | `TestStartRetriesOnceOnPortInUse`: Docker's bind must fail on these ports; spec `:91` | exception |
| `metric/lifecycle_test.go:70` — `held, err := net.Listen("tcp", ":0")` | blocks `Server`'s `":port"` bind (`metric/handler.go:145,163`); comment `:68-69` says so | exception |
| `service/metrics_owner_test.go:52` — `held, err := net.Listen("tcp", ":0")` | `heldPort`: blocks `":port"` binds (6 callers, e.g. `service_manager_health_listener_test.go:105`); probe H1 on #343 | exception |
| `service/metrics_owner_test.go:112` — `wildcard, err := net.Listen("tcp", ":0")` | #388 reproduction: plays the manager's every-address listener | exception |
| `service/pprof_test.go:93` — `held, err := net.Listen("tcp", ":0")` | blocks `MaybeStartPProf`'s `":port"` bind (`service/pprof.go:48-49`) | exception |

No `ListenConfig`, `ListenTCP`, `"0.0.0.0:0"` or `"[::]:0"` hit. Production binds `":port"` on every address on
purpose (`service/service_manager_http.go:75`, `service/pprof.go:49`, `metric/handler.go:163`; `service/README.md:133,837`):
out of scope. Tests that configure those with port 0 are not a `net.Listen` literal in test code (review only).

Release checks: `git grep -nE 'Dial(Timeout)?\("tcp' -- '*.go'`: `metric/handler_test.go:167` (live 127.0.0.1 listener),
`natsclient/client_close_held_test.go:347`, `natsclient/socket_test.go:24`,
`processor/graph-ingest/lifecycle_integration_test.go:779` (relay upstream), `service/metrics_owner_test.go:78` —
`conn, err := net.DialTimeout("tcp", target, time.Second)`, the one release check, already dialing the loopback of the
listener's family (PR #389, merged via #386 a1a6f3e). `metric/handler_test.go:128` `requireListenerClosed` checks the
listener's state, not a dial.

Script: `scripts/lint-test-ports.sh:5-6` — `# net.Listen. Ephemeral allocation via net.Listen("tcp", ":0") or` /
`"127.0.0.1:0" is the required pattern.`; `:10-11` claims it is sourced from `taskfiles/lint.yml` and
`.github/workflows/ci.yml`: `ls taskfiles` → no such directory. Its callers (`git grep -n lint-test-ports`):
`Taskfile.yml:53-54` (the `lint` task, run by `task verify`, which CI runs at `.github/workflows/ci.yml:55`) and
`TestLintTestPortsPasses` (`internal/harness/contract/addresses_test.go:51-56`, runner `runLintTestPorts` `:77-81`,
under `task test:unit`); `TestLintTestPortsHonoursNoMarker` (`:87-120`) runs a copy.

Script scan scope: `scripts/lint-test-ports.sh:54-55` —
`matches=$(grep -rnE "$LITERAL_PATTERN" --include='*_test.go' . ...`
reads every file under the working directory, including git-ignored ones (`.gitignore:24` — `.worktrees/`). Measured
from the main checkout at 6b23fdf with the system grep: `/usr/bin/grep -rl 'net.Listen' --include='*_test.go' .` lists
144 files under `./.worktrees/` beside 18 in the tree. A defect in this repository (#212); fixed in this change (D6).

## 2. Other spellings of the fact

- The pick: `freePort` (`fixture.go:322`) and `deadPort` (`fixture_integration_test.go:181`) are two spellings of
  "a closed port the OS handed out", in one package.
- The guards on test addresses: `TestNoFixedAddressesInTests` (`addresses_test.go:24-27`) with
  `fixedAddressViolations` (`:59-75`) and its sensitivity test (`:29-47`) refuse fixed broker addresses in `*_test.go`;
  `scripts/lint-test-ports.sh` refuses fixed `net.Listen` ports. Both serve `harness-boundaries` "No fixed addresses in
  tests". Neither reads the family of a port-0 pick.
- The reason-comment shape: `TestOwnServerReason` (`testtext_test.go:371-374`), `ownServerViolations` (`:412`),
  `codeBeforeLineComment` (`:358-363`).
- The guidance: `scripts/lint-test-ports.sh:63` (`127.0.0.1:0`), `docs/testing.md:498` (`127.0.0.1:0`),
  `docs/test-infrastructure.md:141-148`, `.agents/contracts/semengine-developer.md:359` and `-reviewer.md:359`
  ("ephemeral ports"). None names the family or the release check.
- Library facts: `go doc net.Listen` (go1.26.10): an empty or unspecified literal host "listens on all available
  unicast and anycast IP addresses ... To only use IPv4, use network "tcp4""; port empty or "0" is chosen. `go doc
  net.Dial`: an unspecified host means the local system. These are documented, so they need no probe.
- macOS pick behaviour: PR #389 body, the 20,000-bind walk (`tcp` `:0` got 63150 once; `tcp4` `:0` and
  `127.0.0.1:0` never). The results are posted; the program is not, and it does not show how often each kind reached
  the ports beside 63150 (ADR-112 gap; design task 1.3). In-tree: `TestListenerReleasedSeesOnlyTheListenersOwnFamily`
  (`service/metrics_owner_test.go:111`) shows macOS lets a tcp4 loopback bind beside an open every-address listener.
- Docker's bind: #345's error `listen tcp 0.0.0.0:63150: bind: address already in use`. Not promised.
- git grep: under Apple Git 2.50.1, `git grep -E` does not match `\s` (a planted fixed port: exit 1 with `\s*`, found
  with `[[:space:]]*`); `git grep --no-index --exclude-standard` searches a directory that is not a repository (exit
  0) and skips a git-ignored `.worktrees/` copy. Measured by hand in temp directories, 2026-10-24.

## 3. Adjacent claims

- Open PRs (`gh pr list --state open`): #404 (this claim); #403 `scripts/port-status.sh`,
  `internal/harness/contract/portstatus_test.go` (same package as the new check, different file); #401 OpenSpec
  files plus `service/component_manager*.go`, `service_manager*.go`, `service_manager_test.go`, `startup_*` and
  `rule_pack_bind.go`. None of `metrics_owner_test.go`, `pprof_test.go` or `addresses_test.go`. No capability overlap
  (#401's delta is `operator-surface`). No in-flight change under `openspec/changes/` besides archive.
- Change 4 (Fable, graph roots, not in tree): a local pin copy (`/tmp/change4-pin.P2hr/src`, local only, SHA not
  checked) has no `graph/` hit; `input/websocket/lifecycle_owner_test.go:32` and
  `output/websocket/websocket_test.go:1173` are later ports that will meet the guard.
- Specs: `harness-boundaries/spec.md:157-177` "No fixed addresses in tests" (fixed ports, no inline exemption, the
  guidance); `:213-229` "A test with its own server says why" (the reason-comment shape); `nats-fixture/spec.md:69-79`
  "Start takes its host port from the operating system", `:81-95` "Start retries a taken host port once".
- Ledger: `docs/admission-ledger.yaml:248-265` (`lint-test-ports.sh`, adapt) and `:267-278` (fixture test, adapt).
- Rules: `AGENTS.md:110` and `.agents/rules-index.md:55` (fixed ports); `AGENTS.md:120` and `rules-index.md:65`
  (`// Own server:`). Reviewer contract `.agents/contracts/semengine-reviewer.md:359` — "Network listeners use
  ephemeral ports." Its adapters `.claude/agents/semengine-reviewer.md` and `.codex/agents/semengine-reviewer.toml`
  link it by path; `internal/harness/contract/agentsetup_test.go` checks the link and adapter length.
- Archived D7 (`openspec/changes/archive/2026-10-01-flake-defense/design.md:282-299`): the marker it removed "exempts a
  line with no recorded reason and has no uses".
- Issues: `gh issue list --search "63150 OR wifiman OR tcp4 OR freePort OR lint-test-ports"`: #390, #391 open (retired
  here); #345 #372 #388 #352 #180 closed. Open `class:flake`: none.

## 4. Who it is for

No exported symbol, subject, bucket or config field. New: one contract test pair (unexported), a reason-comment
convention read by it.

## 5. Problem shape

A literal call that needs a reason on the line above, checked by a contract test with a fake tree. Closest instance:
`internal/harness/contract/testtext_test.go:365-410` (`ownServerCall`, `TestOwnServerReason`,
`TestOwnServerReasonSensitivity`). Adopted, placed in `addresses_test.go` beside the guard of the same requirement
family (§ 2).

## 6. Pin decision records and adopters

No package carried from the pin: no ADR applies. Adopter seam: the surface is `task verify`, reached by developers and
agents writing tests here (consumers do not run it). (1) They must know: pick `tcp4`/`127.0.0.1:0`, or write a
reason. (2) If they do nothing: `task verify` fails naming `file:line` with the fix. (3) They find out from a command
error. (4) What they should need to know is what the failure message tells them.
