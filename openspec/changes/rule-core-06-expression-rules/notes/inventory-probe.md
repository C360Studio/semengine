# Inventory probe: rule-core-06-expression-rules

Evidence for `inventory.md` § 2, so it can be re-run on another machine. Run by the architect on 2026-11-27 on a
scratch copy of the SemStreams pin `8b99efe9` (`gh api repos/C360Studio/semstreams/tarball/8b99efe9`, unpacked).

## PR 6's tests on the pin copy

In `processor/rule` of the pin copy. The `wc -l` line guards against an empty list: `-run "^()$"` runs nothing and
still prints `ok`.

```bash
cd <pin copy>/processor/rule
R=$(grep -h -oE '^func (Test[A-Za-z0-9_]+)' expression_factory_test.go matches_test.go payload_projection_test.go | sed 's/func //' | paste -sd'|' -)
echo "$R" | tr '|' '\n' | wc -l                                                   # 36
go test -count=1 -race -v -run "^($R)\$" . 2>&1 | grep -E -c '^--- PASS: Test'     # 36
go test -count=5 -cpu=1 -shuffle=on -v -run "^($R)\$" . 2>&1 | grep -E '^-test.shuffle|^ok|^FAIL'
go test -tags integration -count=1 -race -v -run '^(TestIntegration_MatchesWithLifecycle_RealManagerUnresolvedEntity|TestIntegration_MatchesWithLifecycle_RealManagerOrdinaryDefinition|TestIntegration_MatchesWithLifecycle_PairAgreesViaRealManager|TestIntegration_RuleReadableProjectionProductionLifecycle)$' . 2>&1 | grep -E '^(--- |ok|FAIL)'
```

Observed: 36 names, 36 `--- PASS` under `-race`; the shuffled run printed `-test.shuffle 1795789945646102000` and
`ok`; the four integration tests each printed `--- PASS` and the package `ok` (Docker reachable).

## Probe file

Saved as `processor/rule/probe_pr6_test.go` in the pin copy, run with
`go test -count=1 -v -run '^TestProbePR6' .`, then deleted. It uses the package's test helpers
`createTestMessage`, `createTestEntityState` (`expression_factory_test.go:464-481`) and `testPlatform`.

```go
package rule

import (
    "bytes"
    "context"
    "log/slog"
    "testing"

    "github.com/c360studio/semstreams/message"
    "github.com/c360studio/semstreams/processor/rule/expression"
)

func probeRule(t *testing.T, conds []expression.ConditionExpression, pattern string) *ExpressionRule {
    def := Definition{ID: "probe", Type: "expression", Name: "probe", Enabled: true, Conditions: conds}
    def.Entity.Pattern = pattern
    r, err := NewExpressionRule(testPlatform, "probe-pack", def)
    if err != nil {
        t.Fatal(err)
    }
    return r
}

// P1: a payload without the field, empty or not.
func TestProbePR6EmptyPayload(t *testing.T) {
    r := probeRule(t, []expression.ConditionExpression{{Field: "$message.x", Operator: "ne", Value: "a"}}, "")
    t.Logf("empty=%v nonempty=%v", r.Evaluate([]message.Message{createTestMessage(map[string]any{})}),
        r.Evaluate([]message.Message{createTestMessage(map[string]any{"y": 1})}))
}

// P2: where a message-path evaluation error goes.
func TestProbePR6MessageErrorLevel(t *testing.T) {
    var buf bytes.Buffer
    old := slog.Default()
    slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
    defer slog.SetDefault(old)
    r := probeRule(t, []expression.ConditionExpression{{Field: "$message.x", Operator: "gt", Value: "abc"}}, "")
    got := r.Evaluate([]message.Message{createTestMessage(map[string]any{"x": 5})})
    t.Logf("result=%v infoLog=%q", got, buf.String())
}

// P3: an $entity.* value template on the message path.
func TestProbePR6EntityTemplateOnMessagePath(t *testing.T) {
    r := probeRule(t, []expression.ConditionExpression{{Field: "$message.who", Operator: "eq", Value: "$entity.id"}}, "")
    t.Logf("literal-match=%v", r.Evaluate([]message.Message{createTestMessage(map[string]any{"who": "$entity.id"})}))
}

// P4: Subscribe without and with an entity pattern.
func TestProbePR6Subscribe(t *testing.T) {
    c := []expression.ConditionExpression{{Field: "$message.x", Operator: "eq", Value: 1}}
    t.Logf("no-pattern=%v pattern=%v", probeRule(t, c, "").Subscribe(), probeRule(t, c, "a.b.c.d.e.*").Subscribe())
}

// P5: an unresolved value template, firing path and Matches.
func TestProbePR6UnresolvedEntityTemplate(t *testing.T) {
    c := []expression.ConditionExpression{{Field: "a.b.c", Operator: "eq", Value: "$entity.triple.x.y.missing"}}
    r := probeRule(t, c, "")
    e := createTestEntityState("c360.p.d.s.t.1", []message.Triple{{Subject: "c360.p.d.s.t.1", Predicate: "a.b.c", Object: "$entity.triple.x.y.missing"}})
    m, err := Matches(context.Background(), Definition{ID: "probe", Type: "expression", Enabled: true, Conditions: c}, e)
    t.Logf("evaluateEntityState=%v matches=%v err=%v", r.EvaluateEntityState(context.Background(), e), m, err)
}

// P6: a transition condition through EvaluateEntityState.
func TestProbePR6Transition(t *testing.T) {
    c := []expression.ConditionExpression{{Field: "a.b.c", Operator: "transition", Value: "on", From: "off"}}
    r := probeRule(t, c, "")
    e := createTestEntityState("c360.p.d.s.t.1", []message.Triple{{Subject: "c360.p.d.s.t.1", Predicate: "a.b.c", Object: "on"}})
    t.Logf("transition=%v", r.EvaluateEntityState(context.Background(), e))
}

// P7: a disabled rule; a rule with no conditions.
func TestProbePR6DisabledAndEmpty(t *testing.T) {
    def := Definition{ID: "probe", Type: "expression", Name: "probe", Enabled: false,
        Conditions: []expression.ConditionExpression{{Field: "$message.x", Operator: "eq", Value: 1}}}
    r, _ := NewExpressionRule(testPlatform, "probe-pack", def)
    t.Logf("disabled=%v", r.Evaluate([]message.Message{createTestMessage(map[string]any{"x": 1})}))
    _, err := NewExpressionRule(testPlatform, "probe-pack", Definition{ID: "empty", Type: "expression", Enabled: true})
    t.Logf("empty-conditions construct err=%v", err)
}
```

## Observed

```text
probe_pr6_test.go:26: empty=false nonempty=false
probe_pr6_test.go:38: result=false infoLog=""
probe_pr6_test.go:44: literal-match=false
probe_pr6_test.go:50: no-pattern=[>] pattern=[]
probe_pr6_test.go:59: evaluateEntityState=true matches=false err=rule.Matches: definition "probe": condition on "a.b.c" has an unresolved value template "$entity.triple.x.y.missing"; it would be compared as a literal string and produce a confident wrong verdict
probe_pr6_test.go:67: transition=false
probe_pr6_test.go:75: disabled=false
probe_pr6_test.go:77: empty-conditions construct err=<nil>
```

## Product rule counts

semteams 4df44ae8 and semboids ab4e0fbc, every `configs/**/*.json` object with `type: expression`, read with a
Python script (`json.load`, no third-party code): 91 rules; 43 without `entity.pattern`, 48 with one; a `required: true`
condition in 36 of the 43 and 48 of the 48; 5 condition values holding `$`; 0 message rules with `$entity.` in a
field or value; 0 entity rules with `$message.` in a field or value; 0 empty condition lists.
