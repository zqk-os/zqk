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
	CheckPaths             bool     `yaml:"check_paths"`
	CheckPerms             bool     `yaml:"check_perms"`
	CheckDups              bool     `yaml:"check_dups"`
	DupMinStatements       int      `yaml:"dup_min_statements"`
	DupMinLines            int      `yaml:"dup_min_lines"`
	DupExemptions          []string `yaml:"dup_exemptions"`
	CheckCLINames          bool     `yaml:"check_cli_names"`
	CheckSubprocessHygiene bool     `yaml:"check_subprocess_hygiene"`
	CheckCommandSpecs      bool     `yaml:"check_command_specs"`
	CheckCLIBuilders       bool     `yaml:"check_cli_builders"`
	CheckRawGoroutines     bool     `yaml:"check_raw_goroutines"`
	GoroutineExemptions    []string `yaml:"goroutine_exemptions"`
	ForbiddenPathLiterals []string `yaml:"forbidden_path_literals"`
	GoScanDirs            []string `yaml:"go_scan_dirs"`
	Exemptions            []string `yaml:"exemptions"`
	CLIExemptFunctions    []string `yaml:"cli_exempt_functions"`
	CLIExemptFiles        []string `yaml:"cli_exempt_files"`
}

type TreePoliceConfig struct {
	ForbiddenPaths             []string `yaml:"forbidden_paths"`
	ForbiddenFiles             []string `yaml:"forbidden_files"`
	ArchivedDocDirs            []string `yaml:"archived_doc_dirs"`
	ArchivedDocNames           []string `yaml:"archived_doc_names"`
	AllowedScripts             []string `yaml:"allowed_scripts"`
	AllowedScriptPrefixes      []string `yaml:"allowed_script_prefixes"`
	ForbiddenPatterns          []string `yaml:"forbidden_patterns"`
	ForbiddenScriptReferences []string `yaml:"forbidden_script_references"`
	GrepExemptions             []string `yaml:"grep_exemptions"`
}

type TokenRule struct {
	ID         string   `yaml:"id"`
	Pattern    string   `yaml:"pattern"`
	Message    string   `yaml:"message"`
	Scope      []string `yaml:"scope"`
	Exemptions []string `yaml:"exemptions"`
}

type PayloadConfig struct {
	RequiredModulePath   string      `yaml:"required_module_path"`
	RequiredArtifacts    []string    `yaml:"required_artifacts"`
	ForbiddenArtifacts   []string    `yaml:"forbidden_artifacts"`
	SensitivePatterns    []string    `yaml:"sensitive_patterns"`
	GlobalGrepExemptions []string    `yaml:"global_grep_exemptions"`
	ProhibitedPatterns   []TokenRule `yaml:"prohibited_patterns"`
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
			CheckPaths:             true,
			CheckPerms:             true,
			CheckDups:              true,
			DupMinStatements:       5,
			DupMinLines:            8,
			DupExemptions:          []string{"*_test.go", "vendor/*", "*/testdata/*", "*/mock/*"},
			CheckCLINames:          true,
			CheckSubprocessHygiene: true,
			CheckCommandSpecs:      true,
			CheckCLIBuilders:       true,
			CheckRawGoroutines:     true,
			GoroutineExemptions:    []string{"*_test.go", "vendor/*", "pkg/goroutinelabels/*", "*/testdata/*"},
			ForbiddenPathLiterals: []string{".zqk", ".zqk/"},
			GoScanDirs:            []string{"pkg/", "cmd/", "internal/", "scripts/", "packs/", "ext/"},
			Exemptions:            []string{"*_test.go", "vendor/*", ".git/*", "pkg/paths/*", "pkg/brand/*", "pkg/vet/*"},
			CLIExemptFunctions:    []string{"CLIUsage", "CLIInvocation", "RewriteCanonicalCLIInvocations"},
			CLIExemptFiles:        []string{"*_test.go", "pkg/paths/cli_command_name.go", "pkg/vet/*"},
		},
		TreePolice: TreePoliceConfig{
			ForbiddenPaths: []string{
				"docs/commercial", "docs/marketing", "docs/launch",
				"marketing-site", "pkg/billing", "cmd/codegen_runner",
			},
			ForbiddenFiles: []string{
				"ack.txt", "remaining.txt", "issues.json", "scripts/open-core/community-bounded-includes.txt",
			},
			ArchivedDocDirs:       []string{"docs"},
			ArchivedDocNames:      []string{"archive", "_archive"},
			AllowedScripts:        []string{"package-community.sh", "install.sh", "generate-openvex.sh"},
			AllowedScriptPrefixes: []string{"scripts/open-core/"},
			GrepExemptions:        []string{"scripts/open-core/police-community-tree.sh", "pkg/vet/*", "config/gates.yaml"},
		},
		Payload: PayloadConfig{
			RequiredModulePath: "github.com/zqk-os/zqk",
			RequiredArtifacts: []string{
				"README.md", "LICENSE", "NOTICE", "SECURITY.md", "go.mod", "config/zqk.yaml",
			},
			SensitivePatterns: []string{
				".zqk/process/**", "docs/process/**",
				".zqk/keystore/**", ".zqk/state/**", "config/zqk-local.yaml",
				"*.pem", "*.key", "*.p12", "*.pfx",
			},
			GlobalGrepExemptions: []string{"pkg/vet/*", "config/gates.yaml"},
		},
	}
}
