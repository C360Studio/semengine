package vocabulary

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test constants and namespace definitions
func TestNamespaceConstants(t *testing.T) {
	// Verify namespace constants are properly defined
	assert.Equal(t, "https://semstreams.semanticstream.ing", SemStreamsBase)
	// RoboticsNamespace moved to domain modules (semops, streamkit-robotics)
	// assert.Equal(t, SemStreamsBase+"/robotics", RoboticsNamespace)
}
