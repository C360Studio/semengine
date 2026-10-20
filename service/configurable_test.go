package service

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/component"
)

// TestNewConfigSchema: NewConfigSchema keeps each property's component schema under its key, with
// the outer Category when one is set (as every pin caller sets it) and the embedded one otherwise,
// and the required list as given (design D6; early review finding).
func TestNewConfigSchema(t *testing.T) {
	port := component.PropertySchema{Type: "int", Description: "listen port", Default: 9090}
	path := component.PropertySchema{Type: "string", Description: "metrics path", Category: "advanced"}
	wantPort := port
	wantPort.Category = "basic"

	schema := NewConfigSchema(map[string]PropertySchema{
		"port": {PropertySchema: port, Category: "basic"},
		"path": {PropertySchema: path},
	}, []string{"port"})

	require.Equal(t, map[string]component.PropertySchema{"port": wantPort, "path": path}, schema.Properties)
	require.Equal(t, []string{"port"}, schema.Required)
	require.Nil(t, schema.ServiceSpecific)
}
