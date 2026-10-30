package query

import (
	"encoding/json"
	"fmt"
	"os"
)

// Example represents a single domain query example for intent classification.
type Example struct {
	Query   string         `json:"query"`   // Natural language query
	Intent  string         `json:"intent"`  // Intent category
	Options map[string]any `json:"options"` // SearchOptions hints (optional)
	Vector  []float32      `json:"-"`       // Runtime field - embedding vector (not serialized)
}

// DomainExamples represents a collection of examples for a domain.
type DomainExamples struct {
	Domain   string    `json:"domain"`
	Version  string    `json:"version"`
	Examples []Example `json:"examples"`
}

// LoadDomainExamples loads query examples from a JSON file.
func LoadDomainExamples(filePath string) (*DomainExamples, error) {
	if filePath == "" {
		return nil, fmt.Errorf("file path cannot be empty")
	}

	// Read file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	// Empty file check
	if len(data) == 0 {
		return nil, fmt.Errorf("file is empty")
	}

	// Unmarshal JSON
	var examples DomainExamples
	if err := json.Unmarshal(data, &examples); err != nil {
		return nil, err
	}

	return &examples, nil
}
