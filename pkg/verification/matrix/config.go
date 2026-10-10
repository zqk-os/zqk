package matrix

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"gopkg.in/yaml.v3"
)

// MatrixConfig defines the declarative specification for a verification matrix.
type MatrixConfig struct {
	Name        string                    `json:"name" yaml:"name"`
	Description string                    `json:"description" yaml:"description"`
	StorageDir  string                    `json:"storage_dir,omitempty" yaml:"storage_dir,omitempty"`
	Profile     *Profile                  `json:"profile,omitempty" yaml:"profile,omitempty"`
	Classes     map[FileClass]ClassConfig `json:"classes,omitempty" yaml:"classes,omitempty"`
	Dimensions  []DimensionSpec           `json:"dimensions,omitempty" yaml:"dimensions,omitempty"`
	Patterns    []PatternCheckConfig      `json:"patterns,omitempty" yaml:"patterns,omitempty"`
	Commands    []CommandCheckConfig      `json:"commands,omitempty" yaml:"commands,omitempty"`
	Agents      []AgentCheckConfig        `json:"agents,omitempty" yaml:"agents,omitempty"`
}

// PatternRuleConfig defines declarative regex rule properties within a pattern check config.
type PatternRuleConfig struct {
	ID               string        `json:"id" yaml:"id"`
	Message          string        `json:"message" yaml:"message"`
	Severity         string        `json:"severity" yaml:"severity"`
	Pattern          string        `json:"pattern" yaml:"pattern"`
	TargetClasses    []FileClass   `json:"target_classes,omitempty" yaml:"target_classes,omitempty"`
	ExcludeClasses   []FileClass   `json:"exclude_classes,omitempty" yaml:"exclude_classes,omitempty"`
	IgnoreIfContains []string      `json:"ignore_if_contains,omitempty" yaml:"ignore_if_contains,omitempty"`
	Dimension        DimensionCode `json:"dimension,omitempty" yaml:"dimension,omitempty"`
}

// PatternCheckConfig defines a pattern-based check primitive.
type PatternCheckConfig struct {
	ID            string              `json:"id" yaml:"id"`
	Name          string              `json:"name" yaml:"name"`
	Description   string              `json:"description" yaml:"description"`
	TargetClasses []FileClass         `json:"target_classes,omitempty" yaml:"target_classes,omitempty"`
	Rules         []PatternRuleConfig `json:"rules,omitempty" yaml:"rules,omitempty"`
}

// CommandCheckConfig defines an external command or linter check to execute.
type CommandCheckConfig struct {
	ID            string      `json:"id" yaml:"id"`
	Name          string      `json:"name" yaml:"name"`
	Description   string      `json:"description" yaml:"description"`
	Command       string      `json:"command" yaml:"command"`
	TargetClasses []FileClass `json:"target_classes" yaml:"target_classes"`
	TimeoutSec    int         `json:"timeout_sec,omitempty" yaml:"timeout_sec,omitempty"`
}

// AgentCheckConfig defines a blind rubric-based agent check primitive.
type AgentCheckConfig struct {
	ID            string      `json:"id" yaml:"id"`
	Name          string      `json:"name" yaml:"name"`
	Description   string      `json:"description" yaml:"description"`
	Rubric        string      `json:"rubric" yaml:"rubric"`
	TargetClasses []FileClass `json:"target_classes,omitempty" yaml:"target_classes,omitempty"`
}

// DefaultConfig returns a sensible, language-agnostic default matrix configuration.
func DefaultConfig() *MatrixConfig {
	profile := DefaultProfile()
	return &MatrixConfig{
		Name:        "default-verification-matrix",
		Description: "Universal content-addressed file verification matrix",
		StorageDir:  ".matrix",
		Profile:     profile,
		Classes:     profile.Classes,
		Dimensions:  DefaultDimensions(),
	}
}

// LoadConfig loads a matrix configuration from a YAML or JSON file.
func LoadConfig(path string) (*MatrixConfig, error) {
	data, err := fileutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := DefaultConfig()
	ext := filepath.Ext(path)
	if ext == ".yaml" || ext == ".yml" {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse yaml config %s: %w", path, err)
		}
	} else {
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse json config %s: %w", path, err)
		}
	}

	// Reconcile Profile and Classes
	if cfg.Profile != nil && len(cfg.Profile.Classes) > 0 {
		if cfg.Classes == nil {
			cfg.Classes = cfg.Profile.Classes
		}
	} else if cfg.Classes != nil && len(cfg.Classes) > 0 {
		cfg.Profile = &Profile{
			Name:    cfg.Name,
			Classes: cfg.Classes,
		}
	}

	return cfg, nil
}
