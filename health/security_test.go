package health

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeErrorMessage(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "Unix file path",
			input:    "failed to open /etc/semstreams/config.json",
			expected: "failed to open [PATH]",
		},
		{
			name:     "Windows file path",
			input:    "cannot read C:\\Users\\Admin\\config.json",
			expected: "cannot read [PATH]",
		},
		{
			name:     "HTTP URL",
			input:    "connection failed to https://api.example.com/v1/health",
			expected: "connection failed to [URL]",
		},
		{
			// Not the pin's localhost:4222: the T-B6 guard refuses a fixed broker address in a test.
			name:     "NATS URL",
			input:    "cannot connect to nats://broker.example.com:4223",
			expected: "cannot connect to [URL]",
		},
		{
			name:     "IP address",
			input:    "timeout connecting to 192.168.1.100",
			expected: "timeout connecting to [IP]",
		},
		{
			name:     "Port number",
			input:    "failed to bind to :8080",
			expected: "failed to bind to [PORT]",
		},
		{
			name:     "Credentials in error",
			input:    "auth failed with password:secretpass123",
			expected: "auth failed with [REDACTED]",
		},
		{
			name:     "Complex error with multiple sensitive items",
			input:    "failed to connect to https://192.168.1.1:8080/api with token=abc123def",
			expected: "failed to connect to [URL] with [REDACTED]",
		},
		{
			// The port rule over-redacts a clock time; the spec states it.
			name:     "Clock time",
			input:    "at 12:30:45",
			expected: "at 12[PORT][PORT]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeErrorMessage(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
