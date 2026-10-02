package contract

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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
    case "$*" in
      "api repos/{owner}/{repo}/rules/branches/main") key=rules ;;
      "api repos/{owner}/{repo}/rulesets/"*) key=ruleset ;;
      "api repos/{owner}/{repo}/pulls/$FAKE_GH_PR") key=pull ;;
      # Without --paginate gh returns the first page only.
      "api --paginate repos/{owner}/{repo}/pulls/$FAKE_GH_PR/files?per_page=100") key=files ;;
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
	// pull is the pull request as the REST API returns it; files is the paged list of its changed
	// files exactly as gh prints it, so a case can plant two pages one after the other.
	pull, files string
	fail        []string // keys whose read exits non-zero
	// jqFail, when set, puts a jq first on PATH that exits non-zero on any call whose arguments
	// contain it and passes every other call to the real jq.
	jqFail string
}

const fakeJQ = `#!/bin/sh
case "$*" in *"$FAKE_JQ_FAIL"*) echo "jq: error (fake)" >&2; exit 5 ;; esac
exec "$REAL_JQ" "$@"
`

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
		pull:    pullJSON(false, "User", "", 1),
		files:   filesJSON("docs/a.md"),
	}
}

// pullJSON is the part of GET /repos/{owner}/{repo}/pulls/{n} the review check reads, with fields
// it does not read beside them, as measured on #48 and #14 (inventory 2.4).
func pullJSON(draft bool, userType, body string, changedFiles int) string {
	b, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf(`{"number":12,"state":"open","draft":%t,"user":{"login":"x","type":%q},"body":%s,"changed_files":%d,"base":{"ref":"main"}}`,
		draft, userType, b, changedFiles)
}

// filesJSON is one page of GET /repos/{owner}/{repo}/pulls/{n}/files, each name modified.
func filesJSON(names ...string) string {
	entries := make([]string, len(names))
	for i, n := range names {
		entries[i] = fmt.Sprintf(`{"filename":%q,"status":"modified","additions":1,"deletions":0}`, n)
	}
	return list(entries...)
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
	for key, body := range map[string]string{"labels": st.labels, "issues": st.issues, "pr": st.pr, "rules": st.rules, "ruleset": st.ruleset, "pull": st.pull, "files": st.files} {
		if err := os.WriteFile(filepath.Join(dir, key+".json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, key := range st.fail {
		if err := os.WriteFile(filepath.Join(dir, key+".fail"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tools := map[string]string{"gh": fakeGH}
	extra := []string{"FAKE_GH_DIR=" + dir, "FAKE_GH_PR=" + pr}
	if st.jqFail != "" {
		realJQ, err := exec.LookPath("jq")
		if err != nil {
			t.Fatalf("jq is needed to run merge-check.sh: %v", err)
		}
		tools["jq"] = fakeJQ
		extra = append(extra, "REAL_JQ="+realJQ, "FAKE_JQ_FAIL="+st.jqFail)
	}
	var env []string
	for _, kv := range fakeBin(t, tools, extra...) {
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

// requireReadPullAndFiles fails unless the run read pull request 12 and every page of its files.
func (r mergeRun) requireReadPullAndFiles(t *testing.T) {
	t.Helper()
	for _, call := range []string{
		"api repos/{owner}/{repo}/pulls/12\n",
		"api --paginate repos/{owner}/{repo}/pulls/12/files?per_page=100\n",
	} {
		if !strings.Contains(r.argv, call) {
			t.Errorf("the run did not call gh %q:\n%s", strings.TrimSpace(call), r.argv)
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
		// The script lists each flake not closed on its own line as "  #<n> <url> <title>".
		if strings.Contains(r.out, "\n  #40 ") {
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
		r := runMergeCheck(t, healthy(), "12")
		r.requirePass(t, "no open class:flake issue")
		r.requireReadPullAndFiles(t)
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
	t.Run("jq fails comparing the flakes with the closing references", func(t *testing.T) {
		// An empty result from a failed jq would read as "every open flake is closed".
		for _, tc := range []struct{ name, args, says string }{
			{"closing references", ".closingIssuesReferences[].url", "jq failed reading the closing references"},
			{"flakes not closed", "--argjson c", "jq failed comparing the open class:flake issues"},
		} {
			st := closes(open40, refJSON(flakeRepo, 7))
			st.jqFail = tc.args
			r := runMergeCheck(t, st, "12")
			if r.status != 2 {
				t.Errorf("%s: exit %d, want 2\n%s", tc.name, r.status, r.out)
			}
			r.requireOutput(t, "unavailable", tc.says)
			if strings.Contains(r.out, "exempts") || strings.Contains(r.out, "merge-check: ok") {
				t.Errorf("%s: a failed jq exempted the pull request:\n%s", tc.name, r.out)
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
		if strings.Contains(r.argv, "issue list") || strings.Contains(r.argv, "pr view") || strings.Contains(r.argv, "/pulls/") {
			t.Errorf("a push run read issues, a pull request or its files:\n%s", r.argv)
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
		r := runMergeCheck(t, st, "12")
		r.requirePass(t, "enforcement=active")
		r.requireReadPullAndFiles(t)
	})
}

// merge-gate › "Cross-agent review check": one case per scenario. Each case writes GitHub's answers
// by hand; the expected exit and words come from the scenario, never from running the script.

// review is healthy() with the pull request and its files replaced: changed_files is the number of
// entries unless a case sets it.
func review(draft bool, author, body string, files ...string) ghState {
	st := healthy()
	st.pull = pullJSON(draft, author, body, len(files))
	st.files = filesJSON(files...)
	return st
}

const codeFilePrefix = "merge-check: code file: "

// requireCodeNames fails unless the code names the script prints are exactly want, in any order.
func (r mergeRun) requireCodeNames(t *testing.T, want ...string) {
	t.Helper()
	var got []string
	for _, line := range strings.Split(r.out, "\n") {
		if name, ok := strings.CutPrefix(line, codeFilePrefix); ok {
			got = append(got, name)
		}
	}
	slices.Sort(got)
	want = slices.Clone(want)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("code names printed %q, want %q\n%s", got, want, r.out)
	}
}

func (r mergeRun) requireNotInOutput(t *testing.T, fragments ...string) {
	t.Helper()
	for _, f := range fragments {
		if strings.Contains(r.out, f) {
			t.Errorf("output contains %q:\n%s", f, r.out)
		}
	}
}

func TestMergeCheckReview(t *testing.T) {
	const implClaude = "Summary.\n\nimplemented-by: claude (opus)\n"

	// Which pull requests are covered (task 2.2).
	t.Run("documents only", func(t *testing.T) {
		r := runMergeCheck(t, review(false, "User", "", "docs/a.md", "openspec/config.yaml"), "12")
		r.requirePass(t, "documents only")
		r.requireCodeNames(t)
	})
	t.Run("one code file among documents", func(t *testing.T) {
		r := runMergeCheck(t, review(false, "User", implClaude, "docs/a.md", "scripts/x.sh"), "12")
		r.requireFail(t, "is a code pull request", "no line starts reviewed-by:")
		r.requireCodeNames(t, "scripts/x.sh")
	})
	t.Run("names near the rule", func(t *testing.T) {
		docs := []string{"AGENTS.md", ".claude/skills/preflight/SKILL.md", "x/.claude/agents/a.md",
			"openspec/changes/x/.openspec.yaml", "openspec/x.sh"}
		first := runMergeCheck(t, review(false, "User", "", docs...), "12")
		first.requirePass(t, "documents only")
		first.requireCodeNames(t)

		code := []string{"README.MD", "docs/a.md.txt", "openspecs/a.yaml", "docs/openspec/a.yaml", "Taskfile.yml",
			"docs/admission-ledger.yaml", ".claude/agents/semengine-reviewer.md", ".codex/agents/semengine-reviewer.toml"}
		st := review(false, "User", "", append(slices.Clone(docs), code...)...)
		entries := strings.TrimSuffix(st.files, "]") + `,{"filename":"scripts/x.sh","status":"removed","additions":0,"deletions":9}]`
		st.files = entries
		st.pull = pullJSON(false, "User", "", len(docs)+len(code)+1)
		second := runMergeCheck(t, st, "12")
		second.requireFail(t, "is a code pull request")
		second.requireNotInOutput(t, "documents only")
		second.requireCodeNames(t, append(code, "scripts/x.sh")...)
	})
	t.Run("renamed file", func(t *testing.T) {
		st := review(false, "User", "")
		st.pull = pullJSON(false, "User", "", 1)
		st.files = `[{"filename":"docs/x.md","previous_filename":"scripts/x.sh","status":"renamed","additions":0,"deletions":0}]`
		r := runMergeCheck(t, st, "12")
		r.requireFail(t, "is a code pull request")
		r.requireCodeNames(t, "scripts/x.sh")
	})
	t.Run("code file on the second page", func(t *testing.T) {
		var md []string
		for n := range 100 {
			md = append(md, fmt.Sprintf("docs/n%03d.md", n))
		}
		for _, pages := range []struct{ how, files string }{
			{"joined", filesJSON(append(slices.Clone(md), "go.mod")...)},
			{"one after the other", filesJSON(md...) + filesJSON("go.mod")},
		} {
			st := review(false, "User", "")
			st.pull = pullJSON(false, "User", "", 101)
			st.files = pages.files
			r := runMergeCheck(t, st, "12")
			if r.status == 0 {
				t.Errorf("%s: exit 0, want non-zero\n%s", pages.how, r.out)
			}
			r.requireOutput(t, "is a code pull request")
			r.requireCodeNames(t, "go.mod")
		}
	})
	t.Run("file list incomplete", func(t *testing.T) {
		var md []string
		for n := range 100 {
			md = append(md, fmt.Sprintf("docs/n%03d.md", n))
		}
		st := review(false, "User", "", md...)
		st.pull = pullJSON(false, "User", "", 101)
		r := runMergeCheck(t, st, "12")
		r.requireFail(t, "incomplete", "100 of 101 files")
		r.requireNotInOutput(t, "documents only")
	})
	t.Run("no changed file", func(t *testing.T) {
		r := runMergeCheck(t, review(false, "User", ""), "12")
		r.requirePass(t, "documents only")
		r.requireCodeNames(t)
	})
}
