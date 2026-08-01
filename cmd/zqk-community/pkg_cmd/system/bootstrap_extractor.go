package system

import (
	"os"
	"path/filepath"

	"github.com/lanceman/zqk/internal/bootstrap"
	"github.com/lanceman/zqk/pkg/appledouble"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
)

// ExtractBootstrapFiles extracts bootstrap files into the project: docs/process/_internal and docs/process/command_specs.
// projectRoot is the repository root. When the binary was built with the bootstrap archive,
// the embedded archive is extracted (REQ-9009, REQ-9010). Otherwise, files are copied from
// source when running from the repo (development).
func ExtractBootstrapFiles(projectRoot string, logger logging.Logger, force bool) error {
	if err := bootstrap.ExtractTo(projectRoot, logger, force); err == nil {
		return nil
	}
	// Fall back to source copy when embedded archive is missing or extraction fails (e.g. dev build)
	return extractBootstrapFilesFromSource(projectRoot, logger, force)
}

// extractBootstrapFilesFromSource copies bootstrap files from the source repo into the target project.
// Copies docs/process/_internal -> projectRoot/docs/process/_internal and docs/process/command_specs -> projectRoot/docs/process/command_specs.
// This is a development approach - production should use embedded filesystem.
func extractBootstrapFilesFromSource(projectRoot string, logger logging.Logger, force bool) error {
	sourceRoot := findProjectRootForBootstrap()
	if sourceRoot == emptyValue {
		return errfmt.Errorf("cannot find project root for bootstrap extraction")
	}

	sourceDir := filepath.Join(sourceRoot, paths.ProcessInternalDir)
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		return errfmt.Errorf("source bootstrap directory not found: %s", sourceDir)
	}

	targetDir := filepath.Join(projectRoot, paths.ProcessInternalDir)
	if err := os.MkdirAll(targetDir, paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create target _internal").Wrap(err)
	}

	// Required config files to copy (root of _internal or under configs/)
	requiredFiles := []string{
		"id_prefixes_config.yaml",
		"kind_mappings_config.yaml",
		"namespaces_config.yaml",
		"paths_config.yaml",
		"blocking_check_config.yaml",
		"scanner_config.yaml",
	}
	for _, filename := range requiredFiles {
		sourcePath := filepath.Join(sourceDir, filename)
		targetPath := filepath.Join(targetDir, filename)
		if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
			sourcePath = filepath.Join(sourceDir, "configs", filename)
			targetPath = filepath.Join(targetDir, "configs", filename)
		}
		if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
			logging.Fluent(logger).Warn("Bootstrap source file not found, skipping").
				File(filename).
				Log()
			continue
		}
		if _, err := os.Stat(targetPath); err == nil && !force {
			continue
		}
		if err := copyFile(sourcePath, targetPath, force); err != nil {
			return errfmt.Newf("failed to copy bootstrap file %s", filename).Wrap(err)
		}
		logging.Fluent(logger).Debug("Copied bootstrap file").
			File(filename).
			Target(targetPath).
			Log()
	}

	if err := copyBootstrapSpecs(sourceDir, targetDir, logger, force); err != nil {
		return errfmt.Newf("failed to copy bootstrap specs").Wrap(err)
	}
	if err := copyBootstrapLifecycles(sourceDir, targetDir, logger, force); err != nil {
		return errfmt.Newf("failed to copy bootstrap lifecycles").Wrap(err)
	}

	// Copy CLI specs so command specs and schemas are available
	sourceCLISpecs := filepath.Join(sourceRoot, paths.ProjectDataDir, paths.CLISpecsDir)
	targetCLISpecs := filepath.Join(projectRoot, "docs/process/command_specs")
	if info, err := os.Stat(sourceCLISpecs); err == nil && info.IsDir() {
		if err := os.MkdirAll(targetCLISpecs, paths.DirPerm755); err != nil {
			return errfmt.Newf("failed to create target docs/process/command_specs").Wrap(err)
		}
		if err := copyDirRecursive(sourceCLISpecs, targetCLISpecs, force, logger); err != nil {
			return errfmt.Newf("failed to copy docs/process/command_specs").Wrap(err)
		}
		logging.Fluent(logger).Debug("Copied CLI specs").
			Target(targetCLISpecs).
			Log()
	}

	// Default policy pack templates for SeedDefaultPolicyPack
	sourcePolicies := filepath.Join(sourceRoot, "scripts", "default_policies")
	targetPolicies := filepath.Join(projectRoot, "scripts", "default_policies")
	if info, err := os.Stat(sourcePolicies); err == nil && info.IsDir() {
		if err := os.MkdirAll(targetPolicies, paths.DirPerm755); err != nil {
			return errfmt.Newf("failed to create target scripts/default_policies").Wrap(err)
		}
		if err := copyDirRecursive(sourcePolicies, targetPolicies, force, logger); err != nil {
			return errfmt.Newf("failed to copy scripts/default_policies").Wrap(err)
		}
	}

	logging.Fluent(logger).Info("Bootstrap files extracted successfully").
		ProjectRoot(projectRoot).
		Log()
	return nil
}

// copyDirRecursive copies a directory tree from src to dst; respects force for existing files.
func copyDirRecursive(src, dst string, force bool, logger logging.Logger) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, ent := range entries {
		if appledouble.SkipNameInReadDir(ent.Name()) {
			continue
		}
		srcPath := filepath.Join(src, ent.Name())
		dstPath := filepath.Join(dst, ent.Name())
		if ent.IsDir() {
			if err := os.MkdirAll(dstPath, paths.DirPerm755); err != nil {
				return err
			}
			if err := copyDirRecursive(srcPath, dstPath, force, logger); err != nil {
				return err
			}
			continue
		}
		if _, err := os.Stat(dstPath); err == nil && !force {
			continue
		}
		if err := copyFile(srcPath, dstPath, force); err != nil {
			return err
		}
		if logger != nil {
			logging.Fluent(logger).Debug("Copied file").
				File(ent.Name()).
				Path(dstPath).
				Log()
		}
	}
	return nil
}

// copyBootstrapSpecs copies core object specs to the target directory
func copyBootstrapSpecs(sourceDir, targetDir string, logger logging.Logger, force bool) error {
	sourceSpecsDir := filepath.Join(sourceDir, "object_specs")
	targetSpecsDir := filepath.Join(targetDir, "object_specs")

	// Create target directory
	if err := os.MkdirAll(targetSpecsDir, paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create specs directory").Wrap(err)
	}

	// Core specs that must be included for basic operations
	coreSpecs := []string{
		"base_object.yaml",
		"auditable.yaml",
		"policy.yaml",
		"backlog_item.yaml",
		"goal.yaml",
		"milestone.yaml",
		"workstream.yaml",
		"priority_plan.yaml",
		"requirement.yaml",
		"criteria.yaml",
		"test_case.yaml",
		"decision.yaml",
		"roadmap.yaml",
		"component.yaml",
		"account.yaml",
		"role.yaml",
		"mission.yaml",
		"vision.yaml",
		"strategic_context.yaml",
		"stakeholder_profile.yaml",
		"important_date.yaml",
		"strategic_plan.yaml",
		"release.yaml",
		"doc_entry.yaml",
		"audit_event.yaml",
		"change_journal_entry.yaml",
		"integrity_manifest.yaml",
		"kind_synonym.yaml",
		"domain_registry.yaml",
		"namespace.yaml",
		"namespace_registry.yaml",
		// Meta: spec definitions for internal object_spec instances (zqk new internal object_spec, internal CRUD).
		"object_spec.yaml",
		"evolution_management.yaml",
		"auth_strategy.yaml",
		"keystore_entry.yaml",
		"workflow.yaml",
		"workstream_transition.yaml",
		"strategic_plan.yaml",
		"technical_debt.yaml",
		"code_quality_metric.yaml",
		"verification_matrix.yaml",
	}

	for _, specFile := range coreSpecs {
		sourcePath := filepath.Join(sourceSpecsDir, specFile)
		targetPath := filepath.Join(targetSpecsDir, specFile)

		if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
			logging.Fluent(logger).Warn("Bootstrap spec not found, skipping").
				String("spec", specFile).
				Log()
			continue
		}

		if _, err := os.Stat(targetPath); err == nil && !force {
			continue
		}

		if err := copyFile(sourcePath, targetPath, force); err != nil {
			return errfmt.Newf("failed to copy spec %s", specFile).Wrap(err)
		}
	}

	return nil
}

// copyBootstrapLifecycles copies core lifecycle definitions to the target directory
func copyBootstrapLifecycles(sourceDir, targetDir string, logger logging.Logger, force bool) error {
	sourceLifecyclesDir := filepath.Join(sourceDir, "lifecycles")
	targetLifecyclesDir := filepath.Join(targetDir, "lifecycles")

	// Create target directory
	if err := os.MkdirAll(targetLifecyclesDir, paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create lifecycles directory").Wrap(err)
	}

	// Core lifecycles that must be included
	coreLifecycles := []string{
		"base_lifecycle.yaml",
		"policy_lifecycle.yaml",
		"backlog_item_lifecycle.yaml",
		"goal_lifecycle.yaml",
		"milestone_lifecycle.yaml",
		"workstream_lifecycle.yaml",
		"priority_plan_lifecycle.yaml",
		"requirement_lifecycle.yaml",
		"criteria_lifecycle.yaml",
		"test_case_lifecycle.yaml",
		"decision_lifecycle.yaml",
		"roadmap_lifecycle.yaml",
		"component_lifecycle.yaml",
		"account_lifecycle.yaml",
		"role_lifecycle.yaml",
		"mission_lifecycle.yaml",
		"vision_lifecycle.yaml",
		"release_lifecycle.yaml",
		"doc_entry_lifecycle.yaml",
		"audit_event_lifecycle.yaml",
		"change_journal_entry_lifecycle.yaml",
		"kind_synonym_lifecycle.yaml",
		"domain_registry_lifecycle.yaml",
		"namespace_lifecycle.yaml",
		"namespace_registry_lifecycle.yaml",
		"evolution_management_lifecycle.yaml",
		"auth_strategy_lifecycle.yaml",
		"keystore_entry_lifecycle.yaml",
		"workflow_lifecycle.yaml",
		"workstream_transition_lifecycle.yaml",
		"strategic_plan_lifecycle.yaml",
		"technical_debt_lifecycle.yaml",
		"code_quality_lifecycle.yaml",
		"verification_matrix_lifecycle.yaml",
	}

	for _, lifecycleFile := range coreLifecycles {
		sourcePath := filepath.Join(sourceLifecyclesDir, lifecycleFile)
		targetPath := filepath.Join(targetLifecyclesDir, lifecycleFile)

		if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
			logging.Fluent(logger).Warn("Bootstrap lifecycle not found, skipping").
				String("lifecycle_file", lifecycleFile).
				Log()
			continue
		}

		if _, err := os.Stat(targetPath); err == nil && !force {
			continue
		}

		if err := copyFile(sourcePath, targetPath, force); err != nil {
			return errfmt.Newf("failed to copy lifecycle %s", lifecycleFile).Wrap(err)
		}
	}

	return nil
}

// copyFile copies a file from source to target
func copyFile(sourcePath, targetPath string, force bool) error {
	// Read source file
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return errfmt.Newf("failed to read source file").Wrap(err)
	}

	// Create target directory if needed
	targetDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDir, paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create target directory").Wrap(err)
	}

	// Check if target exists
	if _, err := os.Stat(targetPath); err == nil && !force {
		return errfmt.Errorf("target file exists and --force not specified: %s", targetPath)
	}

	// Write target file
	if err := os.WriteFile(targetPath, data, paths.FilePerm644); err != nil { //nolint:gosec // Extracted files - 0600 is acceptable for user-readable files
		return errfmt.Newf("failed to write target file").Wrap(err)
	}

	return nil
}

// findProjectRootForBootstrap finds the project root by looking for docs/process/_internal
func findProjectRootForBootstrap() string {
	// Start from current directory
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}

	// Walk up directory tree looking for docs/process/_internal
	for {
		testPath := filepath.Join(dir, paths.ProcessInternalDir)
		if _, err := os.Stat(testPath); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break // Reached filesystem root
		}
		dir = parent
	}

	return ""
}
