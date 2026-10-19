package health

import (
	"testing"
	"time"

	"github.com/c360studio/semengine/component"
)

func TestStatus_IsDegraded(t *testing.T) {
	tests := []struct {
		name   string
		status Status
		want   bool
	}{
		{
			name:   "degraded status returns true",
			status: Status{Status: "degraded"},
			want:   true,
		},
		{
			name:   "healthy status returns false",
			status: Status{Status: "healthy"},
			want:   false,
		},
		{
			name:   "unhealthy status returns false",
			status: Status{Status: "unhealthy"},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.IsDegraded(); got != tt.want {
				t.Errorf("Status.IsDegraded() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStatus_IsUnhealthy(t *testing.T) {
	tests := []struct {
		name   string
		status Status
		want   bool
	}{
		{
			name:   "unhealthy status returns true",
			status: Status{Status: "unhealthy"},
			want:   true,
		},
		{
			name:   "healthy status returns false",
			status: Status{Status: "healthy"},
			want:   false,
		},
		{
			name:   "degraded status returns false",
			status: Status{Status: "degraded"},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.IsUnhealthy(); got != tt.want {
				t.Errorf("Status.IsUnhealthy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFromComponentHealth(t *testing.T) {
	tests := []struct {
		name            string
		componentName   string
		componentHealth component.HealthStatus
		wantStatus      string
		wantMessage     string
	}{
		{
			name:          "healthy component",
			componentName: "test-component",
			componentHealth: component.HealthStatus{
				Healthy:    true,
				LastCheck:  time.Now(),
				ErrorCount: 0,
				Uptime:     time.Hour,
			},
			wantStatus:  "healthy",
			wantMessage: "Component healthy",
		},
		{
			name:          "unhealthy component with error",
			componentName: "failing-component",
			componentHealth: component.HealthStatus{
				Healthy:    false,
				LastCheck:  time.Now(),
				ErrorCount: 3,
				LastError:  "connection failed",
				Uptime:     time.Minute,
			},
			wantStatus:  "unhealthy",
			wantMessage: "connection failed",
		},
		{
			name:          "unhealthy component without error message",
			componentName: "broken-component",
			componentHealth: component.HealthStatus{
				Healthy:    false,
				LastCheck:  time.Now(),
				ErrorCount: 1,
				Uptime:     time.Second,
			},
			wantStatus:  "unhealthy",
			wantMessage: "Component unhealthy", // fallback message (Q3, #318 comment 6271710866)
		},
		{
			// Healthy=false is unhealthy whatever the component's own word says
			// (owner ruling, #318 comment 6271450376).
			name:          "component reporting itself degraded",
			componentName: "poisoned-component",
			componentHealth: component.HealthStatus{
				Healthy:    false,
				Status:     "degraded",
				LastCheck:  time.Now(),
				ErrorCount: 2,
				Uptime:     time.Minute,
			},
			wantStatus:  "unhealthy",
			wantMessage: "Component unhealthy",
		},
		{
			name:          "secrets in the error text",
			componentName: "leaky-component",
			componentHealth: component.HealthStatus{
				Healthy:   false,
				LastCheck: time.Now(),
				LastError: "failed to connect to https://api.example.com/v1 with password=secret123",
			},
			wantStatus:  "unhealthy",
			wantMessage: "failed to connect to [URL] with [REDACTED]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FromComponentHealth(tt.componentName, tt.componentHealth)

			if result.Component != tt.componentName {
				t.Errorf("Expected component name %s, got %s", tt.componentName, result.Component)
			}

			if result.Status != tt.wantStatus {
				t.Errorf("Expected status %s, got %s", tt.wantStatus, result.Status)
			}

			if result.Message != tt.wantMessage {
				t.Errorf("Expected message %s, got %s", tt.wantMessage, result.Message)
			}

			if result.Metrics == nil {
				t.Error("Expected metrics to be set")
			} else {
				if result.Metrics.Uptime != tt.componentHealth.Uptime {
					t.Errorf("Expected uptime %v, got %v", tt.componentHealth.Uptime, result.Metrics.Uptime)
				}

				if result.Metrics.ErrorCount != tt.componentHealth.ErrorCount {
					t.Errorf("Expected error count %d, got %d", tt.componentHealth.ErrorCount, result.Metrics.ErrorCount)
				}

				if !result.Metrics.LastActivity.Equal(tt.componentHealth.LastCheck) {
					t.Errorf("Expected last activity %v, got %v", tt.componentHealth.LastCheck, result.Metrics.LastActivity)
				}
			}

			if result.Timestamp.IsZero() {
				t.Error("Expected timestamp to be set")
			}
		})
	}
}
