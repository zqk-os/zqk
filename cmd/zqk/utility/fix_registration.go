package utility

import (
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/datacell"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"fmt"
	"path/filepath"
	"slices"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/appledouble"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/validation"
)

// NewFixRegistrationCmd creates a registration fixing command
func NewFixRegistrationCmd() *cobra.Command {
	var (
		kind   string
		all    bool
		fix    bool
		dryRun bool
		quiet  bool
	)

	helpBuilder := clipkg.DynamicHelpBuilder(
		"Fix registration issues (ID format mismatches)",
		"Fix registration issues where object IDs don't match their kind's expected format.",
		"",
		"This command helps resolve ID format mismatches by either:",
		"  - Updating the ID prefix configuration to accept alternative formats (--fix)",
		"  - Providing guidance on how to fix the object ID",
	).
		AddExample("Fix registration for a specific object", "%s utility fix-registration ADR-001").
		AddExample("Fix registration for all objects of a kind", "%s utility fix-registration --kind decision").
		AddExample("Fix all registration issues", "%s utility fix-registration --all").
		AddExample("Update config to accept found prefixes (dry run)", "%s utility fix-registration --all --fix --dry-run").
		ExcludeCommonFlags()

	fixRegistrationCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewUtilityFixRegistrationCommandBuilder(), &cobra.Command{
		Use: "fix-registration [id...]",
		Args: func(cmd *cobra.Command, args []string) error {
			// If --kind or --all is specified, IDs are optional
			if kind != emptyValue || all {
				return nil
			}
			// Otherwise, require at least one ID
			if len(args) == 0 {
				return errfmt.Errorf("requires at least one object ID, or use --kind or --all")
			}
			return nil
		},
	})
	cli.BindAsyncProgress(fixRegistrationCmd, func(cmd *cobra.Command, args []string) error {
		return runFixRegistration(cmd, args, kind, all, fix, dryRun, quiet)
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(fixRegistrationCmd)

	fixRegistrationCmd.Flags().StringVar(&kind, "kind", "", "Fix registration for all objects of the specified kind")
	fixRegistrationCmd.Flags().BoolVar(&all, "all", false, "Fix all registration issues in the system")
	fixRegistrationCmd.Flags().BoolVar(&fix, "fix", false, "Update id_prefixes config to accept discovered prefixes")
	fixRegistrationCmd.Flags().BoolVar(&dryRun, "dry-run", false, "With --fix, show what would be added without writing")
	fixRegistrationCmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Only show errors and fix summary")

	return fixRegistrationCmd
}

func runFixRegistration(cmd *cobra.Command, args []string, kind string, all bool, fix, dryRun, quiet bool) error {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return errfmt.Errorf("failed to get context")
	}

	profile := ctx.Profile
	if profile == emptyValue {
		profile = string(pkgctx.ProfileHuman)
	}
	logger := logging.GetLoggerFromProfile(profile)

	projectRoot := ctx.ProjectRoot
	if projectRoot == emptyValue {
		return errfmt.Errorf("not a ZQK project (no project root found)")
	}

	// Get ID validator
	idValidator := validation.GetIDValidator()
	if err := idValidator.LoadPatterns(); err != nil {
		return errfmt.Newf("failed to load ID patterns").Wrap(err)
	}

	var issues []registrationIssue

	if all {
		if !quiet {
			_ = cli.WriteOutput(cmd, []byte("Scanning objects for registration issues...\n")) //nolint:errcheck
		}
		issues = findAllRegistrationIssues(projectRoot, idValidator, logger)
	} else if kind != emptyValue {
		if !quiet {
			_ = cli.WriteOutput(cmd, []byte("Scanning objects for registration issues...\n")) //nolint:errcheck
		}
		issues = findKindRegistrationIssues(projectRoot, kind, idValidator, logger)
	} else {
		// Check specific IDs
		for _, id := range args {
			issue := checkRegistrationIssue(projectRoot, id, idValidator, logger)
			if issue != nil {
				issues = append(issues, *issue)
			}
		}
	}

	if len(issues) == 0 {
		msg := "No registration issues found\n"
		//nolint:errcheck // Output errors are non-critical
		_ = cli.WriteOutput(cmd, []byte(msg))
		return nil
	}

	// Optionally apply fix: add missing prefixes to id_prefixes_config
	if fix {
		added, err := applyRegistrationFix(cmd, projectRoot, issues, dryRun, quiet, logger)
		if err != nil {
			return err
		}
		if dryRun && len(added) > 0 {
			msg := fmt.Sprintf("Dry run: would add %d prefix(es) to id_prefixes config. No changes made.\n", len(added))
			_ = cli.WriteOutput(cmd, []byte(msg)) //nolint:errcheck
		} else if len(added) > 0 {
			validation.ResetGlobalIDPrefixesConfig()
			if !quiet {
				msg := fmt.Sprintf("Added %d prefix(es) to id_prefixes config. Run again to verify.\n", len(added))
				_ = cli.WriteOutput(cmd, []byte(msg)) //nolint:errcheck
			}
		}
		// When --fix we still report issues below unless quiet
	}

	if quiet && fix {
		return nil
	}

	// Report issues and provide guidance
	msg := fmt.Sprintf("Found %d registration issue(s):\n\n", len(issues))
	for _, issue := range issues {
		msg += fmt.Sprintf("  %s (kind: %s):\n", issue.ObjectID, issue.Kind)
		msg += fmt.Sprintf("    Issue: %s\n", issue.Message)
		msg += fmt.Sprintf("    Recommendation: %s\n\n", issue.Recommendation)
	}
	msg += "Note: Use --fix to update id_prefixes config to accept these prefixes, or fix object IDs to match expected formats.\n"

	//nolint:errcheck // Output errors are non-critical
	_ = cli.WriteOutput(cmd, []byte(msg))

	return nil
}

type registrationIssue struct {
	ObjectID       string
	Kind           string
	Message        string
	Recommendation string
}

func checkRegistrationIssue(projectRoot, objectID string, validator *validation.IDValidator, logger logging.Logger) *registrationIssue {
	// Find the object file
	processDir := datacell.ProcessPrimaryDir(projectRoot)
	var filePath string

	err := filepath.Walk(processDir, func(path string, info fileutil.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if appledouble.SkipPathInTreeWalk(path) {
			return nil
		}
		if filepath.Base(path) == objectID+".yaml" || filepath.Base(path) == objectID+".yml" {
			filePath = path
			return filepath.SkipAll
		}
		return nil
	})

	if err != nil || filePath == emptyValue {
		logging.Fluent(logger).Debug("Object file not found").
			ObjectID(objectID).
			Log()
		return nil
	}

	// Read and parse the object
	data, err := fileutil.ReadFile(filePath)
	if err != nil {
		logging.Fluent(logger).Debug("Failed to read object file").
			ObjectID(objectID).
			WithError(err).
			Log()
		return nil
	}

	var obj map[string]any
	if err := yaml.Unmarshal(data, &obj); err != nil {
		logging.Fluent(logger).Debug("Failed to parse object file").
			ObjectID(objectID).
			WithError(err).
			Log()
		return nil
	}

	objKind, ok := obj[objects.FieldKeyKind].(string)
	if !ok || objKind == emptyValue {
		return nil
	}

	// Validate the ID
	valid, err := validator.ValidateID(objectID, objKind)
	if err != nil || !valid {
		// Get expected prefixes
		config := validation.GetGlobalIDPrefixesConfig()
		var expectedPrefixes []string
		if config != nil {
			expectedPrefixes = config.GetPrefixesForKind(objKind)
		}

		recommendation := fmt.Sprintf("Update ID prefix configuration to accept '%s' prefix, or change object ID to match expected format", getPrefixFromID(objectID))
		if len(expectedPrefixes) > 0 {
			recommendation = fmt.Sprintf("Update ID prefix configuration to accept '%s' prefix, or change object ID to use one of: %v", getPrefixFromID(objectID), expectedPrefixes)
		}

		return &registrationIssue{
			ObjectID:       objectID,
			Kind:           objKind,
			Message:        fmt.Sprintf("ID format does not match kind %s", objKind),
			Recommendation: recommendation,
		}
	}

	return nil
}

func findKindRegistrationIssues(projectRoot, kind string, validator *validation.IDValidator, logger logging.Logger) []registrationIssue {
	var issues []registrationIssue

	// Get directory for this kind
	dirName := objects.GetDirectoryFromKind(kind)
	if dirName == emptyValue {
		return issues
	}

	kindDir := datacell.CellCASPrimaryDir(projectRoot, dirName)
	if _, err := fileutil.Stat(kindDir); fileutil.IsNotExist(err) {
		return issues
	}

	// Walk the kind directory
	err := filepath.Walk(kindDir, func(path string, info fileutil.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if appledouble.SkipPathInTreeWalk(path) {
			return nil
		}
		if filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml" {
			return nil
		}
		// Skip hash registry files
		if filepath.Base(path)[0] == '.' {
			return nil
		}

		// Extract object ID from filename
		filename := filepath.Base(path)
		objectID := filename[:len(filename)-len(filepath.Ext(filename))]

		issue := checkRegistrationIssue(projectRoot, objectID, validator, logger)
		if issue != nil {
			issues = append(issues, *issue)
		}

		return nil
	})

	if err != nil {
		logging.Fluent(logger).Warn("Error walking directory").
			String("dir", kindDir).
			WithError(err).
			Log()
	}

	return issues
}

func findAllRegistrationIssues(projectRoot string, validator *validation.IDValidator, logger logging.Logger) []registrationIssue {
	var issues []registrationIssue

	// This would be expensive for large systems, so we'll limit to known problematic kinds
	// or use system check to identify issues first
	processDir := datacell.ProcessPrimaryDir(projectRoot)

	// Walk all object directories
	err := filepath.Walk(processDir, func(path string, info fileutil.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if appledouble.SkipPathInTreeWalk(path) {
			return nil
		}
		if filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml" {
			return nil
		}
		// Skip hash registry files
		if filepath.Base(path)[0] == '.' {
			return nil
		}

		// Extract object ID from filename
		filename := filepath.Base(path)
		objectID := filename[:len(filename)-len(filepath.Ext(filename))]

		issue := checkRegistrationIssue(projectRoot, objectID, validator, logger)
		if issue != nil {
			issues = append(issues, *issue)
		}

		return nil
	})

	if err != nil {
		logging.Fluent(logger).Warn("Error walking process directory").
			WithError(err).
			Log()
	}

	return issues
}

func getPrefixFromID(id string) string {
	// Extract prefix from ID (e.g., "ADR-001" -> "ADR-")
	for i, r := range id {
		if r == '-' {
			return id[:i+1]
		}
	}
	return ""
}

// kindPrefix pairs kind with a prefix for deduplication
type kindPrefix struct {
	kind   string
	prefix string
}

// applyRegistrationFix loads id_prefixes config, adds missing (kind, prefix) entries, and writes back.
// Returns the list of "kind: prefix" that were added. If dryRun is true, no file is written.
func applyRegistrationFix(cmd *cobra.Command, projectRoot string, issues []registrationIssue, dryRun, quiet bool, logger logging.Logger) (added []string, err error) {
	configPath := filepath.Join(projectRoot, paths.ProcessInternalConfigsDir, paths.IdPrefixesConfigFile)
	if _, statErr := fileutil.Stat(configPath); statErr != nil {
		if fileutil.IsNotExist(statErr) {
			return nil, errfmt.Errorf("id_prefixes config not found at %s (create it or run without --fix to see recommendations)", configPath)
		}
		return nil, errfmt.Newf("cannot access id_prefixes config").Wrap(statErr)
	}

	config, loadErr := validation.LoadIDPrefixesConfig(configPath)
	if loadErr != nil {
		return nil, errfmt.Newf("failed to load id_prefixes config").Wrap(loadErr)
	}

	// Dedupe: collect unique (kind, prefix) from issues
	seen := make(map[kindPrefix]struct{})
	var toAdd []kindPrefix
	for _, issue := range issues {
		prefix := getPrefixFromID(issue.ObjectID)
		if prefix == emptyValue {
			continue
		}
		kp := kindPrefix{kind: issue.Kind, prefix: prefix}
		if _, ok := seen[kp]; ok {
			continue
		}
		seen[kp] = struct{}{}
		toAdd = append(toAdd, kp)
	}

	for _, kp := range toAdd {
		if ensurePrefixForKind(config, kp.kind, kp.prefix) {
			added = append(added, kp.kind+": "+kp.prefix)
			if !quiet {
				logging.Fluent(logger).Info("Adding prefix for kind").
					String("kind", kp.kind).
					String("prefix", kp.prefix).
					Log()
			}
		}
	}

	if dryRun || len(added) == 0 {
		return added, nil
	}

	// Write config: marshal and write atomically (temp then rename)
	data, marshalErr := yaml.Marshal(config)
	if marshalErr != nil {
		return nil, errfmt.Newf("failed to marshal id_prefixes config").Wrap(marshalErr)
	}
	tmpPath := configPath + ".tmp"
	if writeErr := fileutil.WriteFile(tmpPath, data, paths.FilePerm644); writeErr != nil { //nolint:gosec // config file - 0600 is standard
		return nil, errfmt.Newf("failed to write id_prefixes config").Wrap(writeErr)
	}
	if renameErr := fileutil.Rename(tmpPath, configPath); renameErr != nil {
		_ = fileutil.Remove(tmpPath) //nolint:errcheck // best effort cleanup
		return nil, errfmt.Newf("failed to replace id_prefixes config").Wrap(renameErr)
	}
	return added, nil
}

// ensurePrefixForKind adds prefix to the kind's list in config if not already present.
// Returns true if the prefix was added.
func ensurePrefixForKind(config *validation.IDPrefixesConfig, kind, prefix string) bool {
	if config.KindToPrefixes == nil {
		config.KindToPrefixes = make(map[string][]string)
	}
	list := config.KindToPrefixes[kind]
	if slices.Contains(list, prefix) {
		return false
	}
	config.KindToPrefixes[kind] = append(list, prefix)
	return true
}
