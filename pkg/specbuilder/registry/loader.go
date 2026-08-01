package registry

import (
	"encoding/json"
	"os"
)

// LoadRegistry reads the registry from a file and returns a FieldRegistry.
func LoadRegistry(path string) (*FieldRegistry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var registry FieldRegistry
	if err := json.Unmarshal(data, &registry); err != nil {
		return nil, err
	}
	return &registry, nil
}
