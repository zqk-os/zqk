package mcp

import (
	"path/filepath"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// registerOnboardingPromptsProgrammatically registers prompts using the default programmatic definitions
// This is the fallback when no spec files or storage objects are found
func registerOnboardingPromptsProgrammatically(server *Server) {
	spec := ConvertPromptsToSpec()
	generator := NewMCPSpecGenerator(server)
	for _, promptSpec := range spec.Prompts {
		_ = generator.RegisterPrompt(promptSpec) //nolint:errcheck // Best effort fallback
	}
}

// RegisterOnboardingPrompts registers all onboarding and help prompts
// Uses spec-driven builder pattern: attempts to load from storage (mcp_spec objects) first,
// then tries spec files, falls back to programmatic registration
// Never panics - always falls back gracefully to programmatic registration
func RegisterOnboardingPrompts(server *Server) {
	// Add panic recovery to ensure we always fall back gracefully
	defer func() {
		if r := recover(); r != nil {
			// Log panic but don't crash - fall back to programmatic registration
			logger := logging.GetLoggerFromProfile(DefaultLoggingProfile)
			logging.Fluent(logger).Error("Panic during onboarding prompts registration, using programmatic fallback",
				errfmt.Errorf("panic: %v", r)).
				EmitComponent("mcp_server").
				String("operation", "RegisterOnboardingPrompts").
				Log()
			// Fall back to programmatic registration
			registerOnboardingPromptsProgrammatically(server)
		}
	}()

	// Try kernel storage first. Persisted-but-invalid configuration is terminal;
	// only missing or unavailable storage may use bootstrap fallback.
	selection, err := selectStoredMCPSpec(pkgctx.NewSystemContext(), server, "onboarding_prompts")
	if err != nil {
		return
	}
	if selection.Spec != nil {
		generator := NewMCPSpecGenerator(server)
		for _, promptSpec := range selection.Spec.Prompts {
			if err := generator.RegisterPrompt(promptSpec); err != nil {
				logMCPSpecRuntimeError(server, "onboarding_prompts", err)
				return
			}
		}
		return
	}

	// Try to load from spec file (externalized configuration)
	// Look for spec in common locations, using project root if available
	projectRoot := server.GetProjectRoot()
	specPaths := []string{
		filepath.Join(projectRoot, paths.ProjectDataDir, paths.MCPDir, "specs", "onboarding_prompts.yaml"),
		filepath.Join(projectRoot, paths.ProjectDataDir, "mcp_specs", "onboarding_prompts.yaml"),
		filepath.Join(datacell.CellCASPrimaryDir(projectRoot, "mcp_specs"), "onboarding_prompts.yaml"),
		filepath.Join(projectRoot, "mcp_specs/onboarding_prompts.yaml"),
		".zqk/mcp/specs/onboarding_prompts.yaml",
		".zqk/mcp_specs/onboarding_prompts.yaml",
		filepath.Join(datacell.ProcessPrimaryDir("."), "mcp_specs", "onboarding_prompts.yaml"),
		"mcp_specs/onboarding_prompts.yaml",
	}

	var specLoaded bool
	for _, specPath := range specPaths {
		if _, err := fileutil.Stat(specPath); err == nil {
			// Spec file exists, try to load it
			// Don't fail if loading fails - continue to next path or fall back
			if err := RegisterOnboardingPromptsFromSpec(server, specPath); err == nil {
				logMCPSpecFallbackSelection(server, "onboarding_prompts", specPath, selection.Reason)
				specLoaded = true
				break
			}
			// If loading failed, continue to next path
		}
	}

	// Fall back to programmatic registration if no spec was loaded
	// This is the default and should always work
	if !specLoaded {
		// Use programmatic registration as fallback
		logMCPSpecFallbackSelection(server, "onboarding_prompts", "programmatic", selection.Reason)
		registerOnboardingPromptsProgrammatically(server)
	}
}

// ExportPromptsToSpec exports the current programmatic prompts to a spec file
// This is a utility function to help migrate from programmatic to spec-based configuration
func ExportPromptsToSpec(outputPath string) error {
	spec := ConvertPromptsToSpec()

	// Ensure directory exists
	dir := filepath.Dir(outputPath)
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create directory").Wrap(err)
	}

	// Marshal to YAML
	data, err := yaml.Marshal(spec)
	if err != nil {
		return errfmt.Newf("failed to marshal spec").Wrap(err)
	}

	// Write to file
	if err := fileutil.WriteFile(outputPath, data, paths.FilePerm600); err != nil {
		return errfmt.Newf("failed to write spec file").Wrap(err)
	}

	return nil
}
