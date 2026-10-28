//go:build integration

package objectstore

import "github.com/c360studio/semengine/internal/harness/natsfixture"

// SharedServer is this package's NATS server, for the tests of both objectstore and
// objectstore_test: the two packages build into one test binary, so they share one server, and
// the external tests reach it as objectstore.SharedServer. It is declared here, in the package
// itself, because the Component's tests read its unexported handles. It starts at the first
// reset; TestMain (store_integration_test.go) stops it after the tests.
var SharedServer natsfixture.Shared
