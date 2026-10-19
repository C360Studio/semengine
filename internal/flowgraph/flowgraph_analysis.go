package flowgraph

import "github.com/c360studio/semengine/component"

// FlowAnalysisResult contains the results of connectivity analysis
type FlowAnalysisResult struct {
	ConnectedComponents [][]string
	DisconnectedNodes   []DisconnectedNode
	OrphanedPorts       []OrphanedPort
}

// DisconnectedNode represents a component with no connections
type DisconnectedNode struct {
	ComponentName string
	Issue         string
	Suggestions   []string
}

// OrphanedPort represents a port with no connections
type OrphanedPort struct {
	ComponentName string
	PortName      string
	Direction     component.Direction
	ConnectionID  string
	Pattern       component.InteractionPattern
	Issue         string
	Required      bool
}

// StreamRequirementWarning represents a mismatch between JetStream subscriber and NATS publisher
type StreamRequirementWarning struct {
	SubscriberComp string
	SubscriberPort string
	Subjects       []string
	PublisherComps []string
}
