# docs/setup-plan.md change (test-infra-audit, pull request 1)

Source: owner ruling, #205 comment 6132580576 ("we do not wait for anything semstreams as semstreams is being retired
as soon as we are at mvp").

## Lines 266-268, "Development and test infrastructure"

Old:

> Start with conservative concurrency limits. Dynamic ports prevent collisions but do not prevent CPU or memory
> saturation. During extraction, admission control for a shared Docker daemon must coordinate SemEngine and SemStreams;
> independent repository locks must not each claim exclusive use of the same daemon.

New:

> Start with conservative concurrency limits. Dynamic ports prevent collisions but do not prevent CPU or memory
> saturation; SemEngine bounds its own load with `go test -p` and one NATS server per package, and does not
> coordinate its use of a Docker daemon with another repository (#205, comment 6132580576).

## Lines 361-363, "SETUP 02 Isolated harness"

Old:

> Build the initial helper kit, owned disposable NATS fixtures, cancellation, bounded readiness, checked cleanup, and
> failure capture before porting slices. Separate persistent development commands. Bound Docker concurrency and
> coordinate admission across the repos sharing the daemon. Prove lifecycle ownership before expanding the service stack.

New:

> Build the initial helper kit, owned disposable NATS fixtures, cancellation, bounded readiness, checked cleanup, and
> failure capture before porting slices. Separate persistent development commands. Bound Docker concurrency within a
> run. Prove lifecycle ownership before expanding the service stack.

The SETUP 02 section records a completed stage; this edit changes what the plan asks of later work, not what SETUP 02
delivered.
