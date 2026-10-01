package cli

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
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
		hasSpec := false
		for _, cand := range equivalentCommandIDs(id) {
			if _, ok := specs[cand]; ok {
				hasSpec = true
				break
			}
		}
		if !hasSpec {
			result.CommandsWithoutSpecs = append(result.CommandsWithoutSpecs, commandPath)
		}
	}
	for id, relativePath := range specs {
		if commandSpecCoverageIgnoredSpecs[id] || dynamicCommandGroup(id) {
			continue
		}
		hasLoaded := false
		for _, cand := range equivalentCommandIDs(id) {
			if _, ok := loaded[cand]; ok {
				hasLoaded = true
				break
			}
		}
		if !hasLoaded {
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
		len(coverage.NewSpecsWithoutCommands) == 0 &&
		len(coverage.SpecsWithoutCommands) == 0
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
	if len(coverage.SpecsWithoutCommands) > 0 {
		return errfmt.Errorf("refusing to write baseline with %d orphaned specs without commands (fix path mapping or add to ignored specs): %s",
			len(coverage.SpecsWithoutCommands), strings.Join(coverage.SpecsWithoutCommands, ", "))
	}
	commandsWithoutSpecs := coverage.CommandsWithoutSpecs
	if commandsWithoutSpecs == nil {
		commandsWithoutSpecs = []string{}
	}
	specsWithoutCommands := coverage.SpecsWithoutCommands
	if specsWithoutCommands == nil {
		specsWithoutCommands = []string{}
	}
	baseline := CommandSpecCoverageBaseline{
		CommandsWithoutSpecs: commandsWithoutSpecs,
		SpecsWithoutCommands: specsWithoutCommands,
	}
	data, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return fileutil.WriteFile(path, data, paths.FilePerm644)
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
	id = strings.TrimPrefix(id, "root_")
	if strings.HasPrefix(id, "mcp_svc_") {
		id = "mcp_" + strings.TrimPrefix(id, "mcp_svc_")
	}
	return id
}

func canonicalCommandID(value string) string {
	replacer := strings.NewReplacer(" ", "_", "-", "_", "/", "_")
	return replacer.Replace(value)
}

func equivalentCommandIDs(id string) []string {
	cands := []string{id}

	switch id {
	case "do":
		cands = append(cands, "workflow_do")
	case "workflow_do":
		cands = append(cands, "do")
	case "inspect":
		cands = append(cands, "object_inspect")
	case "object_inspect":
		cands = append(cands, "inspect")
	case "mutate":
		cands = append(cands, "object_mutate")
	case "object_mutate":
		cands = append(cands, "mutate")
	case "query":
		cands = append(cands, "graph_query")
	case "graph_query":
		cands = append(cands, "query")
	case "rollback":
		cands = append(cands, "object_rollback")
	case "object_rollback":
		cands = append(cands, "rollback")
	case "completion":
		cands = append(cands, "system_completion")
	case "system_completion":
		cands = append(cands, "completion")
	case "learn":
		cands = append(cands, "agent_learn")
	case "agent_learn":
		cands = append(cands, "learn")
	case "new":
		cands = append(cands, "object_new")
	case "object_new":
		cands = append(cands, "new")
	case "join":
		cands = append(cands, "graph_join")
	case "graph_join":
		cands = append(cands, "join")
	case "mcp_cursor_adapter":
		cands = append(cands, "vendor_cursor_adapter")
	case "vendor_cursor_adapter":
		cands = append(cands, "mcp_cursor_adapter")
	}

	if id == "pplan" || strings.HasPrefix(id, "pplan_") {
		cands = append(cands, "object_"+id)
	} else if id == "object_pplan" || strings.HasPrefix(id, "object_pplan_") {
		cands = append(cands, strings.TrimPrefix(id, "object_"))
	}

	if id == "spec" || strings.HasPrefix(id, "spec_") {
		cands = append(cands, "object_"+id)
	} else if id == "object_spec" || strings.HasPrefix(id, "object_spec_") {
		cands = append(cands, strings.TrimPrefix(id, "object_"))
	}

	if id == "sync" || strings.HasPrefix(id, "sync_") {
		cands = append(cands, "mesh_"+id)
	} else if id == "mesh_sync" || strings.HasPrefix(id, "mesh_sync_") {
		cands = append(cands, strings.TrimPrefix(id, "mesh_"))
	}

	if id == "pre_commit" || strings.HasPrefix(id, "pre_commit_") {
		cands = append(cands, "system_"+id)
	} else if id == "system_pre_commit" || strings.HasPrefix(id, "system_pre_commit_") {
		cands = append(cands, strings.TrimPrefix(id, "system_"))
	}

	if id == "reports" || strings.HasPrefix(id, "reports_") {
		cands = append(cands, "system_"+id)
	} else if id == "system_reports" || strings.HasPrefix(id, "system_reports_") {
		cands = append(cands, strings.TrimPrefix(id, "system_"))
	}

	if id == "tray" || strings.HasPrefix(id, "tray_") {
		cands = append(cands, "service_"+id)
	} else if id == "service_tray" || strings.HasPrefix(id, "service_tray_") {
		cands = append(cands, strings.TrimPrefix(id, "service_"))
	}

	if id == "validate" || strings.HasPrefix(id, "validate_") {
		cands = append(cands, "system_"+id)
	} else if id == "system_validate" || strings.HasPrefix(id, "system_validate_") {
		cands = append(cands, strings.TrimPrefix(id, "system_"))
	}

	if id == "feed" || strings.HasPrefix(id, "feed_") {
		cands = append(cands, "agent_"+id)
	} else if id == "agent_feed" || strings.HasPrefix(id, "agent_feed_") {
		cands = append(cands, strings.TrimPrefix(id, "agent_"))
	}

	return cands
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
	"system_generate_agent_configs":    true,
	"system_maintenance_request_cycle": true,
	objects.FieldKeyVersion:            true,
}
