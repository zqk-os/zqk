package system

import (
	"os"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/storage/migration"
	"github.com/lanceman/zqk/pkg/zqkenv"

	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	clicontext "github.com/lanceman/zqk/internal/cli/context"
	"github.com/lanceman/zqk/pkg/appledouble"
	"github.com/lanceman/zqk/pkg/brand"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/docman"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/mcp"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

// InitMode represents the type of initialization
type InitMode string

const (
	InitModeGreenfield             InitMode = "greenfield" // Brand new project
	InitModeLegacy                 InitMode = "legacy"     // Existing project
	InitModeSnapshot               InitMode = "snapshot"   // From snapshot
	initErrCreateDirectoryFmt               = "failed to create directory %s: %w"
	initErrCreateProjectDataDirFmt          = "failed to create project data directory: %w"
	initErrCreateConfigFileFmt              = "failed to create config file: %w"
	initInternalDirName                     = "_internal"
	initInternalObjectSpecsDir              = "object_specs"
	initInternalLifecyclesDir               = "lifecycles"
	initInternalDocumentationDir            = "documentation"
	initBucketYearMonthA                    = "2025-12"
	initBucketYearMonthB                    = "2026-01"
	initLogFieldProject                     = "project"
	initLogFieldDirectory                   = "directory"
	initLogFieldMode                        = "mode"
	initLogFieldNote                        = "note"
	initLogFieldKind                        = "kind"
	initLogFieldPath                        = "path"
)

// runInit handles all three init scenarios plus optional discovery wizard (BLI-770), optional maintenance jobs, and optional onboarding roadmap job.
func runInit(_ *cobra.Command, projectName, template string, force bool, snapshotPath, answerFilePath string, legacy, merge, wipe, discover, withMaintenanceJobs, withOnboardingRoadmap, simple, advanced bool, importOntology string) error {
	logger := logging.GetLoggerFromProfile(systemProfileHuman)

	// Determine project root (respect ZQK_TEST_ROOT or ZQK_PROJECT_ROOT)
	projectRoot := determineProjectRoot()
	if projectRoot == emptyValue {
		cwd, err := fileutil.Getwd()
		if err != nil {
			return errfmt.Newf("failed to get current directory").Wrap(err)
		}
		projectRoot = cwd
	}

	// Use directory name as project name if not provided
	if projectName == emptyValue {
		projectName = filepath.Base(projectRoot)
	}

	// Auto-detect legacy mode for existing codebases (Painless Drop-In)
	if !legacy && snapshotPath == emptyValue {
		if _, err := fileutil.Stat(filepath.Join(projectRoot, paths.ProjectDataDir)); fileutil.IsNotExist(err) {
			entries, err := fileutil.ReadDir(projectRoot)
			if err == nil {
				for _, entry := range entries {
					name := entry.Name()
					if name != ".git" && name != ".zqk-test-settings.yaml" && name != "zqk-test-settings.yaml" {
						legacy = true
						logging.Fluent(logger).Info("Detected existing codebase; automatically enabling legacy mode for painless drop-in").Log()
						break
					}
				}
			}
		}
	}

	// Determine and execute init mode
	var mode InitMode
	var initErr error
	switch {
	case snapshotPath != emptyValue:
		mode = InitModeSnapshot
		logging.Fluent(logger).Info("Initializing ZQK project from snapshot").
			String(initLogFieldMode, string(mode)).
			String(initLogFieldProject, projectName).
			String(initLogFieldDirectory, projectRoot).
			Log()
		initErr = runSnapshotInit(projectRoot, projectName, snapshotPath, merge, wipe, force, logger)
	case legacy:
		mode = InitModeLegacy
		logging.Fluent(logger).Info("Initializing ZQK project (legacy mode)").
			String(initLogFieldMode, string(mode)).
			String(initLogFieldProject, projectName).
			String(initLogFieldDirectory, projectRoot).
			Log()
		initErr = runLegacyInit(projectRoot, projectName, template, force, logger)
	default:
		mode = InitModeGreenfield
		logging.Fluent(logger).Info("Initializing ZQK project").
			String(initLogFieldMode, string(mode)).
			String(initLogFieldProject, projectName).
			String(initLogFieldDirectory, projectRoot).
			Log()

		// Semantic Bridge Phase 1: Adaptive Interface
		// If discover is requested and a draft exists, we implicitly force initialization
		// to allow the wizard to resume without hitting the "project already initialized" error.
		if discover && hasDiscoveryDraft(projectRoot) {
			force = true
		}

		if simple {
			out := os.Stdout
			fmt.Fprintln(out, "Welcome to zqk!")
			fmt.Fprintln(out)
			fmt.Fprintln(out, "Let's set up your project:")
			// Minimal interactive mockup for Phase 1
			fmt.Fprintf(out, "1. What's your project name? [%s]\n", projectName)
			fmt.Fprintln(out, "2. How many people are on your team? [5]")
			fmt.Fprintln(out, "3. What type of project is this? [software]")
			fmt.Fprintln(out)
			fmt.Fprintln(out, "✓ Setting up zqk with default configurations...")
			fmt.Fprintln(out, "✓ Creating organizational structure...")
		} else if advanced {
			out := os.Stdout
			fmt.Fprintln(out, "Initializing in advanced semantic mode...")
			if importOntology != "" {
				fmt.Fprintf(out, "✓ Scheduled import for ontology: %s\n", importOntology)
			}
		}

		initErr = runGreenfieldInit(projectRoot, projectName, template, force, logger)

		if initErr == nil {
			// Phase 1 Semantic Bridge: Create base ontology templates
			baseOntologyErr := createBaseOntologies(projectRoot)
			if baseOntologyErr != nil {
				return errfmt.Newf("failed to create base ontologies").Wrap(baseOntologyErr)
			}

			if simple {
				out := os.Stdout
				fmt.Fprintln(out, "✓ Ready to use!")
			}
		}
	}
	if initErr != nil {
		return initErr
	}

	if err := clicontext.EnsureTestRootBrandSettingsFiles(projectRoot); err != nil {
		logging.Fluent(logger).Warn("Failed to write test-root brand settings (zqk-test-settings.yaml / test-settings.yaml); scheduler may fail until they exist").
			WithError(err).
			Log()
	}

	// Seed curated default policy pack ([REDACTED-ID] / [REDACTED-ID])
	if !legacy {
		if _, err := SeedDefaultPolicyPack(projectRoot, logger); err != nil {
			logging.Fluent(logger).Warn("Failed to seed default policy pack").
				WithError(err).
				String(initLogFieldNote, "Run again after fixing storage, or create policies manually from scripts/default_policies").
				Log()
		}
		// Community default personas + feed skill so emit-status --persona-ref works out of the gate.
		if _, err := SeedDefaultAgentSeatingPack(projectRoot, logger); err != nil {
			logging.Fluent(logger).Warn("Failed to seed default agent seating pack").
				WithError(err).
				String(initLogFieldNote, "Run again after fixing storage, or create personas/skills from scripts/default_personas and scripts/default_agent_skills").
				Log()
		}
	}

	// Seed kernel declarative answer file if provided (--answer-file)
	if answerFilePath != emptyValue {
		if _, err := SeedKernelFromAnswerFile(projectRoot, answerFilePath, logger); err != nil {
			logging.Fluent(logger).Warn("Failed to seed kernel from answer file").
				WithError(err).
				String("answer_file", answerFilePath).
				Log()
		}
	}

	// After init, run discovery wizard if requested (BLI-770)
	if discover {
		if legacy {
			if err := runLegacyDiscoverWizard(projectRoot, logger); err != nil {
				return err
			}
		} else {
			if err := runInteractiveWizard(projectRoot, logger); err != nil {
				return err
			}
		}
	}

	// After init, ensure maintenance jobs (retention_tolerance, audit_event_aggregation) if requested.
	// Fail loudly when --with-maintenance-jobs is set but jobs cannot be ensured ([REDACTED-ID]).
	if withMaintenanceJobs {
		result, err := EnsureRetentionJobsInProject(projectRoot, logger, nil)
		if err != nil {
			return errfmt.Newf("init --with-maintenance-jobs failed; run 'zqk system ensure-retention-jobs' after fixing the error").Wrap(err)
		}
		if result != nil && !result.AlreadySatisfied {
			logging.Fluent(logger).Info("Maintenance jobs ensured").
				String("summary", result.Message).
				Log()
		}
		if err := verifyMaintenanceJobsPresent(projectRoot, logger); err != nil {
			return err
		}
	}

	// After init, create onboarding roadmap seed scheduler job if requested (run scheduler to create curriculum)
	if withOnboardingRoadmap {
		if err := EnsureOnboardingRoadmapJobInProject(projectRoot, logger); err != nil {
			logging.Fluent(logger).Warn("Failed to ensure onboarding roadmap job; create manually from scripts/scheduler_jobs/onboarding_roadmap_seed.yaml").
				WithError(err).
				Log()
		}
	}

	if !withMaintenanceJobs {
		if _, err := EnsureRetentionJobsInProject(projectRoot, logger, nil); err != nil {
			logging.Fluent(logger).Warn("Failed to ensure maintenance jobs on init").WithError(err).Log()
		}
	}

	if err := injectAgentBootProtocol(projectRoot, legacy, logger); err != nil {
		logging.Fluent(logger).Warn("Failed to inject agent boot protocol").WithError(err).Log()
	}
	if err := generateGettingStartedGuide(projectRoot, logger); err != nil {
		logging.Fluent(logger).Warn("Failed to generate getting-started guide").WithError(err).Log()
	}

	exe := brand.ExecutableName()
	logging.Fluent(logger).Info("Initialization complete").
		String(initLogFieldProject, projectRoot).
		String(initLogFieldNote, "Run '"+exe+" system start-here'. Background loops: '"+exe+" scheduler start'.").
		Log()

	return nil
}

// determineProjectRoot determines project root from environment variables or auto-discovery.
// Greenfield init anchors to CWD when neither ZQK_PROJECT_ROOT nor ZQK_TEST_ROOT is set,
// preventing upward directory discovery from walking into parent/HOME .zqk markers (TDE-71e3706a).
func determineProjectRoot() string {
	// Check ZQK_PROJECT_ROOT first (explicit override)
	if root := zqkenv.ProjectRoot().Get(); root != emptyValue {
		if _, err := fileutil.Stat(root); err == nil {
			return root
		}
	}

	// Check ZQK_TEST_ROOT (for test scenarios)
	if root := zqkenv.TestRoot().Get(); root != emptyValue {
		if _, err := fileutil.Stat(root); err == nil {
			return root
		}
	}

	// Greenfield init anchors to CWD (handled by caller when empty)
	return ""
}

// runGreenfieldInit handles greenfield project initialization
func runGreenfieldInit(projectRoot, projectName, template string, force bool, logger logging.Logger) error {
	projectDataDir := filepath.Join(projectRoot, paths.ProjectDataDir)
	processDir := datacell.ProcessPrimaryDir(projectRoot)

	// Check if already initialized
	if !force {
		if _, err := fileutil.Stat(projectDataDir); err == nil {
			return errfmt.Errorf("project already initialized (found %s directory). Use --force to overwrite or --legacy for existing project", paths.ProjectDataDir)
		}
		if _, err := fileutil.Stat(processDir); err == nil {
			return errfmt.Errorf("project already initialized (found %s directory). Use --force to overwrite or --legacy for existing project", paths.ProcessDir)
		}
	}

	// Create directory structure
	if err := createProjectDataDir(projectDataDir, force); err != nil {
		return errfmt.Errorf(initErrCreateProjectDataDirFmt, err)
	}

	if err := createProcessDir(processDir, force); err != nil {
		return errfmt.Errorf("failed to create %s directory: %w", paths.ProcessDir, err)
	}

	// Extract bootstrap files (.zqk/specs and .zqk/cli/specs)
	internalDir := filepath.Join(processDir, initInternalDirName)
	if err := fileutil.MkdirAll(internalDir, paths.DirPerm755); err != nil {
		return errfmt.Errorf("failed to create "+initInternalDirName+" directory: %w", err)
	}

	if err := ExtractBootstrapFiles(projectRoot, logger, force); err != nil {
		logging.Fluent(logger).Warn("Failed to extract bootstrap files").
			WithError(err).
			String(initLogFieldNote, "Project may not be fully functional without bootstrap files").
			Log()
		// Don't fail init if bootstrap extraction fails - user can fix manually
	}

	if err := writeSystemAccount(projectRoot); err != nil {
		logging.Fluent(logger).Warn("Failed to create system account").WithError(err).Log()
	}

	if err := seedStarterKernelGraph(projectRoot, logger); err != nil {
		logging.Fluent(logger).Warn("Failed to seed starter kernel graph").WithError(err).Log()
	}

	// Create project config in .zqk/config/config.yaml and .zqk/config.yaml
	if err := writeProjectConfigFiles(projectDataDir, projectName, template, force); err != nil {
		return errfmt.Errorf(initErrCreateConfigFileFmt, err)
	}

	// Create MCP config in .zqk/mcp/config.yaml with alias_mode: true
	if err := writeMCPConfigFile(projectDataDir, force); err != nil {
		logging.Fluent(logger).Warn("Failed to create MCP config file").WithError(err).Log()
	}

	// Create config/zqk.yaml (committed defaults; local overrides in config/zqk-local.yaml)
	if err := writeBrandSettings(projectRoot, force); err != nil {
		logging.Fluent(logger).Warn("Failed to write config/zqk.yaml").WithError(err).Log()
	}

	// Write root isolation and kernel config files
	if err := writeRootIsolationFiles(projectRoot, force, logger); err != nil {
		logging.Fluent(logger).Warn("Failed to write root isolation files").WithError(err).Log()
	}

	// Persist bundled object_spec rows from .zqk/specs/objects (REQ-035 / CRIT-9035).
	if _, err := migration.EnsureBundledObjectSpecsMigrated(context.Background(), projectRoot, logger); err != nil {
		logging.Fluent(logger).Warn("Bundled object_spec migration did not complete").
			WithError(err).
			String("note", "Run from repo after fixing storage, or use zqk spec list (file fallback)").
			Log()
	}

	// Register shipped documentation in doc_entry graph (architecture, best-practices, onboarding).
	// Skips archive trees and docs/launch per TDE-1789629835679972000-dc60b78d / REQ-1789654891514127000-25cbd9d1.
	if reg, skip, err := docman.RegisterShippedDocs(context.Background(), projectRoot, logger); err != nil {
		return errfmt.Newf("failed to register shipped documentation in kernel graph").Wrap(err)
	} else if reg > 0 || skip > 0 {
		logging.Fluent(logger).Info("Shipped documentation registered in graph").
			Int("registered", reg).
			Int("skipped", skip).
			Log()
	}

	// Update .gitignore if .git exists
	if _, err := fileutil.Stat(filepath.Join(projectRoot, ".git")); err == nil {
		gitignorePath := filepath.Join(projectRoot, ".gitignore")
		if err := updateGitignore(gitignorePath); err != nil {
			logging.Fluent(logger).Warn("Failed to update .gitignore").
				WithError(err).
				Log()
		}
	}

	// Automatically configure MCP for supported IDEs and Agents
	if err := mcp.AutoInstall(projectRoot, logger); err != nil {
		logging.Fluent(logger).Warn("Failed to auto-install MCP server configuration").
			WithError(err).
			Log()
	}

	logging.Fluent(logger).Info("Greenfield project initialized successfully").
		String(initLogFieldProject, projectName).
		Log()
	return nil
}

// runLegacyInit handles legacy project initialization
func runLegacyInit(projectRoot, projectName, template string, force bool, logger logging.Logger) error {
	projectDataDir := filepath.Join(projectRoot, paths.ProjectDataDir)
	processDir := datacell.ProcessPrimaryDir(projectRoot)

	// Create project data directory if it doesn't exist
	if _, err := fileutil.Stat(projectDataDir); fileutil.IsNotExist(err) {
		if err := createProjectDataDir(projectDataDir, false); err != nil {
			return errfmt.Errorf(initErrCreateProjectDataDirFmt, err)
		}
		logging.Fluent(logger).Info("Created project data directory structure").Log()
	}

	// Create .zqk/process if it doesn't exist, or add missing directories
	if _, err := fileutil.Stat(processDir); fileutil.IsNotExist(err) {
		if err := createProcessDir(processDir, false); err != nil {
			return errfmt.Errorf("failed to create %s directory: %w", paths.ProcessDir, err)
		}
		logging.Fluent(logger).Info("Created " + paths.ProcessDir + " directory structure").Log()
	} else {
		// Add missing directories without removing existing ones
		if err := addMissingProcessDirectories(processDir); err != nil {
			return errfmt.Newf("failed to add missing directories").Wrap(err)
		}
		logging.Fluent(logger).Info("Added missing directories to existing " + paths.ProcessDir + " structure").Log()
	}

	// Create or update config file in .zqk/config/config.yaml and .zqk/config.yaml
	canonicalConfig := filepath.Join(projectDataDir, paths.ConfigDir, paths.ProjectConfigFile)
	legacyConfig := filepath.Join(projectDataDir, paths.ProjectConfigFile)
	_, errCanonical := fileutil.Stat(canonicalConfig)
	_, errLegacy := fileutil.Stat(legacyConfig)
	if fileutil.IsNotExist(errCanonical) || fileutil.IsNotExist(errLegacy) {
		if err := writeProjectConfigFiles(projectDataDir, projectName, template, false); err != nil {
			return errfmt.Errorf(initErrCreateConfigFileFmt, err)
		}
	}

	// Create or update MCP config in .zqk/mcp/config.yaml
	mcpConfig := filepath.Join(projectDataDir, paths.MCPDir, paths.MCPConfigFile)
	if _, err := fileutil.Stat(mcpConfig); fileutil.IsNotExist(err) {
		if err := writeMCPConfigFile(projectDataDir, false); err != nil {
			logging.Fluent(logger).Warn("Failed to create MCP config file").WithError(err).Log()
		}
	}

	if fileutil.IsNotExist(errCanonical) || fileutil.IsNotExist(errLegacy) {
		logging.Fluent(logger).Info("Created config file").
			Path(filepath.Join(paths.ProjectDataDir, paths.ConfigDir, paths.ProjectConfigFile)).
			Log()
	} else if force {
		if err := writeProjectConfigFiles(projectDataDir, projectName, template, true); err != nil {
			return errfmt.Newf("failed to update config file").Wrap(err)
		}
		logging.Fluent(logger).Info("Updated config file (--force)").
			Path(filepath.Join(paths.ProjectDataDir, paths.ConfigDir, paths.ProjectConfigFile)).
			Log()
	} else {
		logging.Fluent(logger).Info("Config file already exists, skipping (use --force to overwrite)").Log()
	}

	// Extract bootstrap files when _internal is missing, or when _internal exists but is empty
	// (e.g. addMissingProcessDirectories created empty _internal/object_specs), or when --force.
	internalDir := filepath.Join(processDir, initInternalDirName)
	objectSpecsDir := filepath.Join(internalDir, initInternalObjectSpecsDir)
	runBootstrap := force
	if _, err := fileutil.Stat(internalDir); fileutil.IsNotExist(err) {
		runBootstrap = true
	} else if !runBootstrap {
		entries, err := fileutil.ReadDir(objectSpecsDir)
		if err != nil || len(entries) == 0 {
			runBootstrap = true // backfill empty or missing specs
		}
	}
	if runBootstrap {
		if err := fileutil.MkdirAll(internalDir, paths.DirPerm755); err != nil {
			return errfmt.Errorf("failed to create "+initInternalDirName+" directory: %w", err)
		}
		if err := ExtractBootstrapFiles(projectRoot, logger, force); err != nil {
			logging.Fluent(logger).Warn("Failed to extract bootstrap files").
				WithError(err).
				String(initLogFieldNote, "Project may not be fully functional without bootstrap files").
				Log()
		}
	}
	if _, err := migration.EnsureBundledObjectSpecsMigrated(context.Background(), projectRoot, logger); err != nil {
		logging.Fluent(logger).Warn("Bundled object_spec migration did not complete").
			WithError(err).
			String("note", "Run from repo after fixing storage, or use zqk spec list (file fallback)").
			Log()
	}

	// Register shipped documentation in doc_entry graph (architecture, best-practices, onboarding).
	if reg, skip, err := docman.RegisterShippedDocs(context.Background(), projectRoot, logger); err != nil {
		return errfmt.Newf("failed to register shipped documentation").Wrap(err)
	} else if reg > 0 || skip > 0 {
		logging.Fluent(logger).Info("Shipped documentation registered in graph").
			Int("registered", reg).
			Int("skipped", skip).
			Log()
	}
	// Create system account if missing
	if err := writeSystemAccount(projectRoot); err != nil {
		logging.Fluent(logger).Warn("Failed to create system account").WithError(err).Log()
	}

	// Create config/zqk.yaml (committed defaults; local overrides in config/zqk-local.yaml)
	if err := writeBrandSettings(projectRoot, force); err != nil {
		logging.Fluent(logger).Warn("Failed to write config/zqk.yaml").WithError(err).Log()
	}

	// Write root isolation and kernel config files
	if err := writeRootIsolationFiles(projectRoot, force, logger); err != nil {
		logging.Fluent(logger).Warn("Failed to write root isolation files").WithError(err).Log()
	}

	// Automatically configure MCP for supported IDEs and Agents
	if err := mcp.AutoInstall(projectRoot, logger); err != nil {
		logging.Fluent(logger).Warn("Failed to auto-install MCP server configuration").
			WithError(err).
			Log()
	}

	// Discovery wizard runs when --discover is set (see runInit); use "zqk system check --auto-fix" to register hashes for discovered objects.

	logging.Fluent(logger).Info("Legacy project initialized successfully").
		String(initLogFieldProject, projectName).
		Log()
	return nil
}

// runSnapshotInit handles snapshot-based initialization
func runSnapshotInit(projectRoot, projectName, snapshotPath string, merge, wipe, force bool, logger logging.Logger) error {
	// Validate snapshot file exists
	if err := validateSnapshotFile(snapshotPath); err != nil {
		return err
	}

	// Load snapshot objects (compressed or JSON)
	var snapshotObjects []map[string]any
	var err error

	if strings.HasSuffix(snapshotPath, ".csnap") {
		// Load compressed snapshot
		expanded, loadErr := loadCompressedSnapshot(snapshotPath, logger)
		if loadErr != nil {
			return loadErr
		}

		// Find original project root
		originalProjectRoot, findErr := findOriginalProjectRoot(snapshotPath, logger)
		if findErr != nil {
			return findErr
		}

		logging.Fluent(logger).Info("Using original project root for object restoration").
			String("original_project_root", originalProjectRoot).
			Log()

		// Temporarily unset ZQK_TEST_ROOT
		defer unsetTestRoot()()

		logging.Fluent(logger).Info("Reading objects from original project").
			String("original_project_root", originalProjectRoot).
			Int("check_results", len(expanded)).
			Log()

		// Convert check results to objects
		snapshotObjects, err = convertCompressedSnapshotToObjects(expanded, originalProjectRoot, logger)
		if err != nil {
			return err
		}
	} else {
		// Load JSON snapshot
		snapshot, loadErr := LoadCheckSnapshot(snapshotPath)
		if loadErr != nil {
			return errfmt.Newf("failed to load check snapshot").Wrap(loadErr)
		}

		logging.Fluent(logger).Info("Loading objects from original project using file paths in snapshot").
			Int("results", len(snapshot.Results)).
			String("original_project_root", snapshot.Metadata.ProjectRoot).
			Log()

		// Convert check results to objects
		snapshotObjects, err = convertJSONSnapshotToObjects(snapshot, logger)
		if err != nil {
			return err
		}
	}

	// Handle wipe mode
	if err := handleWipeMode(projectRoot, wipe, force, logger); err != nil {
		return err
	}

	// Setup project directories
	if err := setupSnapshotProjectDirectories(projectRoot, force); err != nil {
		return err
	}

	// Extract bootstrap files (snapshot init also needs bootstrap files for validation)
	internalDir := filepath.Join(projectRoot, paths.ProcessInternalDir)
	if _, err := fileutil.Stat(internalDir); fileutil.IsNotExist(err) || force {
		if err := fileutil.MkdirAll(internalDir, paths.DirPerm755); err != nil {
			return errfmt.Errorf("failed to create "+initInternalDirName+" directory: %w", err)
		}

		if err := ExtractBootstrapFiles(projectRoot, logger, force); err != nil {
			logging.Fluent(logger).Warn("Failed to extract bootstrap files").
				WithError(err).
				String(initLogFieldNote, "Project may not be fully functional without bootstrap files").
				Log()
			// Don't fail init if bootstrap extraction fails - user can fix manually
		}
	}

	// Create project config
	projectDataDir := filepath.Join(projectRoot, paths.ProjectDataDir)
	if err := writeProjectConfigFiles(projectDataDir, projectName, "standard", force); err != nil {
		return errfmt.Errorf(initErrCreateConfigFileFmt, err)
	}

	// Restore objects from snapshot
	if err := restoreObjectsFromSnapshot(projectRoot, snapshotObjects, merge, logger); err != nil {
		return errfmt.Newf("failed to restore objects").Wrap(err)
	}

	if _, err := migration.EnsureBundledObjectSpecsMigrated(context.Background(), projectRoot, logger); err != nil {
		logging.Fluent(logger).Warn("Bundled object_spec migration did not complete after snapshot restore").
			WithError(err).
			Log()
	}

	logging.Fluent(logger).Info("Snapshot initialization completed successfully").
		String(initLogFieldProject, projectName).
		Int("objects_restored", len(snapshotObjects)).
		Log()

	return nil
}

// createAllKindDirectories creates directories for all known object kinds
func createAllKindDirectories(processDir string) error {
	// Get all known kinds from kind mappings (use EnsureReady for loader pattern / timeouts)
	mapper := objects.GetGlobalKindMapper()
	if err := mapper.EnsureReady(pkgctx.NewSystemContext()); err != nil {
		// If initialization fails, just create common directories
		// This can happen if processDir doesn't exist yet
		return nil
	}
	kinds := mapper.GetAllKinds()

	// Create _internal directories first
	internalDirs := []string{
		filepath.Join(processDir, initInternalDirName, initInternalObjectSpecsDir),
		filepath.Join(processDir, initInternalDirName, initInternalLifecyclesDir),
		filepath.Join(processDir, initInternalDirName, initInternalDocumentationDir),
	}
	for _, dir := range internalDirs {
		if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
			return errfmt.Errorf(initErrCreateDirectoryFmt, dir, err)
		}
	}

	// Create directories for each kind
	createdDirs := make(map[string]bool)
	for _, kind := range kinds {
		dirName := objects.GetDirectoryFromKind(kind)
		if dirName == emptyValue {
			continue
		}
		kindDir := filepath.Join(processDir, dirName)
		if !createdDirs[kindDir] {
			if err := fileutil.MkdirAll(kindDir, paths.DirPerm755); err != nil {
				return errfmt.Errorf(initErrCreateDirectoryFmt, kindDir, err)
			}
			createdDirs[kindDir] = true
		}
	}

	// Also create common subdirectories (bucketed storage only; object-kind dirs e.g. scheduler_jobs come from kind list above)
	commonSubdirs := []string{
		filepath.Join(processDir, "audit", initBucketYearMonthA),
		filepath.Join(processDir, "audit", initBucketYearMonthB),
		filepath.Join(processDir, "change_journal", initBucketYearMonthA),
		filepath.Join(processDir, "change_journal", initBucketYearMonthB),
	}
	for _, dir := range commonSubdirs {
		if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
			return errfmt.Errorf(initErrCreateDirectoryFmt, dir, err)
		}
	}

	return nil
}

// restoreObjectsFromSnapshot restores objects from snapshot to disk
func restoreObjectsFromSnapshot(projectRoot string, snapshotObjects []map[string]any, merge bool, logger logging.Logger) error {
	// Create storage factory
	ctx := pkgctx.NewSystemContext()
	factory, err := storage.NewStorageFactory(ctx, projectRoot)
	if err != nil {
		return errfmt.Newf("failed to create storage factory").Wrap(err)
	}

	storageProvider := factory.GetStorage()
	secCtx := pkgctx.NewSystemSecurityContext()

	restored := 0
	skipped := 0
	errors := 0

	for _, obj := range snapshotObjects {
		// Extract object ID and kind
		objID, ok := obj[objects.FieldKeyID].(string)
		if !ok || objID == emptyValue {
			logging.Fluent(logger).Warn("Skipping object without ID").
				String("object_id", "").
				Log()
			skipped++
			continue
		}

		objKind, ok := obj[objects.FieldKeyKind].(string)
		if !ok || objKind == emptyValue {
			logging.Fluent(logger).Warn("Skipping object without kind").
				ObjectID(objID).
				Log()
			skipped++
			continue
		}

		// Check if object already exists (for merge mode)
		if merge {
			_, err := storageProvider.Read(ctx, secCtx, objID)
			if err == nil {
				logging.Fluent(logger).Debug("Object already exists, skipping (merge mode)").
					ObjectID(objID).
					String(initLogFieldKind, objKind).
					Log()
				skipped++
				continue
			}
		}

		// Create object using storage provider
		// This ensures proper validation, hash registry updates, etc.
		// Note: Create signature is Create(ctx, secCtx, obj) - obj must have id and kind fields
		err := storageProvider.Create(ctx, secCtx, obj)
		if err != nil {
			// Check if it's just "already exists" error (for merge mode)
			if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "object exists") {
				logging.Fluent(logger).Debug("Object already exists, skipping").
					ObjectID(objID).
					String(initLogFieldKind, objKind).
					Log()
				skipped++
				continue
			}
			logging.Fluent(logger).Warn("Failed to restore object").
				ObjectID(objID).
				String(initLogFieldKind, objKind).
				WithError(err).
				Log()
			errors++
			continue
		}

		restored++
		if restored%100 == 0 {
			logging.Fluent(logger).Info("Restoring objects...").
				Int("restored", restored).
				Log()
		}
	}

	logging.Fluent(logger).Info("Object restoration completed").
		Int("restored", restored).
		Int("skipped", skipped).
		Int("errors", errors).
		Log()

	if errors > 0 {
		return errfmt.Errorf("restoration completed with %d errors", errors)
	}

	return nil
}

// addMissingProcessDirectories adds missing directories to existing process directory
func addMissingProcessDirectories(processDir string) error {
	// Get all required directories from createProcessDir
	requiredDirs := []string{
		filepath.Join(processDir, initInternalDirName, initInternalObjectSpecsDir),
		filepath.Join(processDir, initInternalDirName, initInternalLifecyclesDir),
		filepath.Join(processDir, initInternalDirName, initInternalDocumentationDir),
		filepath.Join(processDir, "backlog_items"),
		filepath.Join(processDir, "policies"),
		filepath.Join(processDir, "requirements"),
		filepath.Join(processDir, objects.KindCriteria),
		filepath.Join(processDir, "test_cases"),
		filepath.Join(processDir, "decisions"),
		filepath.Join(processDir, "goals"),
		filepath.Join(processDir, "milestones"),
		filepath.Join(processDir, "workstreams"),
		filepath.Join(processDir, "priority_plans"),
		filepath.Join(processDir, "questions"),
		filepath.Join(processDir, "doc_entries"),
		filepath.Join(processDir, "missions"),
		filepath.Join(processDir, "visions"),
		filepath.Join(processDir, "strategic_contexts"),
		filepath.Join(processDir, "stakeholder_profiles"),
		filepath.Join(processDir, "important_dates"),
		filepath.Join(processDir, "strategic_plans"),
		filepath.Join(processDir, "architecture"),
		filepath.Join(processDir, "audit", initBucketYearMonthA),
		filepath.Join(processDir, "change_journal", initBucketYearMonthA),
		filepath.Join(processDir, "planning"),
		filepath.Join(processDir, "scheduler_jobs"),
	}

	// Also add directories for all known kinds (use EnsureReady for loader pattern)
	mapper := objects.GetGlobalKindMapper()
	if err := mapper.EnsureReady(pkgctx.NewSystemContext()); err == nil {
		kinds := mapper.GetAllKinds()
		for _, kind := range kinds {
			dirName := objects.GetDirectoryFromKind(kind)
			if dirName != emptyValue {
				requiredDirs = append(requiredDirs, filepath.Join(processDir, dirName))
			}
		}
	}

	// Create missing directories
	for _, dir := range requiredDirs {
		if _, err := fileutil.Stat(dir); fileutil.IsNotExist(err) {
			if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
				return errfmt.Errorf(initErrCreateDirectoryFmt, dir, err)
			}
		}
	}

	return nil
}

type discoveryDraft struct {
	ProjectType         string `json:"project_type"`
	CustomerType        string `json:"customer_type"`
	Domain              string `json:"domain"`
	Vision              string `json:"vision"`
	Mission             string `json:"mission"`
	Stakeholder         string `json:"stakeholder"`
	Expectations        string `json:"expectations"`
	ImportantDate       string `json:"important_date"`
	ImportantDateReason string `json:"important_date_reason"`
}

func readDiscoveryDraft(projectRoot string) *discoveryDraft {
	draft := &discoveryDraft{}
	draftPath := filepath.Join(projectRoot, paths.ProjectDataDir, paths.CacheDir, "wizard_draft.json")
	if data, err := fileutil.ReadFile(draftPath); err == nil {
		_ = json.Unmarshal(data, draft)
	}
	return draft
}

func saveDiscoveryDraft(projectRoot string, draft *discoveryDraft) {
	draftPath := filepath.Join(projectRoot, paths.ProjectDataDir, paths.CacheDir, "wizard_draft.json")
	_ = fileutil.MkdirAll(filepath.Dir(draftPath), 0755)
	if data, err := json.Marshal(draft); err == nil {
		_ = fileutil.WriteFile(draftPath, data, 0644)
	}
}

func hasDiscoveryDraft(projectRoot string) bool {
	draftPath := filepath.Join(projectRoot, paths.ProjectDataDir, paths.CacheDir, "wizard_draft.json")
	if _, err := fileutil.Stat(draftPath); err == nil {
		return true
	}
	return false
}

func clearDiscoveryDraft(projectRoot string) {
	draftPath := filepath.Join(projectRoot, paths.ProjectDataDir, paths.CacheDir, "wizard_draft.json")
	_ = fileutil.Remove(draftPath)
}

func promptWizard(reader *bufio.Reader, out *fileutil.File, promptText, defaultVal string) string {
	if defaultVal != "" {
		fmt.Fprintf(out, "%s [%s]: ", promptText, defaultVal)
	} else {
		fmt.Fprintf(out, "%s: ", promptText)
	}
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultVal
	}
	return input
}

// runDiscoverWizard discovers existing object files under .zqk/process and reports counts per kind (BLI-770).
// runInteractiveWizard runs the interactive Project Discovery Wizard (BLI-770).
func runInteractiveWizard(projectRoot string, logger logging.Logger) error {
	reader := bufio.NewReader(os.Stdin)
	out := os.Stdout

	draft := readDiscoveryDraft(projectRoot)

	fmt.Fprintln(out, "\n--- Project Discovery Wizard ---")
	fmt.Fprintln(out, "Let's configure the strategic alignment for this project. (Your progress is auto-saved)")
	fmt.Fprintln(out)

	draft.ProjectType = promptWizard(reader, out, "Project Type (e.g., prototype, new product, enterprise initiative)", draft.ProjectType)
	saveDiscoveryDraft(projectRoot, draft)

	draft.CustomerType = promptWizard(reader, out, "Customer Type (e.g., individual, small team, enterprise)", draft.CustomerType)
	saveDiscoveryDraft(projectRoot, draft)

	draft.Domain = promptWizard(reader, out, "Domain (e.g., Technology, Finance)", draft.Domain)
	saveDiscoveryDraft(projectRoot, draft)

	fmt.Fprintln(out)
	draft.Vision = promptWizard(reader, out, "Vision (What is the desired future state?)", draft.Vision)
	saveDiscoveryDraft(projectRoot, draft)

	draft.Mission = promptWizard(reader, out, "Mission (Why does this project exist?)", draft.Mission)
	saveDiscoveryDraft(projectRoot, draft)

	fmt.Fprintln(out)
	draft.Stakeholder = promptWizard(reader, out, "Primary Stakeholder (e.g., Executive Team, Customers)", draft.Stakeholder)
	saveDiscoveryDraft(projectRoot, draft)

	draft.Expectations = promptWizard(reader, out, "Stakeholder Expectations (comma-separated)", draft.Expectations)
	saveDiscoveryDraft(projectRoot, draft)

	expectationsStr := strings.Split(draft.Expectations, ",")
	for i := range expectationsStr {
		expectationsStr[i] = strings.TrimSpace(expectationsStr[i])
	}

	fmt.Fprintln(out)
	draft.ImportantDate = promptWizard(reader, out, "Important Date (e.g., 2026-12-31)", draft.ImportantDate)
	saveDiscoveryDraft(projectRoot, draft)

	draft.ImportantDateReason = promptWizard(reader, out, "Important Date Reason (e.g., Product Launch)", draft.ImportantDateReason)
	saveDiscoveryDraft(projectRoot, draft)

	// Save the objects ([REDACTED-ID]: create first-class mission/vision).
	ctx := pkgctx.NewSystemContext()
	factory, err := storage.NewStorageFactory(ctx, projectRoot)
	if err != nil {
		return errfmt.Newf("failed to create storage factory").Wrap(err)
	}
	sp := factory.GetStorage()
	secCtx := pkgctx.NewSystemSecurityContext()
	var createErrs []string
	var missionID, visionID string

	createOrCollect := func(label string, obj map[string]any) string {
		if err := sp.Create(ctx, secCtx, obj); err != nil {
			logging.Fluent(logger).Warn("Failed to create " + label).WithError(err).Log()
			createErrs = append(createErrs, fmt.Sprintf("%s: %v", label, err))
			return emptyValue
		}
		return objects.GetString(obj, objects.FieldKeyID)
	}

	// 1. Mission (required field: mission_statement) — let storage assign ID
	missionTitle := draft.Mission
	if missionTitle == emptyValue {
		missionTitle = "Project mission"
	}
	missionID = createOrCollect("mission", map[string]any{
		objects.FieldKeyKind:             objects.KindMission,
		objects.FieldKeyTitle:            missionTitle,
		objects.FieldKeyMissionStatement: draft.Mission,
		objects.FieldKeyStatus:           objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion:    objects.DefaultSchemaVersion,
	})

	// 2. Vision (required field: narrative)
	visionTitle := draft.Vision
	if visionTitle == emptyValue {
		visionTitle = "Project vision"
	}
	visionObj := map[string]any{
		objects.FieldKeyKind:          objects.KindVision,
		objects.FieldKeyTitle:         visionTitle,
		objects.FieldKeyNarrative:     draft.Vision,
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	if missionID != emptyValue {
		visionObj[objects.FieldKeyMissionRefs] = []string{missionID}
	}
	visionID = createOrCollect("vision", visionObj)
	_ = visionID

	// 3. Strategic context (aggregates wizard answers)
	createOrCollect("strategic_context", map[string]any{
		objects.FieldKeyKind:        objects.KindStrategicContext,
		objects.FieldKeyTitle:       fmt.Sprintf("Strategic Context for %s", draft.ProjectType),
		objects.FieldKeyContextType: "project_foundation",
		objects.FieldKeyContent:     fmt.Sprintf("Vision: %s\nMission: %s\nDomain: %s\nCustomer Type: %s", draft.Vision, draft.Mission, draft.Domain, draft.CustomerType),
	})

	// 4. Stakeholder profile
	createOrCollect("stakeholder_profile", map[string]any{
		objects.FieldKeyKind:            objects.KindStakeholderProfile,
		objects.FieldKeyTitle:           draft.Stakeholder,
		objects.FieldKeyStakeholderType: "decision_maker",
		objects.FieldKeyExpectations:    expectationsStr,
	})

	// 5. Important date
	createOrCollect("important_date", map[string]any{
		objects.FieldKeyKind:        objects.KindImportantDate,
		objects.FieldKeyTitle:       draft.ImportantDateReason,
		objects.FieldKeyDate:        draft.ImportantDate,
		objects.FieldKeyDateType:    "deadline",
		objects.FieldKeyImportance:  "high",
		objects.FieldKeyImpactScope: "project_wide",
	})

	if len(createErrs) > 0 {
		return errfmt.Errorf("discovery wizard failed to create foundational objects: %s (draft retained at .zqk/cache/wizard_draft.json)", strings.Join(createErrs, "; "))
	}

	clearDiscoveryDraft(projectRoot)

	fmt.Fprintln(out, "\n✓ Discovery complete. Mission, vision, strategic context, stakeholder profile, and important date objects created.")
	return nil
}

// Use with "zqk system init --legacy --discover".
func runLegacyDiscoverWizard(projectRoot string, logger logging.Logger) error {
	processDir := datacell.ProcessPrimaryDir(projectRoot)
	if _, err := fileutil.Stat(processDir); fileutil.IsNotExist(err) {
		logging.Fluent(logger).Info("No "+paths.ProcessDir+" directory found; nothing to discover").
			String(initLogFieldDirectory, projectRoot).
			Log()
		return nil
	}

	kinds := discoverObjectKinds(processDir)
	if len(kinds) == 0 {
		logging.Fluent(logger).Info("Discovery found no object kinds (field registry may not be loaded)").Log()
		return nil
	}

	var total int
	type kindCount struct {
		kind  string
		count int
	}
	var counts []kindCount
	for _, kind := range kinds {
		kindDir := getKindDirectory(projectRoot, kind)
		if kindDir == emptyValue {
			continue
		}
		if _, err := fileutil.Stat(kindDir); fileutil.IsNotExist(err) {
			continue
		}
		var n int
		_ = filepath.Walk(kindDir, func(path string, info fileutil.FileInfo, err error) error {
			if err != nil {
				return nil // skip errors, continue walking
			}
			if info.IsDir() {
				return nil
			}
			if appledouble.SkipPathInTreeWalk(path) {
				return nil
			}
			if strings.HasSuffix(strings.ToLower(info.Name()), ".yaml") {
				n++
			}
			return nil
		})
		if n > 0 {
			counts = append(counts, kindCount{kind: kind, count: n})
			total += n
		}
	}

	if total == 0 {
		logging.Fluent(logger).Info("Discovery complete: no object files found under "+paths.ProcessDir).
			String(initLogFieldDirectory, processDir).
			Log()
		return nil
	}

	logging.Fluent(logger).Info("Discovery complete: existing objects by kind").
		Total(total).
		KindsWithObjects(len(counts)).
		Log()
	for _, c := range counts {
		logging.Fluent(logger).Info("  "+c.kind+": "+fmt.Sprintf("%d", c.count)+" object(s)").
			String(initLogFieldKind, c.kind).
			Int("count", c.count).
			Log()
	}
	logging.Fluent(logger).Info("To register hashes and validate discovered objects, run: zqk system check --auto-fix").Log()
	return nil
}

func createBaseOntologies(projectRoot string) error {
	baseDir := filepath.Join(projectRoot, paths.ProjectDataDir, "ontologies", "base")
	if err := fileutil.MkdirAll(baseDir, paths.DirPerm755); err != nil {
		return err
	}

	orgContent := []byte(`ontology:
  id: organizational_base
  namespace: domain:organizational:*
  version: "1.0.0"
  description: "Base organizational structure ontology"
  maturity_level: 0

objects:
  - id: organization
    kind: organization
    fields:
      - name: organization_name
        type: string
        semantic_type: identifier
      - name: divisions
        type: list
        item_type: division_ref
  
  - id: division
    kind: division
    fields:
      - name: division_name
        type: string
        semantic_type: identifier
      - name: parent_division_ref
        type: division_ref
      - name: teams
        type: list
        item_type: team_ref
`)

	if err := fileutil.WriteFile(filepath.Join(baseDir, "organizational.yaml"), orgContent, paths.FilePerm644); err != nil {
		return err
	}

	partnerContent := []byte(`ontology:
  id: partnership_base
  namespace: domain:partnership:*
  version: "1.0.0"
  description: "Base partnership structure ontology"

objects:
  - id: partnership
    kind: partnership
    fields:
      - name: partner_organizations
        type: list
        item_type: organization_ref
      - name: partnership_type
        type: enum
        values: [code_owner_feature_builder, joint_development, service_provider]
`)

	if err := fileutil.WriteFile(filepath.Join(baseDir, "partnership.yaml"), partnerContent, paths.FilePerm644); err != nil {
		return err
	}

	return nil
}

func agentBootProtocolText() string {
	exe := brand.ExecutableName()
	prod := brand.ProductName()
	return fmt.Sprintf(`
# %s agent boot
You are connected to the %s knowledge kernel.

First commands:
- %s workflow whats-next --format json
- %s object list
- %s system start-here
- %s mcp install

Process data under .zqk/process/ only through the CLI (object create / update / promote).
Do not export ZQK_PROJECT_ROOT in your shell profile.
Status transitions use promote, not a raw status-field update.
`, prod, prod, exe, exe, exe, exe)
}

func injectAgentBootProtocol(projectRoot string, isLegacy bool, logger logging.Logger) error {
	ruleFiles := []string{".iderules", ".clinerules", ".windsurfrules"}
	for _, file := range ruleFiles {
		path := filepath.Join(projectRoot, file)
		f, err := fileutil.OpenFile(path, fileutil.O_APPEND|fileutil.O_CREATE|fileutil.O_WRONLY, 0644)
		if err != nil {
			logging.Fluent(logger).Warn("Could not open rule file for appending").String("file", file).WithError(err).Log()
			continue
		}

		if _, err := f.WriteString(agentBootProtocolText()); err != nil {
			_ = f.Close()
			return err
		}
		_ = f.Close()
	}
	return nil
}

func generateGettingStartedGuide(projectRoot string, logger logging.Logger) error {
	path := filepath.Join(projectRoot, "ZQK_GETTING_STARTED.md")
	// Only create if it doesn't already exist (e.g., in a legacy project)
	if _, err := fileutil.Stat(path); err == nil {
		return nil
	}

	exe := brand.ExecutableName()
	prod := brand.ProductName()

	template := fmt.Sprintf(`# Welcome to %s

Initialized. Next steps:

1. `+"`"+`%s system agent-onboard --format json`+"`"+`
2. `+"`"+`%s system start-here`+"`"+`
3. `+"`"+`%s mcp install`+"`"+`
4. `+"`"+`%s object list`+"`"+`
5. `+"`"+`%s workflow whats-next --format json`+"`"+`

Kernel data stays under `+"`.zqk/`"+`. Process YAML goes through the CLI, not a text editor.
Do not export ZQK_PROJECT_ROOT in your shell profile.

`+"`"+`%s --help`+"`"+` lists commands. If this checkout ships onboarding docs, start at docs/onboarding/COMMUNITY_FIRST_RUN.md.
`, prod, exe, exe, exe, exe, exe, exe)

	return fileutil.WriteSecureFile(path, []byte(template))
}
