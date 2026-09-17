package api_builders

import (
	"gopkg.in/yaml.v3"
)

// BaseAPISpecGenBuilder provides common functionality for api builders
// Note: APIs store raw YAML structure due to diverse api types
type BaseAPISpecGenBuilder struct {
	fileName string
	version  string
	api      map[string]any // Raw YAML structure
}

// NewBaseAPISpecGenBuilder creates a new base api builder
func NewBaseAPISpecGenBuilder(fileName, version string) *BaseAPISpecGenBuilder {
	return &BaseAPISpecGenBuilder{
		fileName: fileName,
		version:  version,
		api:      make(map[string]any),
	}
}

// SetAPI sets the api map (raw YAML structure)
func (b *BaseAPISpecGenBuilder) SetAPI(api map[string]any) *BaseAPISpecGenBuilder {
	b.api = api
	return b
}

// Build builds the api as YAML bytes
func (b *BaseAPISpecGenBuilder) Build() ([]byte, error) {
	// Marshal api map to YAML
	return yaml.Marshal(b.api)
}

// GetVersion returns the version
func (b *BaseAPISpecGenBuilder) GetVersion() string {
	return b.version
}

// GetFileName returns the api file name
func (b *BaseAPISpecGenBuilder) GetFileName() string {
	return b.fileName
}
