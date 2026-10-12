# Codex section-5 M2 notes: stored fixtures carry statement metadata

Finding: PR #93 comment 6126836956, M2. The integration tests in `pkg/lifecycle` stored statements with no `Source`
and no `Timestamp`. The exact-read double in `manager_integration_test.go` serves them, but graph-ingest's
`decodeStoredEntity` refuses them with `graph_state_reset_required` / `missing_statement_metadata`. Test files only.

## Choices the design does not spell out

- One stamp for the package's integration tests: `fixtureSource` (`"lifecycle-test-producer"`) and `fixtureTime`
  (`query_transition_integration_test.go`). The source is not `graph.SourceLifecycle`, because these statements stand
  for another producer's writes; lifecycle writes no child link.
- Stamped: `putRawEntity`, the Children test's `link` statements, and the statement the concurrent writer adds in
  `TestIntegration_DespawnWith_DoesNotDeleteNewerStateAfterTerminalCommit`. That last one is not read back through
  the double today; it is stamped because it is a stored statement those tests write.
- No unstamped value is kept. No integration test here sets out to show a stored-value refusal. The Children case "an
  entity outside every registered pattern is refused" now stores a valid value, so the pattern check is the only
  thing that can refuse it.
- The double is unchanged. Making it refuse permanently is a separate decision, not taken here.

## Shown to matter

The exact-read double was edited by hand to refuse what `decodeStoredEntity` refuses (key mismatch, missing source or
timestamp). The edit was not committed; the file was restored from a `cp` backup and the SHA-256 checksums matched.
The run was `task test:integration -- -failfast=false ./pkg/lifecycle/`:

- before the fix: `TestIntegration_ListFiltersAndPagesOneWorkflow` (9 subtests),
  `TestIntegration_ChildrenLoadsLinkedChildrenAcrossWorkflows`, and
  `TestIntegration_CompleteMovesToTheReachableTerminal/no_phase_triple` failed;
- after the fix: the package passed.

Not covered: the unit-test double `bucketExactEntityReader` (`manager_test_helper_test.go`) has the same gap. When it
was made to refuse in the same way (again a temporary edit, restored with matching checksums), 10 unit tests failed.
Those were not triaged, and some of them exercise poison on purpose. It is outside this finding's double.
