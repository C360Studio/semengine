//go:build integration

package natsfixture

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
)

// processLabel is the label natsfixture puts on every container this test process starts,
// spelled here from the requirement (the package and the process ID), not read from the fixture.
func processLabel() string {
	return fmt.Sprintf("org.c360studio.semengine.test=github.com/c360studio/semengine/internal/harness/natsfixture pid %d", os.Getpid())
}

// processContainers lists, from Docker, every container carrying this process's label, running or
// not. Tests in a package run one at a time, so it is the containers of the running test.
func processContainers(t *testing.T) []string {
	t.Helper()
	out, err := exec.Command("docker", "ps", "-a", "--no-trunc", "--filter", "label="+processLabel(), "--format", "{{.ID}}").Output()
	if err != nil {
		t.Fatalf("docker ps --filter label=%s: %v", processLabel(), err)
	}
	return strings.Fields(string(out))
}

// dockerRun starts a container from the pinned image with the docker CLI, an oracle independent
// of natsfixture, and removes it when the test ends. A failed removal fails the test: no sweep
// reaches a container without the label. `docker rm -f` exits zero for a container already gone,
// as the swept one in TestOnlyOldLabelledContainersAreSwept is by then.
func dockerRun(t *testing.T, args ...string) string {
	t.Helper()
	image, err := pinnedImage(imagePin)
	if err != nil {
		t.Fatal(err)
	}
	id, err := stdout(exec.CommandContext(t.Context(), "docker", append(append([]string{"run", "-d"}, args...), image)...))
	if err != nil {
		t.Fatalf("docker run: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), stopBound)
		defer cancel()
		if out, err := exec.CommandContext(ctx, "docker", "rm", "-f", id).CombinedOutput(); err != nil {
			t.Errorf("removing control container %s: docker rm -f: %v\n%s", id, err, out)
		}
	})
	return id
}

// nats-fixture › "Ryuk off, labelled containers and the sweep", scenario "Ryuk is off". The test
// sets no environment: testcontainers reads its configuration once per process, so what it reports
// here is what natsfixture set before its first testcontainers call.
func TestRyukIsOff(t *testing.T) {
	startFixture(t)
	if v, ok := testcontainers.GenericLabels()["org.testcontainers.reap"]; ok {
		t.Fatalf("after natsfixture's first container testcontainers labels org.testcontainers.reap=%s: Ryuk is on", v)
	}
}

// Every container natsfixture starts carries the label, valued with the package and process ID.
func TestFixtureContainerIsLabelled(t *testing.T) {
	f := startFixture(t)
	if got := processContainers(t); len(got) != 1 || got[0] != f.containerID {
		t.Fatalf("containers labelled %q: %v, want only the fixture's %s", processLabel(), got, f.containerID)
	}
}

// Scenario "Only old labelled containers are swept". The test's container carries a label value of
// its own, so its sweep at age zero reaches no other process's containers.
func TestOnlyOldLabelledContainersAreSwept(t *testing.T) {
	prepareDocker(t.Context(), t.Logf) // Ryuk off before this test's first testcontainers call, as Start does
	own := fmt.Sprintf("org.c360studio.semengine.test=%s pid %d", t.Name(), os.Getpid())
	labelled := dockerRun(t, "--label", own)
	unlabelled := dockerRun(t)

	sweep(t.Context(), own, time.Hour, t.Logf)
	if !containerExists(t, labelled) {
		t.Fatal("a sweep with an age of one hour removed a container created seconds ago")
	}
	sweep(t.Context(), own, 0, t.Logf)
	if containerExists(t, labelled) {
		// Docker records creation in its own clock (design P4); one ahead of the host's keeps it.
		created, _ := exec.Command("docker", "inspect", "--format", "{{.Created}}", labelled).Output()
		t.Fatalf("a sweep with an age of zero kept the labelled container, created %s by Docker; host time %s",
			strings.TrimSpace(string(created)), time.Now().UTC().Format(time.RFC3339Nano))
	}
	running, err := stdout(exec.Command("docker", "inspect", "--format", "{{.State.Running}}", unlabelled))
	if err != nil || running != "true" {
		t.Fatalf("the unlabelled container after the sweeps: running %q, %v; want it still running", running, err)
	}
}

// A removal error never fails a test: a container already gone is ignored, and any other error is
// written to the test output.
func TestSweepRemovalErrorsGoToTheLog(t *testing.T) {
	prepareDocker(t.Context(), t.Logf) // Ryuk off before this test's first testcontainers call, as Start does
	cli, err := testcontainers.NewDockerClientWithOpts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cli.Close() }()
	var lines []string
	logf := func(format string, args ...any) { lines = append(lines, fmt.Sprintf(format, args...)) }

	removeContainer(t.Context(), cli, "natsfixture-no-such-container", logf)
	if len(lines) != 0 {
		t.Fatalf("removing a container already gone wrote %q, want nothing", lines)
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	removeContainer(cancelled, cli, "natsfixture-cancelled-removal", logf)
	if len(lines) != 1 || !strings.Contains(lines[0], "natsfixture-cancelled-removal") {
		t.Fatalf("a failed removal wrote %q, want one line naming the container", lines)
	}
}
