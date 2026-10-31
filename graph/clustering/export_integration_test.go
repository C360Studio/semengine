//go:build integration

package clustering

// FixtureClient gives the package-external TestMaintainerAsAProduct this package's shared
// NATS server, reached through its reset.
var FixtureClient = fixtureClient
