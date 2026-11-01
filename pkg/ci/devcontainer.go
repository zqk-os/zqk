package ci

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	// DefaultDevcontainerRelPath is the canonical relative path to devcontainer.json.
	DefaultDevcontainerRelPath = ".devcontainer/devcontainer.json"

	// ExpectedGoImage is the pinned official Go image.
	ExpectedGoImage = "mcr.microsoft.com/devcontainers/go:1.26"

	// ExpectedRemoteUser is the required non-root user.
	ExpectedRemoteUser = "vscode"
)

// DevcontainerConfig models the subset of devcontainer.json fields relevant to zero-touch onboarding.
type DevcontainerConfig struct {
	Schema            string            `json:"$schema"`
	Name              string            `json:"name"`
	Image             string            `json:"image"`
	RemoteUser        string            `json:"remoteUser"`
	Features          map[string]any    `json:"features"`
	Customizations    map[string]any    `json:"customizations"`
	ContainerEnv      map[string]string `json:"containerEnv"`
	PostCreateCommand string            `json:"postCreateCommand"`
	RunArgs           []string          `json:"runArgs"`
	ForwardPorts      []int             `json:"forwardPorts,omitempty"`
}

// DevcontainerValidationResult records verification findings.
type DevcontainerValidationResult struct {
	Valid          bool     `json:"valid"`
	DevcontainerPath string `json:"devcontainer_path"`
	ChecksPassed   []string `json:"checks_passed"`
}

// ValidateRepoDevcontainer locates and validates .devcontainer/devcontainer.json in repoRoot.
func ValidateRepoDevcontainer(repoRoot string) (*DevcontainerValidationResult, error) {
	if strings.TrimSpace(repoRoot) == "" {
		repoRoot = "."
	}
	cfgPath := filepath.Join(repoRoot, DefaultDevcontainerRelPath)
	return ValidateDevcontainerFile(cfgPath)
}

// ValidateDevcontainerFile parses and verifies the devcontainer configuration file.
func ValidateDevcontainerFile(path string) (*DevcontainerValidationResult, error) {
	if !fileutil.Exists(path) {
		return nil, errfmt.Errorf("devcontainer configuration not found at: %s", path)
	}

	raw, err := fileutil.ReadFile(path)
	if err != nil {
		return nil, errfmt.Errorf("failed to read devcontainer file: %w", err)
	}

	var cfg DevcontainerConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, errfmt.Errorf("invalid json in devcontainer file: %w", err)
	}

	var passed []string
	if err := validateStaticInvariants(&cfg, &passed); err != nil {
		return nil, err
	}
	if err := validateOperationalCommands(&cfg, &passed); err != nil {
		return nil, err
	}
	if err := validateSecurityBoundaries(string(raw), &cfg, &passed); err != nil {
		return nil, err
	}

	return &DevcontainerValidationResult{
		Valid:            true,
		DevcontainerPath: path,
		ChecksPassed:     passed,
	}, nil
}

func validateStaticInvariants(cfg *DevcontainerConfig, passed *[]string) error {
	if !strings.HasPrefix(cfg.Image, "mcr.microsoft.com/devcontainers/go:1.26") {
		return fmt.Errorf("devcontainer image must pin Go 1.26+, found: %q", cfg.Image)
	}
	*passed = append(*passed, "image_go_toolchain_pinned")

	if cfg.RemoteUser != ExpectedRemoteUser {
		return fmt.Errorf("devcontainer remoteUser must be %q to prevent root execution, found: %q", ExpectedRemoteUser, cfg.RemoteUser)
	}
	*passed = append(*passed, "non_root_remote_user")

	if cfg.Features == nil || cfg.Features["ghcr.io/devcontainers/features/github-cli:1"] == nil {
		return fmt.Errorf("devcontainer must include github-cli feature for paired collaboration")
	}
	*passed = append(*passed, "features_github_cli_enabled")

	return nil
}

func validateOperationalCommands(cfg *DevcontainerConfig, passed *[]string) error {
	postCmd := cfg.PostCreateCommand
	if !strings.Contains(postCmd, "go build") || !strings.Contains(postCmd, "./cmd/") {
		return fmt.Errorf("postCreateCommand must compile the CLI binary")
	}
	if !strings.Contains(postCmd, "mcp ensure") {
		return fmt.Errorf("postCreateCommand must execute mcp ensure")
	}
	if !strings.Contains(postCmd, "agent-onboard") {
		return fmt.Errorf("postCreateCommand must run agent-onboard")
	}
	*passed = append(*passed, "post_create_command_verified")

	if cfg.ContainerEnv == nil || cfg.ContainerEnv["ZQK_CONTEXT"] != "ai-agent" {
		return fmt.Errorf("containerEnv must set ZQK_CONTEXT=ai-agent")
	}
	*passed = append(*passed, "container_env_preconditioned")

	return nil
}

func validateSecurityBoundaries(rawContent string, cfg *DevcontainerConfig, passed *[]string) error {
	// Rejection of privileged mode and root capabilities
	hasCapDrop := false
	for _, arg := range cfg.RunArgs {
		if strings.Contains(arg, "--privileged") {
			return fmt.Errorf("security invariant violated: --privileged mode is prohibited")
		}
		if arg == "--cap-drop=ALL" {
			hasCapDrop = true
		}
	}
	if !hasCapDrop {
		return fmt.Errorf("security invariant violated: runArgs must include --cap-drop=ALL")
	}
	*passed = append(*passed, "cap_drop_isolation_enforced")

	// Secrets leakage check
	secretSignatures := []string{"ghp_", "gho_", "glpat-", "AKIA", "PRIVATE KEY"}
	for _, sig := range secretSignatures {
		if strings.Contains(rawContent, sig) {
			return fmt.Errorf("security invariant violated: detected possible hardcoded credential pattern %q", sig)
		}
	}
	*passed = append(*passed, "zero_hardcoded_secrets")

	return nil
}
