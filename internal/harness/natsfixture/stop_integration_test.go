//go:build integration

package natsfixture

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// nats-fixture › "Own server per test", scenario "Container gone after the test": the Stop New
// registers as the test's cleanup removes the container. The oracle is the Docker CLI.
func TestContainerGoneAfterTheTest(t *testing.T) {
	var id string
	t.Run("own server", func(t *testing.T) {
		f := startFixture(t)
		id = f.containerID
		if !containerExists(t, id) {
			t.Fatalf("container %s missing while its test runs", id)
		}
	})
	if id == "" {
		t.Fatal("the subtest recorded no container ID")
	}
	if containerExists(t, id) {
		t.Fatalf("container %s still exists after its test ended", id)
	}
}

// nats-fixture › "Own server per test": Stop closes the connection and removes the container,
// nothing more. Removing the container removes every stream in it, so Stop needs no answer from
// the server: a paused container answers nothing, and Stop still removes it.
func TestStopNeedsNoAnswerFromTheServer(t *testing.T) {
	f := startFixture(t)
	if _, err := CreateStream(t.Context(), f.JetStream(), jetstream.FileStorage, "orders", "orders.>"); err != nil {
		t.Fatal(err)
	}
	id, nc := f.containerID, conn(f)
	if out, err := exec.Command("docker", "pause", id).CombinedOutput(); err != nil {
		t.Fatalf("docker pause %s: %v\n%s", id, err, out)
	}
	// Ample for closing a connection and removing a container; a Stop that waits on the paused
	// server for anything ends here instead.
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	if err := f.Stop(ctx); err != nil {
		t.Fatalf("Stop on a paused server: %v", err)
	}
	if !nc.IsClosed() {
		t.Fatal("Stop returned nil with the fixture's connection open")
	}
	if containerExists(t, id) {
		t.Fatalf("container %s exists after Stop returned nil", id)
	}
}
