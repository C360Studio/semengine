package component

// SimpleMockComponent is a Discoverable with no ports, for tests that need a
// factory to return some component.
type SimpleMockComponent struct {
	name string
}

// InputPorts returns no ports.
func (m *SimpleMockComponent) InputPorts() []Port { return nil }

// OutputPorts returns no ports.
func (m *SimpleMockComponent) OutputPorts() []Port { return nil }

// Health reports healthy.
func (m *SimpleMockComponent) Health() HealthStatus { return HealthStatus{Healthy: true} }

// DataFlow reports no traffic.
func (m *SimpleMockComponent) DataFlow() FlowMetrics { return FlowMetrics{} }

// intPtr returns a pointer to i, for optional schema fields such as Minimum
// and Maximum.
func intPtr(i int) *int { return &i }
