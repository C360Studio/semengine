package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestOpenAPIRegistryReturnsACopy: GetAllOpenAPISpecs returns a map the caller may change without
// changing the registry (design D6).
func TestOpenAPIRegistryReturnsACopy(t *testing.T) {
	const name = "registry-copy-test"
	spec := &OpenAPISpec{Paths: map[string]PathSpec{"/probe": {}}}
	RegisterOpenAPISpec(name, spec)
	t.Cleanup(func() {
		openAPIRegistryMu.Lock()
		defer openAPIRegistryMu.Unlock()
		delete(openAPIRegistry, name)
	})

	got := GetAllOpenAPISpecs()
	require.Same(t, spec, got[name])

	delete(got, name)
	got["added-by-caller"] = &OpenAPISpec{}

	again := GetAllOpenAPISpecs()
	require.Same(t, spec, again[name], "deleting from the returned map removed the registration")
	require.NotContains(t, again, "added-by-caller", "adding to the returned map registered a spec")
}
