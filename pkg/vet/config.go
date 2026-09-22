package vet

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// GatesConfig defines declarative parameters for verification checks.
type GatesConfig struct {
	Version    string           `yaml:"version"`
	Hygiene    HygieneConfig    `yaml:"hygiene"`
	TreePolice TreePoliceConfig `yaml:"tree_police"`
	Payload    PayloadConfig    `yaml:"payload"`
}

type HygieneConfig struct {
	CheckPaths    bool     `yaml:"check_paths"`
	CheckPerms    bool     `yaml:"check_perms"`
	CheckDups     bool     `yaml:"check_dups"`
	CheckCLINames bool     `yaml:"check_cli_names"`
	Exemptions    []string `yaml:"exemptions"`
}

type TreePoliceConfig struct {
	ForbiddenPaths             []string `yaml:"forbidden_paths"`
	ForbiddenFiles             []string `yaml:"forbidden_files"`
	ForbiddenPatterns          []string `yaml:"forbidden_patterns"`
	ForbiddenScriptReferences []string `yaml:"forbidden_script_references"`
}

type TokenRule struct {
	ID         string   `yaml:"id"`
	Pattern    string   `yaml:"pattern"`
	Message    string   `yaml:"message"`
	Scope      []string `yaml:"scope"`
	Exemptions []string `yaml:"exemptions"`
}

type PayloadConfig struct {
	RequiredArtifacts  []string    `yaml:"required_artifacts"`
	ForbiddenArtifacts []string    `yaml:"forbidden_artifacts"`
	ProhibitedPatterns []TokenRule `yaml:"prohibited_patterns"`
}

// LoadConfig loads GatesConfig from a yaml file. If path is empty, looks for config/gates.yaml in projectRoot.
func LoadConfig(projectRoot, configPath string) (*GatesConfig, error) {
	if configPath == "" {
		configPath = filepath.Join(projectRoot, "config", "gates.yaml")
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, err
	}
	var cfg GatesConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// DefaultConfig provides baseline fail-closed configuration if config/gates.yaml is missing.
func DefaultConfig() *GatesConfig {
	return &GatesConfig{
		Version: "1.0.0",
		Hygiene: HygieneConfig{
			CheckPaths:    true,
			CheckPerms:    true,
			CheckCLINames: true,
			Exemptions:    []string{"*_test.go"},
		},
		TreePolice: TreePoliceConfig{
			ForbiddenPaths: []string{
				"docs/commercial", "docs/marketing", "docs/launch",
				"marketing-site", "pkg/billing", "cmd/codegen_runner",
			},
			ForbiddenFiles: []string{
				"ack.txt", "remaining.txt", "issues.json", "scripts/open-core/community-bounded-includes.txt",
			},
		},
		Payload: PayloadConfig{
			RequiredArtifacts: []string{
				"README.md", "LICENSE", "NOTICE", "SECURITY.md", "go.mod", "config/zqk.yaml",
			},
		},
	}
}
