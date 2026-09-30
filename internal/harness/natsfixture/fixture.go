// Package natsfixture gives one test one disposable, admitted, real NATS server in a container
// (openspec change setup-02-isolated-harness, capability nats-fixture). It is test-only: contract
// test T-B1 refuses any production import of it.
package natsfixture

import (
	"testing"
)

// Fixture is one test's NATS server and the resources it creates on it.
type Fixture struct {
	testName string
}

// New binds a fixture to its test. It makes no Docker call.
func New(t testing.TB) *Fixture {
	t.Helper()
	return &Fixture{testName: t.Name()}
}
