package config_builders

import (
	"gopkg.in/yaml.v3"
)

// BaseConfigBuilder provides common functionality for config builders
// Note: Configs store raw YAML structure due to diverse config types
type BaseConfigBuilder struct {
	fileName string
	version  string
	config   map[string]any // Raw YAML structure
}

// NewBaseConfigBuilder creates a new base config builder
func NewBaseConfigBuilder(fileName, version string) *BaseConfigBuilder {
	return &BaseConfigBuilder{
		fileName: fileName,
		version:  version,
		config:   make(map[string]any),
	}
}

// SetConfig sets the config map (raw YAML structure)
func (b *BaseConfigBuilder) SetConfig(config map[string]any) *BaseConfigBuilder {
	b.config = config
	return b
}

// Build builds the config as YAML bytes
func (b *BaseConfigBuilder) Build() ([]byte, error) {
	// Marshal config map to YAML
	return yaml.Marshal(b.config)
}

// GetVersion returns the version
func (b *BaseConfigBuilder) GetVersion() string {
	return b.version
}

// GetFileName returns the config file name
func (b *BaseConfigBuilder) GetFileName() string {
	return b.fileName
}
