# Inventory probe: rule-core-03-context

Evidence for `inventory.md` § 2 and findings F3, F19 and F20, so it can be re-run on another machine. The runs were
made by the inventory reviewer (PR #759 comment 7168964321) on a scratch copy of the SemStreams pin `8b99efe9`
(`gh api repos/C360Studio/semstreams/tarball/8b99efe9`, unpacked). The probe file below reproduces the three cases;
the results are the reviewer's observations.

## PR 3's tests on the pin copy

In `processor/rule` of the pin copy, with `R` the 92 `Test*` names from PR 3's ten test files joined by `|`. The
`wc -l` line guards against an empty list: `-run "^()$"` runs nothing and still prints `ok`.

```bash
cd <pin copy>/processor/rule
R=$(grep -h -oE '^func (Test[A-Za-z0-9_]+)' for_each_substitution_test.go lifecycle_substitution_test.go state_tracker_test.go typed_substitution_test.go entity_substitution_test.go message_substitution_test.go caller_substitution_test.go triple_length_substitution_test.go triple_triples_substitution_test.go triple_value_substitution_test.go | sed 's/func //' | paste -sd'|' -)
echo "$R" | tr '|' '\n' | wc -l                                                     # 92
go test -count=1 -race -run "^($R)\$" .                                              # ok
go test -count=1 -race -v -run "^($R)\$" . 2>&1 | grep -c '^--- PASS: Test'          # 92
go test -count=5 -cpu=1 -shuffle=on -v -run "^($R)\$" . 2>&1 | grep -E '^-test.shuffle|^ok|^FAIL'
```

Observed (review round 2): 92 names; `-race` ok, 92 `--- PASS`; the shuffled run printed
`-test.shuffle 1795626557990464000` and `ok`. The probe file below was run as written and gave the results under
"Observed".

## Probe file

Saved as `processor/rule/probe_inventory_test.go` in the pin copy (indented with spaces here; `gofmt` restores
tabs), run with `go test -count=1 -v -run '^TestProbeInventory' .`, then deleted.

```go
package rule

import (
    "bytes"
    "context"
    "log/slog"
    "testing"

    gtypes "github.com/c360studio/semstreams/graph"
    "github.com/c360studio/semstreams/message"
)

const probeID = "c360.platform1.robotics.mav1.drone.001"

func probeEntity(triples ...message.Triple) *gtypes.EntityState {
    for i := range triples {
        triples[i].Subject = probeID
    }
    return &gtypes.EntityState{ID: probeID, Triples: triples}
}

// F3: a predicate stamped as three scalar triples.
func TestProbeInventoryF3(t *testing.T) {
    ec := &ExecutionContext{EntityID: probeID, Entity: probeEntity(
        message.Triple{Predicate: "gather.child.completed", Object: "a"},
        message.Triple{Predicate: "gather.child.completed", Object: "b"},
        message.Triple{Predicate: "gather.child.completed", Object: "c"},
    )}
    t.Logf("length=%q", ec.SubstituteVariables(context.Background(), "$entity.triple.gather.child.completed.length"))
    t.Logf("triples=%q", ec.SubstituteVariables(context.Background(), "$entity.triple.gather.child.completed.triples"))
}

// F19: a triple object holding a token from a later pass.
func TestProbeInventoryF19(t *testing.T) {
    ec := &ExecutionContext{
        EntityID:    probeID,
        Entity:      probeEntity(message.Triple{Predicate: "test.fixture.note", Object: "see $message.secret"}),
        MessageData: map[string]any{"secret": "LEAK"},
    }
    t.Logf("result=%q", ec.SubstituteVariables(context.Background(), "note: $entity.triple.test.fixture.note"))
}

// F20: a reference to an absent predicate that extends a present one. Not parallel: it replaces the process logger.
func TestProbeInventoryF20(t *testing.T) {
    var logs bytes.Buffer
    prev := slog.Default()
    slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
    t.Cleanup(func() { slog.SetDefault(prev) })

    ec := &ExecutionContext{EntityID: probeID, Entity: probeEntity(
        message.Triple{Predicate: "agent.lineage.researcher-plan", Object: "P"},
    )}
    t.Logf("result=%q", ec.SubstituteVariables(context.Background(), "$entity.triple.agent.lineage.researcher-plan-entity"))
    t.Logf("log=%q", logs.String())
}
```

## Observed

| Case | Template | Result |
| --- | --- | --- |
| F3 | `$entity.triple.gather.child.completed.length` | `"[ERROR_LENGTH_NOT_LIST:gather.child.completed]"` |
| F3 | `$entity.triple.gather.child.completed.triples` | `["a","b","c"]` |
| F19 | `note: $entity.triple.test.fixture.note`, object `"see $message.secret"`, `MessageData{"secret":"LEAK"}` | `"note: see LEAK"` |
| F20 | `$entity.triple.agent.lineage.researcher-plan-entity`, entity has `agent.lineage.researcher-plan` = `P` | `"P-entity"`, no warning logged |

Not run by the inventory's author (the session's permission system refused running pin tests); the file text above is
a reproduction of the reviewer's probe, the results are the reviewer's.
