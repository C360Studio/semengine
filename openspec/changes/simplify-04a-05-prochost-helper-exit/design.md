# Design: simplify-04a-05-prochost-helper-exit

Status: draft, awaiting inventory review, pre-owner design review and owner acceptance on #208. Inventory:
`inventory.md` (base `d5ea8f6`).

## Context

Issue #208; epic #223 (owner split, #223 comment 6191234934). A **helper** is a function in a test binary that
`prochost.Start` runs in a child process. The **test process** is the test binary that called `Start`. Today only the
test's cleanup ends a helper the test left running, and a test process that is interrupted, killed, panics or hits
its `-timeout` runs no cleanup. The helper waits on SIGTERM and stays up under PID 1 (inventory § 1, P1 column A).

## Options

The ways a test process can end without cleanup:

- an exit without cleanup (including a helper-parent whose `fn` returned);
- Ctrl-C to its process group;
- SIGTERM or SIGKILL sent to it alone;
- a panic, which includes `testing`'s `-timeout` alarm.

| Option | Ways covered (probe P1) | Cost |
| --- | --- | --- |
| Do nothing | none | helpers pile up under PID 1, holding NATS connections; the 30-minute sweep removes only containers |
| (a) Helper exits when its stdin pipe from the test process reaches end-of-file (recommended) | all of them | a few lines in `Start` and `Helper`; stdin reserved |
| (b) Drop the helper's own process group | Ctrl-C to the group only | rewrite `Signal`/`Kill`, which signal `-pid` and would then hit the test binary's own group; covers nothing else |
| (c) Both | same as (a) | (b)'s rewrite for no added coverage |
| Linux parent-death signal | Linux only | does not exist on macOS (issue #208) |
| Poll the parent pid | all, late | a polling loop in the harness; a subreaper changes what the parent pid reads |

Option (b) alone does not cover a parent killed by itself. In a shared group, SIGKILL to the parent reaches only the
parent, and the helper keeps running (P1, column B, SIGKILL row). The kernel closes every file the dead process held,
including the pipe's write end. So the helper sees end-of-file however its parent died (P1, column C). End-of-file is
also a state, not an event: a helper that starts watching after its parent has already died reads end-of-file at
once, so there is no window at start-up.

## Decisions

### D1 A helper ends when its test process ends

While the test process runs, a helper ends only by what the test does to it (`Signal`, `Kill`, its own return) or by
the cleanup's kill, as today. Once the test process has ended, a helper that is not stopped exits on its own:

- it exits whichever way the test process ended;
- it writes a line to its stderr naming the reason;
- it exits with a non-zero status.

A helper's standard input belongs to `prochost`, and helper functions do not read it. The helper keeps its process
group (D2), and `Kill`, `Pause`, `Resume`, `Signal`, `Wait`, `Alive` and the cleanup keep their behaviour.

Proving test (`internal/harness/prochost/prochost_test.go`, new, table-driven):

- **Setup.** The test starts a helper-parent with `Start`. The helper-parent starts a grandchild helper that parks
  without catching anything but SIGTERM. It records the grandchild's pid, start identity (`ps lstart`) and stderr
  file, then ends in one of four ways.
- **Four ways.** Killed: the test calls `Kill` on the helper-parent, which reaches its group only. Interrupted: the
  test sends `Signal(SIGINT)`, which the helper-parent does not catch. Returned: its `fn` returns, and `Helper` exits
  without running cleanups. Panicked: this also stands for the `-timeout` alarm, which is a panic.
- **Checks.** Before ending the helper-parent, the test confirms the grandchild is running. Afterwards it confirms,
  within `failureBound`, that the grandchild's pid is gone or names a process with another start identity, and that
  the grandchild's stderr names the reason.
- **On the base commit** each row fails because the grandchild survives. The test's own cleanup kills a surviving
  grandchild whose start identity still matches, so a failing run leaves nothing behind.
- SIGTERM to the parent alone is the same operating-system event as the interrupted row (P1).

Early-exit guard: `TestStartedHelperHasItsOwnGroupAndCapturedOutput` expects exit status 0 after SIGTERM. A helper
that saw end-of-file early, for example because stdin was left as the null device, exits non-zero and fails it.

### D2 The helper keeps its own process group

Dropping the group (option b) covers only Ctrl-C, which D1 already covers. The group came with the removed runner's
test code (archived floor `design.md:149-154`); it keeps `Kill` and `Pause` reaching anything a helper starts, and
the spec's "Helper is started" scenario and its test stay as they are.

### D3 A stopped helper is not promised

A helper the test has paused (SIGSTOP) cannot read its stdin until it is continued, so D1 does not cover it. On macOS
a paused helper still ended when its parent was SIGKILLed (P2), most likely by the operating system's rule for
orphaned process groups. That is measured on macOS only and is not promised. Linux is not measured.

### D4 Documentation

The doc comment names the failure and #208 (`docs/test-infrastructure.md:146-148`). With the watch running, Go's
deadlock detector no longer ends a helper whose other goroutines all block (P3, measured with a shell pipe). The
reason given in `prochost.go:10-12` and `docs/testing.md:117-119` is replaced. The advice stands: wait for SIGTERM,
never a bare `select {}`, which the bare-select guard refuses, and SIGTERM is how a test ends a helper cleanly.

## Merge order

No open pull request touches these files or the `process-host` spec (inventory § 3). No order is needed.
