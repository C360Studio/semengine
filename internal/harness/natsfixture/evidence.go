package natsfixture

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/testcontainers/testcontainers-go"
)

// record is the fixture's evidence file: what it started, how long each phase took, what it owned,
// and how Stop went. The runner collects the directory; nothing here is read back by the fixture.
type record struct {
	Test       string            `json:"test"`
	Name       string            `json:"name"`
	SessionID  string            `json:"testcontainers_session"`
	Image      string            `json:"image"`
	Attempts   []attemptRecord   `json:"attempts"`
	Container  string            `json:"container_id,omitempty"`
	MappedPort string            `json:"mapped_port,omitempty"`
	Owned      []string          `json:"owned"`
	Stops      []stopRecord      `json:"stops,omitempty"`
	StreamMsgs map[string]uint64 `json:"stream_msgs_before_delete,omitempty"`
	Remaining  []string          `json:"remaining"`
}

type attemptRecord struct {
	Attempt   int           `json:"attempt"`
	Phases    []phaseRecord `json:"phases"`
	Container string        `json:"container_id,omitempty"`
	Error     string        `json:"error,omitempty"`
	ParentErr string        `json:"parent_context_error,omitempty"`
	Cleanup   string        `json:"cleanup,omitempty"`
	Logs      string        `json:"logs_file,omitempty"`
}

type phaseRecord struct {
	Phase     string `json:"phase"`
	ElapsedMS int64  `json:"elapsed_ms"`
	Error     string `json:"error,omitempty"`
}

type stopRecord struct {
	Phases []phaseRecord `json:"phases"`
	Result string        `json:"result"`
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func since(start time.Time) int64 { return time.Since(start).Milliseconds() }

// recordSession appends this process's testcontainers session label, as key=value, before the
// first container exists. The runner's leak check selects containers by exactly these lines. The
// key is read from the labels testcontainers applies rather than spelled here: v0.40 labels
// containers org.testcontainers.sessionId, while its deprecated exported constant still says
// org.testcontainers.golang.sessionId, and a leak check on the wrong key passes silently. Every
// package's test binary may carry its own session, so the file is a set of lines, not a value.
func recordSession(dir string) (string, error) {
	sid := testcontainers.SessionID()
	label := ""
	for k, v := range testcontainers.GenericLabels() {
		if v == sid && sid != "" {
			label = k + "=" + v
		}
	}
	if label == "" {
		return sid, fmt.Errorf("testcontainers applies no label carrying session %q; the leak check could not find this run's containers", sid)
	}
	f, err := os.OpenFile(filepath.Join(dir, "testcontainers-session"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return sid, err
	}
	if _, err := fmt.Fprintln(f, label); err != nil {
		_ = f.Close()
		return sid, err
	}
	return sid, f.Close()
}

// writeRecord replaces the fixture's evidence file with the current record.
func writeRecord(dir, name string, r record) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	fixtures := filepath.Join(dir, "fixtures")
	if err := os.MkdirAll(fixtures, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(fixtures, name+".json"), data, 0o644)
}

// writeLogs stores a container's log beside its record and returns the file name.
func writeLogs(dir, name string, attempt int, logs []byte) (string, error) {
	fixtures := filepath.Join(dir, "fixtures")
	if err := os.MkdirAll(fixtures, 0o755); err != nil {
		return "", err
	}
	file := fmt.Sprintf("%s-attempt%d.log", name, attempt)
	return file, os.WriteFile(filepath.Join(fixtures, file), logs, 0o644)
}
