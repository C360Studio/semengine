# Inventory probe: rule-core-04-graph-actions

Evidence for `inventory.md` § 2, so it can be re-run on another machine. Run by the architect on 2026-11-28 on a
scratch copy of the SemStreams pin `8b99efe9` (`gh api repos/C360Studio/semstreams/tarball/8b99efe9`, unpacked),
`go1.26.4 darwin/arm64`, Docker 29.9.3.

## PR 4's tests on the pin copy

In the pin copy's root. The `-run` pattern selects 39 top-level tests: 18 in the carried ranges of `actions_test.go`
and 11 in `actions_subject_override_test.go` (29 carried), 5 in `typed_substitution_test.go:60-272` that PR 3
forwarded (34 owned by PR 4), and 5 in dropped ranges it also matches (`actions_test.go:186`, `:257`, `:268`, `:335`,
`:378`).

```bash
R='^(TestAction_AddTriple|TestAction_RemoveTriple|TestAction|TestActionConstants|TestAction_TTLParsing|TestAction_VariableSubstitution|TestActionExecutor|TestAction_Publish|TestAction_Publish_PayloadFormat|TestExecutePublish_SubstitutesPropertyTemplates|TestExecutePublish_NilPropertiesNoOp|TestAction_Publish_NoPublisher|TestAction_Publish_ErrorHandling|TestAction_UpdateTriple|TestAction_UpdateTriple_NoMutator|TestAction_UpdateTriple_RemoveFailsContinues|TestAction_UpdateTriple_AddFails|TestExecutionContext_SubstituteVariables|TestExecutionContext_SubstituteVariables_WarnsOnUnresolved|TestExecutionContext_SubstituteVariables_NoWarnOnClean|TestDenyVerdict_ErrorString|TestDenyVerdict_ErrorsIs|TestDenyVerdict_ErrorsAs|TestExecuteAddTriple_.*|TestExecuteRemoveTriple_.*|TestExecuteUpdateTriple_.*|TestResolveTripleSubject_TruthTable)$'
go test -count=1 -race -run "$R" -v ./processor/rule/ 2>&1 | grep -c '^--- PASS'          # 39
go test -count=5 -cpu=1 -shuffle=on -run "$R" -v ./processor/rule/ 2>&1 | grep -E '^(ok|FAIL|-test.shuffle)'
go test -count=5 -cpu=1 -shuffle=on -run "$R" -v ./processor/rule/ 2>&1 | grep -c -- '--- FAIL'   # 0
go test -count=1 -race -tags integration -v \
  -run '^(TestTripleMutator_SuppressedAddDoesNotClaimAnotherWritersRevision|TestTripleMutator_NoOpRemoveDoesNotClaimAnotherWritersRevision)$' \
  ./processor/rule/ 2>&1 | grep -E '^(--- |ok|FAIL)'
grep -n -E 'time\.Sleep|\.Skip(f|Now)?\(|//go:build' processor/rule/actions_test.go \
  processor/rule/actions_subject_override_test.go processor/rule/triple_mutator_revision_integration_test.go \
  processor/rule/executor_platform_wiring_integration_test.go processor/rule/action_failure_metrics_test.go
```

Observed: 39 `--- PASS` under `-race`, `ok`; the shuffled run printed `-test.shuffle 1795863972197797000` and `ok`, 0
`--- FAIL`; both integration tests `--- PASS`, `ok`; the text scan found only the two `//go:build integration` lines.

## Probe files

Saved in the pin copy's `processor/rule/`, run, and left in the scratch copy only. They use the package's test fakes
`mockTripleMutator` (`actions_test.go:698-732`), `mockPublisher` (`:403-423`), `testExecutorPlatform` (`:3656`), and the
integration harness `newRevisionClaimHarness` (`triple_mutator_revision_integration_test.go:47-94`).

Unit probes, `go test -count=1 -v -run '^TestProbePR4' ./processor/rule/`:

```go
package rule

import (
    "bytes"
    "context"
    "encoding/json"
    "log/slog"
    "testing"

    gtypes "github.com/c360studio/semstreams/graph"
    "github.com/c360studio/semstreams/message"
)

const probeEntity = "c360.platform.robotics.mav1.drone.p4"

// P1: triple actions with no mutator succeed and write nothing.
func TestProbePR4NoMutator(t *testing.T) {
    x := NewActionExecutor(nil)
    ec := &ExecutionContext{EntityID: probeEntity}
    for _, a := range []Action{
        {Type: ActionTypeAddTriple, Predicate: "a.b.c", Object: "v"},
        {Type: ActionTypeRemoveTriple, Predicate: "a.b.c"},
        {Type: ActionTypeUpdateTriple, Predicate: "a.b.c", Object: "v"},
        {Type: ActionTypePublish, Subject: "x.y"},
    } {
        t.Logf("P1 %s err=%v", a.Type, x.Execute(context.Background(), a, ec))
    }
}

// P3/P4: what the mutator receives for remove_triple with object, and update_triple with properties and ttl.
func TestProbePR4MutatorArgs(t *testing.T) {
    m := &mockTripleMutator{}
    x := NewActionExecutorWithMutator(nil, m, testExecutorPlatform())
    ec := &ExecutionContext{EntityID: probeEntity}
    err := x.Execute(context.Background(), Action{Type: ActionTypeRemoveTriple, Predicate: "a.b.c", Object: "only-this"}, ec)
    t.Logf("P3 remove err=%v removed=%+v", err, m.removedTriples)
    m2 := &mockTripleMutator{}
    x2 := NewActionExecutorWithMutator(nil, m2, testExecutorPlatform())
    err = x2.Execute(context.Background(), Action{Type: ActionTypeUpdateTriple, Predicate: "a.b.c", Object: "v", TTL: "1m", Properties: map[string]any{"k": "v"}}, ec)
    t.Logf("P4 update err=%v removed=%+v added=%d", err, m2.removedTriples, len(m2.addedTriples))
    if len(m2.addedTriples) == 1 {
        b, _ := json.Marshal(m2.addedTriples[0])
        t.Logf("P4 added triple json=%s", b)
    }
}

// P5: an unresolved token in a triple object is written as text; in a subject it is refused.
func TestProbePR4UnresolvedObject(t *testing.T) {
    m := &mockTripleMutator{}
    x := NewActionExecutorWithMutator(nil, m, testExecutorPlatform())
    ec := &ExecutionContext{EntityID: probeEntity, Entity: &gtypes.EntityState{ID: probeEntity}}
    err := x.Execute(context.Background(), Action{Type: ActionTypeAddTriple, Predicate: "a.b.c", Object: "$entity.triple.x.y.z"}, ec)
    if len(m.addedTriples) == 1 {
        t.Logf("P5 object err=%v object=%#v", err, m.addedTriples[0].Object)
    } else {
        t.Logf("P5 object err=%v added=%d", err, len(m.addedTriples))
    }
    err = x.Execute(context.Background(), Action{Type: ActionTypeAddTriple, Subject: "$entity.triple.x.y.z", Predicate: "a.b.c", Object: "v"}, ec)
    t.Logf("P5 subject err=%v added=%d", err, len(m.addedTriples))
}

// P6/P7: publish with unresolved tokens, and the payload's keys.
func TestProbePR4Publish(t *testing.T) {
    var logs bytes.Buffer
    prev := slog.Default()
    slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
    defer slog.SetDefault(prev)
    p := &mockPublisher{}
    x := NewActionExecutorFull(nil, nil, p, testExecutorPlatform())
    ec := &ExecutionContext{EntityID: probeEntity, MessageData: map[string]any{"a": 7}}
    err := x.Execute(context.Background(), Action{Type: ActionTypePublish, Subject: "s.$message.missing", Properties: map[string]any{"n": "$message.a", "m": "$message.missing", "k": 3}}, ec)
    t.Logf("P6 err=%v published=%d", err, len(p.published))
    for _, pm := range p.published {
        t.Logf("P6 subject=%q data=%s", pm.subject, pm.data)
    }
    err = x.Execute(context.Background(), Action{Type: ActionTypePublish, Subject: "s.t"}, ec)
    t.Logf("P7 err=%v data=%s", err, p.published[len(p.published)-1].data)
    t.Logf("P6 warn lines=%d", bytes.Count(logs.Bytes(), []byte("level=WARN")))
    _ = message.Triple{}
}
```

Observed:

```text
P1 add_triple err=<nil>
P1 remove_triple err=<nil>
P1 update_triple err=<nil>
P1 publish err=<nil>
P3 remove err=<nil> removed=[{subject:c360.platform.robotics.mav1.drone.p4 predicate:a.b.c}]
P4 update err=<nil> removed=[{subject:c360.platform.robotics.mav1.drone.p4 predicate:a.b.c}] added=1
P4 added triple json={"subject":"c360.platform.robotics.mav1.drone.p4","predicate":"a.b.c","object":"v","source":"rule_engine","timestamp":"2026-11-28T05:11:10.730783-06:00","confidence":1,"expires_at":"2026-11-28T05:12:10.730783-06:00"}
P5 object err=<nil> object="$entity.triple.x.y.z"
P5 subject err=resolve add_triple subject: action.subject "$entity.triple.x.y.z" has unresolved template variables [$entity.triple.x.y.z] after substitution (…); refusing to write triple with garbled subject added=1
P6 err=<nil> published=1
P6 subject="s.$message.missing" data={"entity_id":"c360.platform.robotics.mav1.drone.p4","properties":{"k":3,"m":"$message.missing","n":"7"},"source":"rule_engine","subject":"s.$message.missing","timestamp":"2026-11-28T05:11:10.731165-06:00"}
P7 err=<nil> data={"entity_id":"c360.platform.robotics.mav1.drone.p4","properties":null,"source":"rule_engine","subject":"s.t","timestamp":"2026-11-28T05:11:10.731194-06:00"}
P6 warn lines=2
```

Integration probes, `go test -count=1 -tags integration -v -run '^TestProbePR4RemoveScope$' ./processor/rule/`:

```go
//go:build integration

package rule

import (
    "context"
    "testing"
    "time"

    "github.com/stretchr/testify/require"

    "github.com/c360studio/semstreams/graph"
    "github.com/c360studio/semstreams/message"
)

// P8: remove_triple clears the predicate's statements from every source.
// P9: add_triple onto an entity that does not exist.
// P10: update_triple's remove and add are two revisions.
func TestProbePR4RemoveScope(t *testing.T) {
    ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
    defer cancel()
    h := newRevisionClaimHarness(ctx, t)
    defer h.owner.finish(ctx, t)
    h.seedEntity(t)
    _, err := h.mutations.Append(ctx, graph.AppendTriplesRequest{Triples: []message.Triple{{
        Subject: revisionClaimEntityID, Predicate: "rule.state.phase", Object: "other", Source: "some-other-component",
        Timestamp: time.Now(), Confidence: 1}}})
    require.NoError(t, err)
    _, err = h.mutator.AddTriple(ctx, "r", ruleTriple("armed"))
    require.NoError(t, err)
    before := h.liveRevision(t)
    _, err = h.mutator.RemoveTriple(ctx, "r", revisionClaimEntityID, "rule.state.phase")
    t.Logf("P8 remove err=%v", err)
    exact, err := h.mutator.(*tripleMutator).reader.ReadExactEntity(ctx, revisionClaimEntityID)
    require.NoError(t, err)
    n := 0
    for _, tr := range exact.Entity.Triples {
        if tr.Predicate == "rule.state.phase" {
            n++
            t.Logf("P8 left: source=%s object=%v", tr.Source, tr.Object)
        }
    }
    t.Logf("P8 statements of rule.state.phase left=%d (revision %d -> %d)", n, before, exact.KVRevision)

    missing := ruleTriple("x")
    missing.Subject = "c360.platform.robotics.mav1.drone.absent1"
    _, err = h.mutator.AddTriple(ctx, "r", missing)
    t.Logf("P9 add to absent entity err=%v", err)

    x := NewActionExecutorWithMutator(nil, h.mutator, testExecutorPlatform())
    r0 := h.liveRevision(t)
    err = x.Execute(ctx, Action{Type: ActionTypeUpdateTriple, Predicate: "rule.state.phase", Object: "armed2"},
        &ExecutionContext{EntityID: revisionClaimEntityID})
    t.Logf("P10 update err=%v revisions %d -> %d", err, r0, h.liveRevision(t))
    r1 := h.liveRevision(t)
    err = x.Execute(ctx, Action{Type: ActionTypeUpdateTriple, Predicate: "rule.state.phase", Object: "armed3"},
        &ExecutionContext{EntityID: revisionClaimEntityID})
    t.Logf("P10 update over a held value err=%v revisions %d -> %d", err, r1, h.liveRevision(t))
}
```

Observed:

```text
P8 remove err=<nil>
P8 statements of rule.state.phase left=0 (revision 3 -> 4)
P9 add to absent entity err=entity not found: c360.platform.robotics.mav1.drone.absent1
P10 update err=<nil> revisions 4 -> 5
P10 update over a held value err=<nil> revisions 5 -> 7
```

## Product usage counts

`python3 -I` over the JSON under `configs/` of semteams 4df44ae8 and semboids ab4e0fbc (semsource has no rules): every
object with an `id`, a string `type` and a condition or action list is a rule (92); each action of `on_enter`,
`on_exit`, `while_true`, `on_recovery` and `actions` is counted by type and key. Template guards: for each
`$entity.triple.<p>` in a triple action's `subject` or `object`, whether the rule has a condition on field `<p>`.
