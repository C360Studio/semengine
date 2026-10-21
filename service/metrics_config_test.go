package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/metric"
)

func TestMetricsRejectsRetiredInnerEnabled(t *testing.T) {
	_, err := NewMetrics(json.RawMessage(`{"enabled":true}`), &Dependencies{})
	if err == nil || !strings.Contains(err.Error(), `unknown field "enabled"`) {
		t.Fatalf("retired enabled error = %v", err)
	}
}

// TestServiceConfigRefusesKeyInAnotherCase: a strictly decoded service payload refuses a key that
// differs only in letter case from its field's name, naming both spellings, at the top level and
// inside server_info; the exact spelling beside each variant decodes (design D5). encoding/json
// alone would decode {"Port":9091} into Port without a word.
//
// spec: service-lifecycle / A service configuration key in another letter case is refused
func TestServiceConfigRefusesKeyInAnotherCase(t *testing.T) {
	deps := func() *Dependencies { return &Dependencies{MetricsRegistry: metric.NewMetricsRegistry()} }

	t.Run("metrics payload through NewMetrics", func(t *testing.T) {
		svc, err := NewMetrics(json.RawMessage(`{"Port":9091}`), deps())
		require.ErrorContains(t, err, "parse metrics config: config key Port must be written port")
		require.Nil(t, svc)

		svc, err = NewMetrics(json.RawMessage(`{"port":9091}`), deps())
		require.NoError(t, err)
		require.NotNil(t, svc)
	})

	for _, tt := range []struct {
		name, variant, exact, want string
	}{
		{
			name:    "manager payload, top-level key",
			variant: `{"HTTP_PORT":8080}`,
			exact:   `{"http_port":8080}`,
			want:    "config key HTTP_PORT must be written http_port",
		},
		{
			name:    "manager payload, nested server_info key",
			variant: `{"server_info":{"Title":"t"}}`,
			exact:   `{"server_info":{"title":"t"}}`,
			want:    "config key server_info.Title must be written server_info.title",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var refused ManagerConfig
			require.ErrorContains(t, decodeStrictServiceJSON(json.RawMessage(tt.variant), &refused), tt.want)

			var accepted ManagerConfig
			require.NoError(t, decodeStrictServiceJSON(json.RawMessage(tt.exact), &accepted))
		})
	}
}
