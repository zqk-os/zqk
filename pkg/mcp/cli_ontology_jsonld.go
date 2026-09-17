package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/zqktime"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// CommandMetrics represents usage metrics for a CLI command
type CommandMetrics struct {
	InvocationCount  int           `json:"invocationCount"`
	SuccessCount     int           `json:"successCount"`
	FailureCount     int           `json:"failureCount"`
	AvgDuration      time.Duration `json:"avgDuration"`
	BaselineDuration time.Duration `json:"baselineDuration"`
	ErrorRate        float64       `json:"errorRate"`
	LastSeen         time.Time     `json:"lastSeen"`
}

// JSONLDCLIOntology represents the CLI command structure in JSON-LD format
type JSONLDCLIOntology struct {
	Context  map[string]any       `json:"@context"`
	Type     string               `json:"@type"`
	ID       string               `json:"@id"`
	Label    string               `json:"rdfs:label"`
	Comment  string               `json:"rdfs:comment"`
	Version  string               `json:"zqk:version,omitempty"`
	Commands []JSONLDCommand      `json:"zqk:commands"`
	Groups   []JSONLDCommandGroup `json:"zqk:groups,omitempty"`
	Metrics  *JSONLDMetrics       `json:"zqk:metrics,omitempty"` // Optional: include metrics
}

// JSONLDCommandGroup represents a command group (e.g., "object", "system")
type JSONLDCommandGroup struct {
	Type     string   `json:"@type"`
	ID       string   `json:"@id"`
	Name     string   `json:"zqk:name"`
	Label    string   `json:"rdfs:label"`
	Comment  string   `json:"rdfs:comment,omitempty"`
	Commands []string `json:"zqk:commands"` // Command paths in this group
}

// JSONLDCommand represents a CLI command in JSON-LD format
type JSONLDCommand struct {
	Type         string                `json:"@type"`
	ID           string                `json:"@id"`
	Path         string                `json:"zqk:path"`            // Full path: "object list"
	Name         string                `json:"zqk:name"`            // Command name: "list"
	Group        string                `json:"zqk:group,omitempty"` // Command group: "object"
	Label        string                `json:"rdfs:label"`
	Comment      string                `json:"rdfs:comment,omitempty"`
	Short        string                `json:"zqk:short,omitempty"`
	Long         string                `json:"zqk:long,omitempty"`
	Subcommands  []string              `json:"zqk:subcommands,omitempty"` // Paths of subcommands
	Flags        []JSONLDFlag          `json:"zqk:flags,omitempty"`
	Permissions  []string              `json:"zqk:permissions,omitempty"`
	Roles        []string              `json:"zqk:roles,omitempty"`
	IsExecutable bool                  `json:"zqk:isExecutable"`      // Has RunE or Run
	Metrics      *JSONLDCommandMetrics `json:"zqk:metrics,omitempty"` // Optional: include metrics
}

// JSONLDFlag represents a command flag in JSON-LD format
type JSONLDFlag struct {
	Name        string `json:"zqk:name"`
	Type        string `json:"zqk:type"` // "string", "boolean", "number", etc.
	Description string `json:"rdfs:comment,omitempty"`
	Default     any    `json:"zqk:default,omitempty"`
	Required    bool   `json:"zqk:required,omitempty"`
	Shorthand   string `json:"zqk:shorthand,omitempty"`
}

// JSONLDMetrics represents aggregated metrics for the CLI ontology
type JSONLDMetrics struct {
	TotalCommands   int    `json:"zqk:totalCommands"`
	TotalExecutions int    `json:"zqk:totalExecutions,omitempty"`
	TotalFailures   int    `json:"zqk:totalFailures,omitempty"`
	LastUpdated     string `json:"zqk:lastUpdated,omitempty"`
}

// JSONLDCommandMetrics represents metrics for a specific command
type JSONLDCommandMetrics struct {
	InvocationCount  int     `json:"zqk:invocationCount,omitempty"`
	SuccessCount     int     `json:"zqk:successCount,omitempty"`
	FailureCount     int     `json:"zqk:failureCount,omitempty"`
	AvgDuration      string  `json:"zqk:avgDuration,omitempty"`      // Duration string
	BaselineDuration string  `json:"zqk:baselineDuration,omitempty"` // Duration string
	ErrorRate        float64 `json:"zqk:errorRate,omitempty"`        // Percentage
	LastSeen         string  `json:"zqk:lastSeen,omitempty"`
}

// ConvertCommandTreeToJSONLD converts the Cobra command tree to JSON-LD ontology
func ConvertCommandTreeToJSONLD(rootCmd *cobra.Command, includeMetrics bool) (*JSONLDCLIOntology, error) {
	context := getJSONLDContext()

	// Discover all commands
	discoveredCommands := DiscoverCLICommands(rootCmd)

	// Build command tree structure
	commands := make([]JSONLDCommand, 0, len(discoveredCommands))
	groupMap := make(map[string][]string) // group -> command paths

	// Load metrics if requested
	var allMetrics map[string]*CommandMetrics
	if includeMetrics {
		// Try to load metrics (non-blocking - if it fails, continue without metrics)
		metricsPath := filepath.Join(paths.ProjectDataDir, paths.MetricsDir, paths.CommandMetricsFile)
		if data, err := os.ReadFile(metricsPath); err == nil { //nolint:gosec
			var fileData map[string]*CommandMetrics
			if err := json.Unmarshal(data, &fileData); err == nil {
				allMetrics = fileData
			}
		}
	}

	for _, cmd := range discoveredCommands {
		jsonldCmd := convertDiscoveredCommandToJSONLD(cmd, allMetrics)
		commands = append(commands, jsonldCmd)

		// Group commands by top-level group
		group := ExtractGroupFromPath(cmd.Path)
		if group != emptyValue {
			groupMap[group] = append(groupMap[group], cmd.Path)
		}
	}

	// Build groups
	groups := make([]JSONLDCommandGroup, 0, len(groupMap))
	for groupName, cmdPaths := range groupMap {
		groups = append(groups, JSONLDCommandGroup{
			Type:     "zqk:CommandGroup",
			ID:       fmt.Sprintf("zqk:cli:group:%s", groupName),
			Name:     groupName,
			Label:    groupName,
			Comment:  fmt.Sprintf("Command group: %s", groupName),
			Commands: cmdPaths,
		})
	}

	// Build metrics summary if available
	var metrics *JSONLDMetrics
	if allMetrics != nil {
		totalExecutions := 0
		totalFailures := 0
		for _, m := range allMetrics {
			totalExecutions += m.InvocationCount
			totalFailures += m.FailureCount
		}
		metrics = &JSONLDMetrics{
			TotalCommands:   len(commands),
			TotalExecutions: totalExecutions,
			TotalFailures:   totalFailures,
			LastUpdated:     zqktime.NowRFC3339UTC(),
		}
	}

	return &JSONLDCLIOntology{
		Context:  context,
		Type:     "zqk:CLIOntology",
		ID:       "zqk:cli",
		Label:    "ZQK CLI Command Ontology",
		Comment:  "Complete CLI command structure and organization",
		Commands: commands,
		Groups:   groups,
		Metrics:  metrics,
	}, nil
}

// convertDiscoveredCommandToJSONLD converts a DiscoveredCommand to JSON-LD format
func convertDiscoveredCommandToJSONLD(cmd *DiscoveredCommand, metrics map[string]*CommandMetrics) JSONLDCommand {
	commandID := fmt.Sprintf("zqk:cli:command:%s", strings.ReplaceAll(cmd.Path, " ", ":"))
	group := ExtractGroupFromPath(cmd.Path)

	// Extract flags
	flags := make([]JSONLDFlag, 0)
	if cmd.Command != nil {
		cmd.Command.Flags().VisitAll(func(flag *pflag.Flag) {
			if flag.Hidden {
				return
			}

			jsonldFlag := JSONLDFlag{
				Name:        flag.Name,
				Type:        MapFlagTypeToJSONLDType(flag.Value.Type()),
				Description: flag.Usage,
				Shorthand:   flag.Shorthand,
			}

			// Add default value
			if flag.DefValue != emptyValue {
				jsonldFlag.Default = ParseDefaultValue(flag.DefValue, flag.Value.Type())
			}

			flags = append(flags, jsonldFlag)
		})
	}

	// Extract subcommand paths
	subcommands := make([]string, 0, len(cmd.Subcommands))
	for _, subCmd := range cmd.Subcommands {
		subcommands = append(subcommands, subCmd.Path)
	}

	// Check if executable
	isExecutable := cmd.Command != nil && (cmd.Command.RunE != nil || cmd.Command.Run != nil)

	// Add metrics if available
	var commandMetrics *JSONLDCommandMetrics
	if metrics != nil {
		if m, exists := metrics[cmd.Path]; exists {
			commandMetrics = &JSONLDCommandMetrics{
				InvocationCount:  m.InvocationCount,
				SuccessCount:     m.SuccessCount,
				FailureCount:     m.FailureCount,
				AvgDuration:      m.AvgDuration.String(),
				BaselineDuration: m.BaselineDuration.String(),
				ErrorRate:        m.ErrorRate,
				LastSeen:         m.LastSeen.Format(time.RFC3339),
			}
		}
	}

	comment := cmd.Short
	if cmd.Long != emptyValue {
		comment = cmd.Long
	}

	return JSONLDCommand{
		Type:         "zqk:CLICommand",
		ID:           commandID,
		Path:         cmd.Path,
		Name:         cmd.Use,
		Group:        group,
		Label:        cmd.Path,
		Comment:      comment,
		Short:        cmd.Short,
		Long:         cmd.Long,
		Subcommands:  subcommands,
		Flags:        flags,
		Permissions:  cmd.Permissions,
		Roles:        cmd.Roles,
		IsExecutable: isExecutable,
		Metrics:      commandMetrics,
	}
}

// extractGroupFromPath is a convenience wrapper around ExtractGroupFromPath.
// Kept for backward compatibility with existing code.
func extractGroupFromPath(path string) string {
	return ExtractGroupFromPath(path)
}
