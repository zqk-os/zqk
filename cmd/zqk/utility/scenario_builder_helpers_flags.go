package utility

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/errfmt"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// ScenarioBuilderFlags contains parsed scenario-builder command flags
type ScenarioBuilderFlags struct {
	TargetDir         string
	ScenarioID        string
	ObjectIDs         []string
	Kinds             []string
	Counts            map[string]int
	Diversity         map[string]int
	DefaultCount      int
	DefaultDiversity  int
	LinkProb          float64
	TimeRange         time.Duration
	DryRun            bool
	SourceProject     string
	IDPrefixOverride  string
	NamespaceOverride string
	PreserveState     bool
	ChangePolicy      string
	FieldOverrides    map[string]any
	DataFile          string // Path to YAML file with object data
	Force             bool   // Force overwrite existing objects
}

// resolveTargetDirectory resolves the target directory path
func resolveTargetDirectory(targetDir string) string {
	if filepath.IsAbs(targetDir) {
		return targetDir
	}

	// Try to find project root from current directory
	projectRoot := cli.ResolveProjectRoot(".")
	if projectRoot == emptyValue {
		// If no project root found, assume target is relative to current directory
		wd, _ := fileutil.Getwd() //nolint:errcheck // Fallback to empty string if Getwd fails
		projectRoot = wd
	}

	return filepath.Join(projectRoot, targetDir)
}

// validateTargetDirectory is deprecated - we no longer require .zqk directory to exist
// The scenario builder will create the necessary directory structure if needed
// This function is kept for backward compatibility but does nothing
func validateTargetDirectory(targetDir string) error {
	// No validation needed - scenario builder will create structure as needed
	return nil
}

// parseScenarioBuilderFlags parses all scenario-builder command flags
func parseScenarioBuilderFlags(cmd *cobra.Command) (*ScenarioBuilderFlags, error) {
	flags := &ScenarioBuilderFlags{
		Counts:         make(map[string]int),
		Diversity:      make(map[string]int),
		FieldOverrides: make(map[string]any),
	}

	// Get target directory
	targetDir, err := cmd.Flags().GetString("target")
	if err != nil || targetDir == emptyValue {
		return nil, errfmt.Errorf("--target is required")
	}
	flags.TargetDir = targetDir

	// Get scenario ID
	scenarioID, _ := cmd.Flags().GetString("scenario-id")
	flags.ScenarioID = scenarioID

	// Get object IDs
	objectIDs, _ := cmd.Flags().GetStringSlice("object-ids")
	flags.ObjectIDs = objectIDs

	// Get kinds
	kinds, err := cmd.Flags().GetStringSlice("kinds")
	if err != nil {
		kinds = []string{}
	}
	flags.Kinds = kinds

	// Get counts
	countMap, err := cmd.Flags().GetStringToString("count")
	if err != nil {
		return nil, errfmt.Newf("failed to parse --count").Wrap(err)
	}
	for kind, countStr := range countMap {
		var count int
		if _, err := fmt.Sscanf(countStr, "%d", &count); err != nil {
			return nil, errfmt.Errorf("invalid count for %s: %s", kind, countStr)
		}
		flags.Counts[kind] = count
	}

	// Get diversity
	diversityMap, err := cmd.Flags().GetStringToString("diversity")
	if err != nil {
		return nil, errfmt.Newf("failed to parse --diversity").Wrap(err)
	}
	for kind, divStr := range diversityMap {
		var div int
		if _, err := fmt.Sscanf(divStr, "%d", &div); err != nil {
			return nil, errfmt.Errorf("invalid diversity for %s: %s", kind, divStr)
		}
		flags.Diversity[kind] = div
	}

	// Get other flags
	defaultCount, _ := cmd.Flags().GetInt("default-count")
	flags.DefaultCount = defaultCount

	defaultDiversity, _ := cmd.Flags().GetInt("default-diversity")
	flags.DefaultDiversity = defaultDiversity

	linkProb, _ := cmd.Flags().GetFloat64("link-probability")
	flags.LinkProb = linkProb

	timeRange, _ := cmd.Flags().GetDuration("time-range")
	flags.TimeRange = timeRange

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	flags.DryRun = dryRun

	force, _ := cmd.Flags().GetBool("force")
	flags.Force = force

	// Copy-related flags
	sourceProject, _ := cmd.Flags().GetString("source-project")
	flags.SourceProject = sourceProject

	idPrefixOverride, _ := cmd.Flags().GetString("id-prefix-override")
	flags.IDPrefixOverride = idPrefixOverride

	namespaceOverride, _ := cmd.Flags().GetString("namespace-override")
	flags.NamespaceOverride = namespaceOverride

	preserveState, _ := cmd.Flags().GetBool("preserve-state")
	flags.PreserveState = preserveState

	changePolicy, _ := cmd.Flags().GetString("change-policy")
	if changePolicy == emptyValue {
		changePolicy = "include" // Default: include current state
	}
	flags.ChangePolicy = changePolicy

	// Parse field overrides
	fieldOverrideMap, _ := cmd.Flags().GetStringToString("field-override")
	for k, v := range fieldOverrideMap {
		flags.FieldOverrides[k] = v
	}

	// Get data file
	dataFile, _ := cmd.Flags().GetString("data-file")
	flags.DataFile = dataFile

	return flags, nil
}
