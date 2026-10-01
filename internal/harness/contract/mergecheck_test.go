package contract

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// merge-gate › "Known-flake check" and "Up-to-date rule": scripts/merge-check.sh runs from a
// throwaway root with a fake gh first on PATH. One case per scenario of the two requirements. The
// fake serves canned answers from files and refuses a call whose arguments differ from the reads
// the script must make, so a case cannot pass on an answer to the wrong question.

const fakeGH = `#!/bin/sh
d="$FAKE_GH_DIR"
echo "$*" >> "$d/argv.log"
args=" $* "
need() { case "$args" in *" $1 "*) ;; *) echo "fake gh: '$*' lacks '$1'" >&2; exit 3 ;; esac; }
case "$1 $2" in
  "label list")
    key=labels; need "--json name"
    # gh 2.97 prints nothing at all, not [], when --search matches no label (measured 2026-10-01).
    case "$args" in *" --search "*) exit 0 ;; esac ;;
  "issue list") key=issues; need "--label class:flake"; need "--state open"; need "--limit 100" ;;
  "pr view")
    key=pr
    [ "$3" = "$FAKE_GH_PR" ] || { echo "fake gh: pr view of '$3', want '$FAKE_GH_PR'" >&2; exit 3; }
    need "--json closingIssuesReferences" ;;
  *)
    case "$1 $2" in
      "api repos/{owner}/{repo}/rules/branches/main") key=rules ;;
      "api repos/{owner}/{repo}/rulesets/"*) key=ruleset ;;
      *) echo "fake gh: unexpected call: $*" >&2; exit 3 ;;
    esac ;;
esac
if [ -e "$d/$key.fail" ]; then echo "HTTP 502: Bad Gateway (fake)" >&2; exit 1; fi
cat "$d/$key.json"
`

const (
	flakeRepo = "https://github.com/C360Studio/semengine/issues/"
	otherRepo = "https://github.com/C360Studio/semstreams/issues/"
)

func issueJSON(n int) string {
	return fmt.Sprintf(`{"number":%d,"url":"%s%d","title":"flaky test %d"}`, n, flakeRepo, n, n)
}

func refJSON(url string, n int) string {
	return fmt.Sprintf(`{"number":%d,"url":"%s%d","repository":{"name":"x","owner":{"login":"C360Studio"}}}`, n, url, n)
}

func list(items ...string) string { return "[" + strings.Join(items, ",") + "]" }

// ghState is what the fake gh answers. Every field holds the literal JSON, so a case can plant an
// answer of the wrong shape.
type ghState struct {
	labels, issues, pr, rules, ruleset string
	fail                               []string // keys whose read exits non-zero
}

const (
	requiredRule = `{"type":"required_status_checks","ruleset_id":24272345,"ruleset_source":"C360Studio/semengine",` +
		`"parameters":{"strict_required_status_checks_policy":true,"do_not_enforce_on_create":false,` +
		`"required_status_checks":[{"context":"Required","integration_id":15368}]}}`
	otherRules = `{"type":"deletion","ruleset_id":24272345},{"type":"non_fast_forward","ruleset_id":24272345}`
)

func healthy() ghState {
	return ghState{
		labels:  list(`{"name":"bug"}`, `{"name":"class:flake"}`),
		issues:  list(),
		pr:      `{"closingIssuesReferences":[]}`,
		rules:   list(otherRules, requiredRule),
		ruleset: `{"id":24272345,"name":"main","enforcement":"active"}`,
	}
}

type mergeRun struct {
	out    string
	status int
	argv   string
}

// runMergeCheck runs a copy of the script with the fake gh. GITHUB_ACTIONS and GITHUB_EVENT_NAME
// are removed from the inherited environment (CI sets both) and set only as the case says.
func runMergeCheck(t *testing.T, st ghState, pr string, actionsEnv ...string) mergeRun {
	t.Helper()
	root := copyScript(t, "merge-check.sh")
	dir := t.TempDir()
	for key, body := range map[string]string{"labels": st.labels, "issues": st.issues, "pr": st.pr, "rules": st.rules, "ruleset": st.ruleset} {
		if err := os.WriteFile(filepath.Join(dir, key+".json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, key := range st.fail {
		if err := os.WriteFile(filepath.Join(dir, key+".fail"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var env []string
	for _, kv := range fakeBin(t, map[string]string{"gh": fakeGH}, "FAKE_GH_DIR="+dir, "FAKE_GH_PR="+pr) {
		if strings.HasPrefix(kv, "GITHUB_ACTIONS=") || strings.HasPrefix(kv, "GITHUB_EVENT_NAME=") {
			continue
		}
		env = append(env, kv)
	}
	args := []string{filepath.Join(root, "scripts", "merge-check.sh")}
	if pr != "" {
		args = append(args, pr)
	}
	cmd := exec.Command("bash", args...)
	cmd.Dir = root
	cmd.Env = append(env, actionsEnv...)
	out, err := cmd.CombinedOutput()
	r := mergeRun{out: string(out)}
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		r.status = exit.ExitCode()
	case err != nil:
		t.Fatalf("run merge-check.sh: %v", err)
	}
	argv, _ := os.ReadFile(filepath.Join(dir, "argv.log"))
	r.argv = string(argv)
	return r
}

func (r mergeRun) requirePass(t *testing.T, fragments ...string) {
	t.Helper()
	if r.status != 0 {
		t.Fatalf("exit %d, want 0\n%s", r.status, r.out)
	}
	r.requireOutput(t, fragments...)
}

func (r mergeRun) requireFail(t *testing.T, fragments ...string) {
	t.Helper()
	if r.status == 0 {
		t.Fatalf("exit 0, want non-zero\n%s", r.out)
	}
	r.requireOutput(t, fragments...)
}

func (r mergeRun) requireOutput(t *testing.T, fragments ...string) {
	t.Helper()
	for _, f := range fragments {
		if !strings.Contains(r.out, f) {
			t.Errorf("output does not contain %q:\n%s", f, r.out)
		}
	}
}

func TestMergeCheckKnownFlake(t *testing.T) {
	open40 := healthy()
	open40.issues = list(issueJSON(40))
	open40And52 := healthy()
	open40And52.issues = list(issueJSON(40), issueJSON(52))
	closes := func(st ghState, refs ...string) ghState {
		st.pr = `{"closingIssuesReferences":` + list(refs...) + `}`
		return st
	}

	t.Run("open flake and an unrelated pull request", func(t *testing.T) {
		runMergeCheck(t, closes(open40, refJSON(flakeRepo, 7)), "12").requireFail(t, "#40")
	})
	t.Run("pull request that closes the only open flake", func(t *testing.T) {
		r := runMergeCheck(t, closes(open40, refJSON(flakeRepo, 40)), "12")
		r.requirePass(t, "closing #40 exempts pull request #12", "::warning::")
		if !strings.Contains(r.out[strings.Index(r.out, "::warning::"):], "#40") {
			t.Errorf("the ::warning:: line does not name #40:\n%s", r.out)
		}
	})
	t.Run("two open flakes and a pull request that closes one", func(t *testing.T) {
		r := runMergeCheck(t, closes(open40And52, refJSON(flakeRepo, 40)), "12")
		r.requireFail(t, "#52")
		if strings.Contains(r.out, "not closed by this pull request: #40") {
			t.Errorf("#40 is closed by the pull request but reported as not closed:\n%s", r.out)
		}
	})
	t.Run("two open flakes and a pull request that closes both", func(t *testing.T) {
		runMergeCheck(t, closes(open40And52, refJSON(flakeRepo, 40), refJSON(flakeRepo, 52)), "12").
			requirePass(t, "closing #40 exempts", "closing #52 exempts")
	})
	t.Run("same number in another repository", func(t *testing.T) {
		runMergeCheck(t, closes(open40, refJSON(otherRepo, 40)), "12").requireFail(t, "#40")
	})
	t.Run("pull request on another base", func(t *testing.T) {
		st := open40
		st.pr = `{"baseRefName":"claude/stacked","closingIssuesReferences":[]}`
		runMergeCheck(t, st, "12").requireFail(t, "#40")
	})
	t.Run("no open flake", func(t *testing.T) {
		runMergeCheck(t, healthy(), "12").requirePass(t, "no open class:flake issue")
	})
	t.Run("label missing", func(t *testing.T) {
		st := open40
		st.labels = list(`{"name":"bug"}`)
		runMergeCheck(t, st, "12").requireFail(t, "label class:flake is missing")
	})
	t.Run("a read fails", func(t *testing.T) {
		for _, tc := range []struct{ key, read string }{
			{"issues", "open class:flake issues"}, {"pr", "pull request #12"}, {"labels", "labels"},
		} {
			for _, how := range []string{"fails", "not a list"} {
				st := open40 // an open flake, so every read is reached
				switch how {
				case "fails":
					st.fail = []string{tc.key}
				default:
					// An error object where a list was asked for, as the API returns one.
					bad := `{"message":"Not Found","status":"404"}`
					switch tc.key {
					case "issues":
						st.issues = bad
					case "labels":
						st.labels = bad
					case "pr":
						st.pr = `{"closingIssuesReferences":` + bad + `}`
					}
				}
				r := runMergeCheck(t, st, "12")
				r.requireFail(t, "unavailable", tc.read)
				if strings.Contains(r.out, "no open class:flake issue") {
					t.Errorf("%s %s: reported no known flake:\n%s", tc.read, how, r.out)
				}
			}
		}
	})
	t.Run("list may be cut short", func(t *testing.T) {
		st := healthy()
		var hundred []string
		for n := 1000; n < 1100; n++ {
			hundred = append(hundred, issueJSON(n))
		}
		st.issues = list(hundred...)
		runMergeCheck(t, st, "12").requireFail(t, "may be incomplete")
	})
	t.Run("push run", func(t *testing.T) {
		r := runMergeCheck(t, open40, "", "GITHUB_ACTIONS=true", "GITHUB_EVENT_NAME=push")
		r.requirePass(t, "push run", "known-flake check does not apply")
		if strings.Contains(r.argv, "issue list") || strings.Contains(r.argv, "pr view") {
			t.Errorf("a push run read issues or a pull request:\n%s", r.argv)
		}
		if !strings.Contains(r.argv, "rules/branches/main") {
			t.Errorf("a push run did not read the rules in force on main:\n%s", r.argv)
		}
	})
	t.Run("push event named outside Actions", func(t *testing.T) {
		r := runMergeCheck(t, open40, "12", "GITHUB_EVENT_NAME=push")
		r.requireFail(t, "pull-request run", "#40")
		if first, _, _ := strings.Cut(r.out, "\n"); !strings.Contains(first, "pull-request run") {
			t.Errorf("first line does not name the kind of run: %q", first)
		}
	})
	t.Run("pull-request run without a number", func(t *testing.T) {
		runMergeCheck(t, open40, "", "GITHUB_ACTIONS=true", "GITHUB_EVENT_NAME=pull_request").
			requireFail(t, "a pull request number is required")
	})
	t.Run("local run without a number", func(t *testing.T) {
		runMergeCheck(t, healthy(), "").requireFail(t, "a pull request number is required")
	})
	t.Run("event with no rule", func(t *testing.T) {
		runMergeCheck(t, healthy(), "12", "GITHUB_ACTIONS=true", "GITHUB_EVENT_NAME=merge_group").
			requireFail(t, "merge_group")
	})
	t.Run("no event inside Actions", func(t *testing.T) {
		runMergeCheck(t, healthy(), "12", "GITHUB_ACTIONS=true").requireFail(t, "the event is missing")
	})
}

func TestMergeCheckUpToDateRule(t *testing.T) {
	t.Run("setting turned off in GitHub", func(t *testing.T) {
		st := healthy()
		st.rules = list(otherRules, strings.Replace(requiredRule, `"strict_required_status_checks_policy":true`, `"strict_required_status_checks_policy":false`, 1))
		runMergeCheck(t, st, "12").requireFail(t, "strict_required_status_checks_policy=false", "ruleset_id=24272345")
	})
	t.Run("no rule requires the check", func(t *testing.T) {
		st := healthy()
		st.rules = list(otherRules, strings.Replace(requiredRule, `"context":"Required"`, `"context":"Verify"`, 1))
		runMergeCheck(t, st, "12").requireFail(t, "no rule in force on main requires Required")
	})
	t.Run("ruleset not active", func(t *testing.T) {
		for _, enforcement := range []string{"evaluate", "disabled"} {
			st := healthy()
			st.ruleset = `{"id":24272345,"name":"main","enforcement":"` + enforcement + `"}`
			r := runMergeCheck(t, st, "12")
			r.requireFail(t, "enforcement="+enforcement)
			if !strings.Contains(r.argv, "rulesets/24272345") {
				t.Errorf("did not read the ruleset the rule names:\n%s", r.argv)
			}
		}
	})
	t.Run("field the script does not read", func(t *testing.T) {
		st := healthy()
		st.rules = list(otherRules, strings.Replace(requiredRule, `"parameters":{`, `"surprise":{"new":1},"parameters":{"another_new_field":[1,2],`, 1))
		st.ruleset = `{"id":24272345,"enforcement":"active","bypass_actors":[{"actor_id":1}],"new_field":"x"}`
		runMergeCheck(t, st, "12").requirePass(t, "enforcement=active")
	})
}
