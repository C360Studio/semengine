# Fixture cancellation retry tasks

## 1. Test correction

- [x] 1.1 Record the baseline test checksum and existing PR #118 failing evidence; confirm the current spec and
  `Start` retry path.
- [x] 1.2 Add a deterministic connect cancellation case after one injected mapped-port failure; run the focused
  integration test and record the intended failure before changing its assertions.
- [x] 1.3 Repair the test oracle for the permitted histories, terminal cancellation error, container cleanup, and no
  start after cancellation; run the focused integration test and report the result.
- [ ] 1.4 Run the relevant repository gates on the completed branch and record exact results and any limits.
  Hold: full verification failed on the separate Docker mapped-port failure in
  [#180](https://github.com/C360Studio/semengine/issues/180).
- [ ] 1.5 Record independent review of the completed code change and reconcile these tasks before the change is
  archived.
  Hold: a passing implementation verification run is needed before the Claude review request.

Task 1.1–1.3 evidence and local-only log names: [notes/task-1.md](notes/task-1.md).
