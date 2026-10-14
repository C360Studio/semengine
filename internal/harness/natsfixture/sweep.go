package natsfixture

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/client"
	"github.com/testcontainers/testcontainers-go"
)

const (
	// testLabel marks every container natsfixture starts. A container a killed run leaves behind
	// (no t.Cleanup runs after SIGINT or a -timeout) is found by it, never by its name.
	testLabel = "org.c360studio.semengine.test"
	// sweepAge is how long ago Docker must have created a labelled container before the sweep
	// removes it (owner ruling, #205 comment 6132995614): three times go test's default 10-minute
	// -timeout, so a run under that timeout has ended before its containers qualify. Age is the only
	// test: a run with a longer or disabled -timeout gets no protection while it is still going.
	sweepAge = 30 * time.Minute
)

var dockerOnce sync.Once

// prepareDocker runs once per process, before natsfixture's first testcontainers call. It turns
// Ryuk off, whatever the caller set: testcontainers reads its configuration once per process, at
// its first call, and the environment wins over ~/.testcontainers.properties
// (internal/config/config.go:103-127 in testcontainers-go v0.44.0; TestRyukIsOff). It then sweeps
// labelled containers older than sweepAge. It fails nothing; what goes wrong goes to logf.
func prepareDocker(ctx context.Context, logf func(format string, args ...any)) {
	dockerOnce.Do(func() {
		if err := os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true"); err != nil {
			logf("natsfixture: turning Ryuk off: %v", err)
		}
		sweep(ctx, testLabel, sweepAge, logf)
	})
}

// labelValue names the test process that starts a container: its package and process ID. go test
// names a test binary's main package <import path>.test (TestFixtureContainerIsLabelled).
func labelValue() string {
	pkg := "unknown package"
	if info, ok := debug.ReadBuildInfo(); ok {
		pkg = strings.TrimSuffix(info.Path, ".test")
	}
	return fmt.Sprintf("%s pid %d", pkg, os.Getpid())
}

// sweep removes, with force, every container carrying label (a key, or key=value) that Docker
// created more than age ago, running or not, judged by Docker's creation time.
func sweep(ctx context.Context, label string, age time.Duration, logf func(format string, args ...any)) {
	cli, err := testcontainers.NewDockerClientWithOpts(ctx)
	if err != nil {
		logf("natsfixture: sweep: Docker client: %v", err)
		return
	}
	defer func() { _ = cli.Close() }()
	list, err := cli.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: make(client.Filters).Add("label", label)})
	if err != nil {
		logf("natsfixture: sweep: list containers labelled %s: %v", label, err)
		return
	}
	for _, c := range list.Items {
		if time.Since(time.Unix(c.Created, 0)) > age {
			removeContainer(ctx, cli, c.ID, logf)
		}
	}
}

// removeContainer removes one container with force. A container already gone is ignored, and
// any other error goes to logf: a removal error never fails a test.
func removeContainer(ctx context.Context, cli *testcontainers.DockerClient, id string, logf func(format string, args ...any)) {
	if _, err := cli.ContainerRemove(ctx, id, client.ContainerRemoveOptions{Force: true}); err != nil && !cerrdefs.IsNotFound(err) {
		logf("natsfixture: sweep: remove container %s: %v", id, err)
	}
}
