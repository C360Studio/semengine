//go:build integration

package storagereport

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/natsclient"
)

// This file is the package's own copy of the natsclient integration helpers its integration test
// uses (natsclient/test_helpers_integration_test.go): test helpers of another package cannot be
// imported. Shared tests reset the package-wide broker via shared.Reset(t); a test that stops the
// broker starts its own via natsfixture.New.

// shared is this package's NATS server. It starts at the first reset; TestMain removes it after the tests.
var shared natsfixture.Shared

func TestMain(m *testing.M) { os.Exit(run(m)) }

func run(m *testing.M) int {
	code := m.Run()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := shared.Stop(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return code
}

type sharedFixture struct {
	url string
}

func (f sharedFixture) URL() string {
	return f.url
}

// startFixture resets the package's shared broker for t and returns its URL handle.
func startFixture(t *testing.T) sharedFixture {
	t.Helper()
	return sharedFixture{url: shared.Reset(t)}
}

// failureBound bounds a wait that a correct run reaches first. The lane runs under -race; a bound
// below 10 s is not a failure bound.
const failureBound = 10 * time.Second

// stopBound bounds a test's own Fixture.Stop, the one natsfixture.New's cleanup also gives it. It is
// a failure bound.
const stopBound = 60 * time.Second

// closeBudget bounds a test's Close (harness-boundaries, "Bounded cleanup roots").
const closeBudget = 10 * time.Second

// closeClient closes c under closeBudget, a bounded root: the test's own context ends only when
// the test does. A Close error fails the test.
func closeClient(t *testing.T, c *natsclient.Client) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), closeBudget)
	defer cancel()
	if err := c.Close(ctx); err != nil {
		t.Errorf("Close: %v", err)
	}
}
