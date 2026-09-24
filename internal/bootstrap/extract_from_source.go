package bootstrap

import (
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/appledouble"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// ExtractFiles extracts bootstrap files into the project: .zqk/specs and .zqk/cli/specs.
// projectRoot is the repository root. When the binary was built with the bootstrap archive,
// the embedded archive is extracted (REQ-9009, REQ-9010). Otherwise, files are copied from
// source when running from the repo (development).
//
// TRACK: BLI-CEF-ARCH-SYSTEM-TRANCHE1 — moved from cmd/zqk/system (God-package tranche-1).
func ExtractFiles(projectRoot string, logger logging.Logger, force bool) error {
	if err := ExtractTo(projectRoot, logger, force); err == nil {
		return nil
	}
	// Fall back to source copy when embedded archive is missing or extraction fails (e.g. dev build)
	return extractFilesFromSource(projectRoot, logger, force)
}

// extractFilesFromSource copies bootstrap files from the source repo into the target project.
// Copies .zqk/specs -> projectRoot/.zqk/specs and .zqk/cli/specs -> projectRoot/.zqk/cli/specs.
// This is a development approach - production should use embedded filesystem.
func extractFilesFromSource(projectRoot string, logger logging.Logger, force bool) error {
	sourceRoot := FindSourceProjectRoot()
	if sourceRoot == "" {
		return errfmt.Errorf("cannot find project root for bootstrap extraction")
	}

	sourceDir := filepath.Join(sourceRoot, paths.ProcessInternalDir)
	if _, err := fileutil.Stat(sourceDir); fileutil.IsNotExist(err) {
		return errfmt.Errorf("source bootstrap directory not found: %s", sourceDir)
	}

	targetDir := filepath.Join(projectRoot, paths.ProcessInternalDir)
	if err := fileutil.EnsureDir(targetDir); err != nil {
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
		if _, err := fileutil.Stat(sourcePath); fileutil.IsNotExist(err) {
			sourcePath = filepath.Join(sourceDir, "configs", filename)
			targetPath = filepath.Join(targetDir, "configs", filename)
		}
		if _, err := fileutil.Stat(sourcePath); fileutil.IsNotExist(err) {
			logging.Fluent(logger).Warn("Bootstrap source file not found, skipping").
				File(filename).
				Log()
			continue
		}
		if _, err := fileutil.Stat(targetPath); err == nil && !force {
			continue
		}
		if err := copyBootstrapFile(sourcePath, targetPath, force); err != nil {
			return errfmt.Newf("failed to copy bootstrap file %s", filename).Wrap(err)
		}
		logging.Fluent(logger).Debug("Copied bootstrap file").
			File(filename).
			Dest(targetPath).
			Log()
	}

	if err := copyBootstrapSpecs(sourceDir, targetDir, logger, force); err != nil {
		return errfmt.Newf("failed to copy bootstrap specs").Wrap(err)
	}
	if err := copyBootstrapLifecycles(sourceDir, targetDir, logger, force); err != nil {
		return errfmt.Newf("failed to copy bootstrap lifecycles").Wrap(err)
	}

	// Copy CLI specs so command specs and schemas are available
	sourceCLISpecs := filepath.Join(sourceRoot, paths.CLICommandSpecsDir)
	targetCLISpecs := filepath.Join(projectRoot, paths.CLICommandSpecsDir)
	if info, err := fileutil.Stat(sourceCLISpecs); err == nil && info.IsDir() {
		if err := fileutil.EnsureDir(targetCLISpecs); err != nil {
			return errfmt.Newf("failed to create target .zqk/cli/specs").Wrap(err)
		}
		if err := copyDirRecursive(sourceCLISpecs, targetCLISpecs, force, logger); err != nil {
			return errfmt.Newf("failed to copy .zqk/cli/specs").Wrap(err)
		}
		logging.Fluent(logger).Debug("Copied CLI specs").
			Dest(targetCLISpecs).
			Log()
	}

	// Default policy pack templates for SeedDefaultPolicyPack
	sourcePolicies := filepath.Join(sourceRoot, "scripts", "default_policies")
	targetPolicies := filepath.Join(projectRoot, "scripts", "default_policies")
	if info, err := fileutil.Stat(sourcePolicies); err == nil && info.IsDir() {
		if err := fileutil.EnsureDir(targetPolicies); err != nil {
			return errfmt.Newf("failed to create target scripts/default_policies").Wrap(err)
		}
		if err := copyDirRecursive(sourcePolicies, targetPolicies, force, logger); err != nil {
			return errfmt.Newf("failed to copy scripts/default_policies").Wrap(err)
		}
	}

	// Default personas + agent_skills for SeedDefaultAgentSeatingPack (feed/chat out of the gate)
	for _, rel := range []string{"scripts/default_personas", "scripts/default_agent_skills"} {
		src := filepath.Join(sourceRoot, filepath.FromSlash(rel))
		dst := filepath.Join(projectRoot, filepath.FromSlash(rel))
		if info, err := fileutil.Stat(src); err == nil && info.IsDir() {
			if err := fileutil.EnsureDir(dst); err != nil {
				return errfmt.Newf("failed to create target %s", rel).Wrap(err)
			}
			if err := copyDirRecursive(src, dst, force, logger); err != nil {
				return errfmt.Newf("failed to copy %s", rel).Wrap(err)
			}
		}
	}

	// Default seed prompt templates (CEF evaluation templates)
	sourceTemplates := filepath.Join(sourceRoot, "packs", "code-eval", "templates")
	targetTemplates := filepath.Join(projectRoot, paths.ProcessDir, "prompt_templates")
	if info, err := fileutil.Stat(sourceTemplates); err == nil && info.IsDir() {
		if err := fileutil.EnsureDir(targetTemplates); err != nil {
			return errfmt.Newf("failed to create target seed prompt templates directory").Wrap(err)
		}
		if err := copyDirRecursive(sourceTemplates, targetTemplates, force, logger); err != nil {
			return errfmt.Newf("failed to copy seed prompt templates").Wrap(err)
		}
	}

	logging.Fluent(logger).Info("Bootstrap files extracted successfully").
		ProjectRoot(projectRoot).
		Log()
	return nil
}

// copyDirRecursive copies a directory tree from src to dst; respects force for existing files.
func copyDirRecursive(src, dst string, force bool, logger logging.Logger) error {
	entries, err := fileutil.ReadDir(src)
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
			if err := fileutil.EnsureDir(dstPath); err != nil {
				return err
			}
			if err := copyDirRecursive(srcPath, dstPath, force, logger); err != nil {
				return err
			}
			continue
		}
		if _, err := fileutil.Stat(dstPath); err == nil && !force {
			continue
		}
		if err := copyBootstrapFile(srcPath, dstPath, force); err != nil {
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

	if err := fileutil.EnsureDir(targetSpecsDir); err != nil {
		return errfmt.Newf("failed to create specs directory").Wrap(err)
	}

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
		// Meta: spec definitions for internal object_spec instances (object template / object create --internal).
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

		if _, err := fileutil.Stat(sourcePath); fileutil.IsNotExist(err) {
			logging.Fluent(logger).Warn("Bootstrap spec not found, skipping").
				File(specFile).
				Log()
			continue
		}

		if _, err := fileutil.Stat(targetPath); err == nil && !force {
			continue
		}

		if err := copyBootstrapFile(sourcePath, targetPath, force); err != nil {
			return errfmt.Newf("failed to copy spec %s", specFile).Wrap(err)
		}
	}

	return nil
}

// copyBootstrapLifecycles copies core lifecycle definitions to the target directory
func copyBootstrapLifecycles(sourceDir, targetDir string, logger logging.Logger, force bool) error {
	sourceLifecyclesDir := filepath.Join(sourceDir, "lifecycles")
	targetLifecyclesDir := filepath.Join(targetDir, "lifecycles")

	if err := fileutil.EnsureDir(targetLifecyclesDir); err != nil {
		return errfmt.Newf("failed to create lifecycles directory").Wrap(err)
	}

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
		"epic_lifecycle.yaml",
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

		if _, err := fileutil.Stat(sourcePath); fileutil.IsNotExist(err) {
			logging.Fluent(logger).Warn("Bootstrap lifecycle not found, skipping").
				File(lifecycleFile).
				Log()
			continue
		}

		if _, err := fileutil.Stat(targetPath); err == nil && !force {
			continue
		}

		if err := copyBootstrapFile(sourcePath, targetPath, force); err != nil {
			return errfmt.Newf("failed to copy lifecycle %s", lifecycleFile).Wrap(err)
		}
	}

	return nil
}

func copyBootstrapFile(sourcePath, targetPath string, force bool) error {
	data, err := fileutil.ReadFile(sourcePath)
	if err != nil {
		return errfmt.Newf("failed to read source file").Wrap(err)
	}

	targetDir := filepath.Dir(targetPath)
	if err := fileutil.EnsureDir(targetDir); err != nil {
		return errfmt.Newf("failed to create target directory").Wrap(err)
	}

	if fileutil.Exists(targetPath) && !force {
		return errfmt.Errorf("target file exists and --force not specified: %s", targetPath)
	}

	if err := fileutil.WriteStandardFile(targetPath, data); err != nil {
		return errfmt.Newf("failed to write target file").Wrap(err)
	}

	return nil
}

// FindSourceProjectRoot finds the repo root by walking up for .zqk/specs.
// Used by init seed packs when the working tree is not yet the install target.
func FindSourceProjectRoot() string {
	dir, err := fileutil.Getwd()
	if err != nil {
		return ""
	}

	for {
		testPath := filepath.Join(dir, paths.ProcessInternalDir)
		if _, err := fileutil.Stat(testPath); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return ""
}
