// Package cleanup provides config-driven cleanup: one YAML config (steps) as the single
// source of input for delete, truncate, archive, move, and build-tool/CLI steps.
// See docs/architecture/CLEANUP_MAINTENANCE_DESIGN.md.
package cleanup

import (
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/errfmt"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// DefaultConfigPath is the path relative to project root when no job override is set.
const DefaultConfigPath = ".zqk/cleanup/config.yaml"

const (
	cleanupStepFieldType    = "type"
	emptyValue              = ""
	errConfigNotFoundFmt    = "cleanup config not found: %s"
	errReadCleanupConfigFmt = "read cleanup config: %w"
	errParseCleanupCfgFmt   = "parse cleanup config: %w"
)

// Config is the root cleanup config: working directory and ordered steps.
type Config struct {
	WorkingDirectory string `yaml:"working_directory,omitempty"`
	Steps            []Step `yaml:"steps"`
}

// Step is one cleanup step: type (delete_files, truncate_files, cli, build_target, etc.) and params.
// Params are type-specific; see CLEANUP_MAINTENANCE_DESIGN.md for each step type's keys.
type Step struct {
	Type   string
	Params map[string]any
}

// UnmarshalYAML implements custom unmarshaling so type and all other keys are captured.
func (s *Step) UnmarshalYAML(n *yaml.Node) error {
	var raw map[string]any
	if err := n.Decode(&raw); err != nil {
		return err
	}
	if t, ok := raw[cleanupStepFieldType].(string); ok {
		s.Type = t
		delete(raw, cleanupStepFieldType)
	}
	s.Params = raw
	return nil
}

// LoadFromPath loads cleanup config from path. Path is relative to projectRoot unless absolute.
// Missing file or empty path returns an error.
func LoadFromPath(projectRoot, path string) (*Config, error) {
	if path == emptyValue {
		path = DefaultConfigPath
	}
	full := path
	if !filepath.IsAbs(path) && projectRoot != emptyValue {
		full = filepath.Join(projectRoot, path)
	}
	data, err := fileutil.ReadFile(full)
	if err != nil {
		if fileutil.IsNotExist(err) {
			// Use logical path (path) in error so persisted issues stay portable (no absolute path).
			return nil, errfmt.Errorf(errConfigNotFoundFmt, path)
		}
		return nil, errfmt.Errorf(errReadCleanupConfigFmt, err)
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, errfmt.Errorf(errParseCleanupCfgFmt, err)
	}
	return &c, nil
}
