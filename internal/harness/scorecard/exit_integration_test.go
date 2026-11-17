//go:build integration

package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	fakeModel  = "fake-embed-v1"
	fakePrefix = "Q: "
)

// fakeProvider is an embedding service shaped like semembed's OpenAI-style API that gives every
// text the same vector, so the vectors rank by entity ID alone. With failQuestions it refuses any
// request holding a question (a text with the query prefix): sections embed, then every semantic
// search fails, as when the provider stops once the vectors are generated.
func fakeProvider(t *testing.T, failQuestions bool) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if r.URL.Path != "/v1/embeddings" || json.NewDecoder(r.Body).Decode(&request) != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		type object struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		}
		reply := struct {
			Data  []object `json:"data"`
			Model string   `json:"model"`
		}{Model: request.Model}
		for i, text := range request.Input {
			if failQuestions && strings.HasPrefix(text, fakePrefix) {
				http.Error(w, `{"error":{"message":"provider stopped"}}`, http.StatusServiceUnavailable)
				return
			}
			reply.Data = append(reply.Data, object{Embedding: []float32{1, 0, 0, 0}, Index: i})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(reply)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/v1"
}

// runProgram builds the program and runs it from the module root, as Task does, against the
// provider at url, and returns its exit status and its standard output and error.
func runProgram(t *testing.T, url string) (int, string, string) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "scorecard")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	cmd := exec.Command(bin, "-commit", strings.Repeat("0", 40), "-tree", "dirty",
		"-url", url, "-model", fakeModel, "-query-prefix", fakePrefix, "-provider", "fake")
	cmd.Dir = root
	cmd.Env = os.Environ()
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return 0, stdout.String(), stderr.String()
	case errors.As(err, &exit):
		return exit.ExitCode(), stdout.String(), stderr.String()
	}
	t.Fatalf("run: %v", err)
	return 0, "", ""
}

// The built program exits 1, after printing the three rows, when the merged row is below the floor:
// vectors that rank by ID alone carry the merged row below 60%. The semantic verb's copy is awaited
// and answers, so the run reaches the questions.
func TestFloorMissExitsOne(t *testing.T) {
	code, stdout, stderr := runProgram(t, fakeProvider(t, false))
	if code != 1 {
		t.Fatalf("exit %d, want 1\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	for _, want := range []string{"words, globalSearch", "vectors, semantic", "merged, searchGraph", "verdict: below the floor"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout does not hold %q:\n%s", want, stdout)
		}
	}
}

// A provider that fails once the vectors are generated ends the run with exit 2, naming the
// failure, with every cleanup run and the container removed; the wait for the semantic verb does
// not ask again.
func TestProviderLossAfterGenerationExitsTwo(t *testing.T) {
	code, stdout, stderr := runProgram(t, fakeProvider(t, true))
	if code != 2 {
		t.Fatalf("exit %d, want 2\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "no verdict") || !strings.Contains(stderr, "503") {
		t.Errorf("stderr does not name the provider failure:\n%s", stderr)
	}
	if strings.Contains(stderr, "condition not met") {
		t.Errorf("the wait asked again after the provider failed, until its budget ran out:\n%s", stderr)
	}
	if strings.Contains(stderr, "a cleanup failed") || !strings.Contains(stderr, "Container terminated") {
		t.Errorf("cleanup did not end with the container removed:\n%s", stderr)
	}
	if strings.Contains(stdout, "verdict:") {
		t.Errorf("a run with no rows printed a verdict:\n%s", stdout)
	}
}
