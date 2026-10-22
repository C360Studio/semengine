# Notes: Codex code review, round 1

Review: PR #367 comment 6345481783 at `5ea607e`. Fixes in `ca9fdcb`; `task verify` ok on `ca9fdcb` (a first run failed
only because the Docker daemon stopped mid-run, in `internal/harness/natsfixture`'s `TestStartPastHeldPorts`; re-run
after Docker was restarted passed). Two earlier notes are superseded by this round: `notes/2.1.md` gives the old
`recordMatchesStatus(value any, ...)` signature (now `[]byte`), and `notes/2.2.md` says `matchesPattern` is kept as at
the pin (it now has the #371 length check).

## #370 (BLOCKING): KV query rounded large JSON numbers

- Fix (service/message_logger_http.go, queryKVBucket): the entry value is answered as `json.RawMessage(entry.Value())`
  when `json.Valid`; otherwise `json.Marshal(string(value))`, the watch path's exact rule
  (message_logger_kv_watch.go:219-224). Non-JSON values therefore still go out as a JSON string, as before the fix.
- recordMatchesStatus now takes the stored bytes and decodes only a `map[string]json.RawMessage` view plus the
  "status" token as a string; field name stays exact (no struct case-folding), null/number/missing status never match.
- Test: TestKVQueryPreservesValueBytes (integration). Oracle = stored bytes, exact string equality, not JSONEq.
  Before (5ea607e): actual
  `{"nested":{"n":[18446744073709552000,100,-0]},"sequence":9007199254740992,"status":"failed"}`.
- TestRecordMatchesStatus moved to byte inputs; added JSON-string, `{"Status":..}` and `{"status":null}` cases.

## #371 (MEDIUM): middle-* glob let prefix and suffix overlap

- Fix: `len(str) >= len(parts[0])+len(parts[1])` added to the two-part branch of matchesPattern.
- Tests: TestMatchesPatternMiddleWildcard (unit: aaa false, aaaa true, aaXaa true, aXaa false);
  TestKVQueryPatternMatchesBeforeLimit (integration, limit=2 over aaXaa/aaa/aaaa).
  Before: `matchesPattern("aaa", "aa*aa")` expected false, actual true; route answered [aaXaa aaa], want [aaXaa aaaa].

## Coverage gap: "*" discovery through a nonempty admitted registry (D7 U-c)

- TestMessageLoggerDiscoversAdmittedComponentSubjects (subscription integration): component admitted via
  admitTestRegistryComponent + NewComponentManager (seals), logger built by NewMessageLoggerService over that registry
  on fixtureClient. Asserts monitored_subjects == [logger.discovered.in logger.discovered.out] and only the declared
  subject's message is stored. Passed first run (gap, not defect); sensitivity shown in-tree (see below).

## Mutation

- #371: task mutate:check, TestMatchesPatternMiddleWildcard, -expect message_logger_http_test.go:430 -> detection (3/3).
- #370: task mutate:check -> inconclusive ("a test in a file built only with the integration tag is not run by this
  command"). Then in-tree cp backup (sha256 c24f4c25...012f before and after, match): mutant fails at
  message_logger_kv_integration_test.go:292 with the rounded value; restored file passes.
- Discovery: in-tree `if !ml.autoDiscover` -> `if true` (sha256 74d2d363...b7f6 before and after, match): fails at
  message_logger_subscription_integration_test.go:417, actual []string{}.
