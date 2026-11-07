package bundle

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"gopkg.in/yaml.v3"
)

// LanguageBundleManifest defines the declarative specification for a language pack or verification bundle.
type LanguageBundleManifest struct {
	SchemaVersion   string          `json:"schema_version" yaml:"schema_version"`
	Name            string          `json:"name" yaml:"name"`
	Language        string          `json:"language" yaml:"language"`
	Version         string          `json:"version" yaml:"version"`
	Description     string          `json:"description" yaml:"description"`
	Standards       []StandardSpec  `json:"standards,omitempty" yaml:"standards,omitempty"`
	ASTPolicies     []ASTPolicySpec `json:"ast_policies,omitempty" yaml:"ast_policies,omitempty"`
	Gates           []GateSpec      `json:"gates,omitempty" yaml:"gates,omitempty"`
	AgentDirectives []DirectiveSpec `json:"agent_directives,omitempty" yaml:"agent_directives,omitempty"`
	Dependencies    []string        `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`
}

type StandardSpec struct {
	ID          string `json:"id" yaml:"id"`
	Title       string `json:"title" yaml:"title"`
	Description string `json:"description" yaml:"description"`
	Severity    string `json:"severity" yaml:"severity"`
	RuleURL     string `json:"rule_url,omitempty" yaml:"rule_url,omitempty"`
}

type ASTPolicySpec struct {
	ID           string   `json:"id" yaml:"id"`
	RuleType     string   `json:"rule_type" yaml:"rule_type"`
	TargetKind   string   `json:"target_kind" yaml:"target_kind"`
	Pattern      string   `json:"pattern,omitempty" yaml:"pattern,omitempty"`
	Forbidden    []string `json:"forbidden,omitempty" yaml:"forbidden,omitempty"`
	MandatoryDoc bool     `json:"mandatory_doc,omitempty" yaml:"mandatory_doc,omitempty"`
}

type GateSpec struct {
	Name        string   `json:"name" yaml:"name"`
	Command     string   `json:"command" yaml:"command"`
	Suite       string   `json:"suite,omitempty" yaml:"suite,omitempty"`
	EnforceCI   bool     `json:"enforce_ci" yaml:"enforce_ci"`
	Environment []string `json:"environment,omitempty" yaml:"environment,omitempty"`
}

type DirectiveSpec struct {
	TargetRole string `json:"target_role" yaml:"target_role"`
	Directive  string `json:"directive" yaml:"directive"`
	Path       string `json:"path,omitempty" yaml:"path,omitempty"`
}

// Validate ensures all required static floor invariants are satisfied.
func (m *LanguageBundleManifest) Validate() error {
	if strings.TrimSpace(m.SchemaVersion) == "" {
		return errors.New("manifest schema_version is required")
	}
	if strings.TrimSpace(m.Name) == "" {
		return errors.New("manifest name is required")
	}
	if strings.TrimSpace(m.Language) == "" {
		return errors.New("manifest language is required")
	}
	if strings.TrimSpace(m.Version) == "" {
		return errors.New("manifest version is required")
	}

	// Negative adversarial security boundaries: name sanitization and traversal prevention
	if strings.Contains(m.Name, "..") || strings.Contains(m.Name, "/") || strings.Contains(m.Name, "\\") {
		return fmt.Errorf("invalid bundle name %q: directory traversal disallowed", m.Name)
	}

	for _, dep := range m.Dependencies {
		if strings.TrimSpace(dep) == "" {
			return errors.New("manifest contains empty dependency")
		}
		if dep == m.Name {
			return fmt.Errorf("circular self-dependency detected: %q depends on itself", m.Name)
		}
	}

	for i, directive := range m.AgentDirectives {
		if directive.Path != "" {
			cleanPath := filepath.Clean(directive.Path)
			if strings.HasPrefix(cleanPath, "..") || filepath.IsAbs(cleanPath) {
				return fmt.Errorf("agent directive %d: path traversal outside workspace disallowed (%q)", i, directive.Path)
			}
		}
	}

	return nil
}

// ParseManifestBytes parses manifest YAML or JSON data.
func ParseManifestBytes(data []byte) (*LanguageBundleManifest, error) {
	if len(data) == 0 {
		return nil, errors.New("bundle manifest data is empty")
	}

	var m LanguageBundleManifest
	// Try JSON first
	if err := json.Unmarshal(data, &m); err == nil && m.Name != "" {
		if err := m.Validate(); err != nil {
			return nil, err
		}
		return &m, nil
	}

	// Fallback to YAML
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("failed to parse manifest as YAML or JSON: %w", err)
	}

	if err := m.Validate(); err != nil {
		return nil, err
	}

	return &m, nil
}

// LoadManifestFile loads a bundle manifest from a file.
func LoadManifestFile(filePath string) (*LanguageBundleManifest, error) {
	data, err := fileutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read bundle manifest file: %w", err)
	}
	return ParseManifestBytes(data)
}
