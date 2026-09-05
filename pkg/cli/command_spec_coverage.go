package cli

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/lanceman/zqk/pkg/objects"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

const (
	commandSpecYAMLExtension  = ".yaml"
	commandSpecYMLExtension   = ".yml"
	commandSpecFilenameSuffix = "_command"
)

// CommandSpecCoverage describes drift between a loaded Cobra tree and command specs.
type CommandSpecCoverage struct {
	Valid                   bool     `json:"valid" yaml:"valid"`
	Parity                  bool     `json:"parity" yaml:"parity"`
	SpecsDir                string   `json:"specs_dir" yaml:"specs_dir"`
	BaselinePath            string   `json:"baseline_path,omitempty" yaml:"baseline_path,omitempty"`
	LoadedCommandCount      int      `json:"loaded_command_count" yaml:"loaded_command_count"`
	CommandSpecCount        int      `json:"command_spec_count" yaml:"command_spec_count"`
	CommandsWithoutSpecs    []string `json:"commands_without_specs" yaml:"commands_without_specs"`
	SpecsWithoutCommands    []string `json:"specs_without_commands" yaml:"specs_without_commands"`
	NewCommandsWithoutSpecs []string `json:"new_commands_without_specs,omitempty" yaml:"new_commands_without_specs,omitempty"`
	NewSpecsWithoutCommands []string `json:"new_specs_without_commands,omitempty" yaml:"new_specs_without_commands,omitempty"`
	ResolvedCommands        []string `json:"resolved_commands,omitempty" yaml:"resolved_commands,omitempty"`
	ResolvedSpecs           []string `json:"resolved_specs,omitempty" yaml:"resolved_specs,omitempty"`
}

// CommandSpecCoverageBaseline records accepted historical drift. It is an
// enforcement inventory, not a command-definition source.
type CommandSpecCoverageBaseline struct {
	CommandsWithoutSpecs []string `json:"commands_without_specs"`
	SpecsWithoutCommands []string `json:"specs_without_commands"`
}

// AnalyzeCommandSpecCoverage compares static commands beneath root with specsDir.
func AnalyzeCommandSpecCoverage(root *cobra.Command, specsDir string) (CommandSpecCoverage, error) {
	loaded := make(map[string]string)
	walkStaticCommands(root, "", loaded)

	specs, err := loadCommandSpecIDs(specsDir)
	if err != nil {
		return CommandSpecCoverage{}, err
	}

	result := CommandSpecCoverage{
		SpecsDir:           specsDir,
		LoadedCommandCount: len(loaded),
		CommandSpecCount:   len(specs),
	}
	for id, commandPath := range loaded {
		if commandSpecCoverageIgnoredLoaded[id] || dynamicCommandGroup(id) {
			continue
		}
		if _, ok := specs[id]; !ok {
			result.CommandsWithoutSpecs = append(result.CommandsWithoutSpecs, commandPath)
		}
	}
	for id, relativePath := range specs {
		if commandSpecCoverageIgnoredSpecs[id] || dynamicCommandGroup(id) {
			continue
		}
		if _, ok := loaded[id]; !ok {
			result.SpecsWithoutCommands = append(result.SpecsWithoutCommands, relativePath)
		}
	}
	slices.Sort(result.CommandsWithoutSpecs)
	slices.Sort(result.SpecsWithoutCommands)
	result.Parity = len(result.CommandsWithoutSpecs) == 0 && len(result.SpecsWithoutCommands) == 0
	result.Valid = result.Parity
	return result, nil
}

// ApplyCommandSpecCoverageBaseline fails only for drift not present in baseline.
// Resolving historical drift remains valid and is reported separately.
func ApplyCommandSpecCoverageBaseline(
	coverage CommandSpecCoverage,
	baseline CommandSpecCoverageBaseline,
	baselinePath string,
) CommandSpecCoverage {
	coverage.BaselinePath = baselinePath
	coverage.NewCommandsWithoutSpecs, coverage.ResolvedCommands = compareCoverageSets(
		coverage.CommandsWithoutSpecs,
		baseline.CommandsWithoutSpecs,
	)
	coverage.NewSpecsWithoutCommands, coverage.ResolvedSpecs = compareCoverageSets(
		coverage.SpecsWithoutCommands,
		baseline.SpecsWithoutCommands,
	)
	coverage.Valid = len(coverage.NewCommandsWithoutSpecs) == 0 &&
		len(coverage.NewSpecsWithoutCommands) == 0
	return coverage
}

// LoadCommandSpecCoverageBaseline reads an accepted historical-drift baseline.
func LoadCommandSpecCoverageBaseline(path string) (CommandSpecCoverageBaseline, error) {
	data, err := fileutil.ReadFile(path)
	if err != nil {
		return CommandSpecCoverageBaseline{}, err
	}
	var baseline CommandSpecCoverageBaseline
	if err := json.Unmarshal(data, &baseline); err != nil {
		return CommandSpecCoverageBaseline{}, err
	}
	slices.Sort(baseline.CommandsWithoutSpecs)
	slices.Sort(baseline.SpecsWithoutCommands)
	return baseline, nil
}

// WriteCommandSpecCoverageBaseline replaces baseline with the measured drift.
func WriteCommandSpecCoverageBaseline(path string, coverage CommandSpecCoverage) error {
	baseline := CommandSpecCoverageBaseline{
		CommandsWithoutSpecs: coverage.CommandsWithoutSpecs,
		SpecsWithoutCommands: coverage.SpecsWithoutCommands,
	}
	data, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return fileutil.WriteFile(path, data, 0o644)
}

func compareCoverageSets(current, baseline []string) (added, removed []string) {
	currentSet := make(map[string]struct{}, len(current))
	for _, value := range current {
		currentSet[value] = struct{}{}
	}
	baselineSet := make(map[string]struct{}, len(baseline))
	for _, value := range baseline {
		baselineSet[value] = struct{}{}
		if _, ok := currentSet[value]; !ok {
			removed = append(removed, value)
		}
	}
	for _, value := range current {
		if _, ok := baselineSet[value]; !ok {
			added = append(added, value)
		}
	}
	slices.Sort(added)
	slices.Sort(removed)
	return added, removed
}

func walkStaticCommands(command *cobra.Command, parentPath string, commands map[string]string) {
	path := command.Name()
	if parentPath != "" {
		path = parentPath + " " + command.Name()
	} else if command.HasParent() == false {
		path = ""
	}
	if path != "" {
		commands[canonicalCommandID(path)] = path
	}
	for _, child := range command.Commands() {
		if child.Name() == "help" || child.Use == "help [command]" || child.Use == "completion [command]" {
			continue
		}
		walkStaticCommands(child, path, commands)
	}
}

func loadCommandSpecIDs(specsDir string) (map[string]string, error) {
	specs := make(map[string]string)
	err := filepath.WalkDir(specsDir, func(path string, entry fileutil.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !isCommandSpecYAML(path) {
			return nil
		}
		relativePath, err := filepath.Rel(specsDir, path)
		if err != nil {
			return err
		}
		id := commandSpecIDFromPath(relativePath)
		specs[id] = relativePath
		return nil
	})
	return specs, err
}

func isCommandSpecYAML(path string) bool {
	extension := filepath.Ext(path)
	return extension == commandSpecYAMLExtension || extension == commandSpecYMLExtension
}

func commandSpecIDFromPath(relativePath string) string {
	extension := filepath.Ext(relativePath)
	rawID := strings.TrimSuffix(relativePath, extension)
	rawID = strings.TrimSuffix(rawID, commandSpecFilenameSuffix)
	rawID = strings.TrimPrefix(rawID, "CSPEC-")
	rawID = strings.TrimSuffix(rawID, "-command")
	id := canonicalCommandID(rawID)

	parts := strings.Split(id, "_")
	deduplicated := parts[:0]
	for _, part := range parts {
		if len(deduplicated) == 0 || part != deduplicated[len(deduplicated)-1] {
			deduplicated = append(deduplicated, part)
		}
	}
	id = strings.Join(deduplicated, "_")
	id = strings.TrimSuffix(id, "_root")
	return strings.TrimPrefix(id, "root_")
}

func canonicalCommandID(value string) string {
	replacer := strings.NewReplacer(" ", "_", "-", "_", "/", "_")
	return replacer.Replace(value)
}

func dynamicCommandGroup(id string) bool {
	group, _, _ := strings.Cut(id, "_")
	return group == "object" || group == "internal" || group == "new"
}

var commandSpecCoverageIgnoredLoaded = map[string]bool{
	"agent_agent_new":               true,
	"agent_execute":                 true,
	"observer_coach":                true,
	"system_generate_spec_index":    true,
	"system_hydrate_graph":          true,
	"system_policy_interrupts_emit": true,
	"system_retention_tolerance":    true,
	"system_state_restore":          true,
	"system_sync_cas_index":         true,
	"system_validate_scenario":      true,
	"system_verify_completion":      true,
}

var commandSpecCoverageIgnoredSpecs = map[string]bool{
	"agent_new":                        true,
	"bundle":                           true,
	"bundle_apply":                     true,
	"project_use":                      true,
	"scheduler_events_aggregate":       true,
	"system_compact_maintenance_wal":   true,
	"system_maintenance_request_cycle": true,
	objects.FieldKeyVersion:            true,
}
