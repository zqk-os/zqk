package system

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/datacell"

	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/appledouble"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// NewRepairYAMLCmd creates a command to repair YAML files with parsing errors
func NewRepairYAMLCmd() *cobra.Command {
	var (
		filePath string
		id       string
		kind     string
		dryRun   bool
	)

	examplePolicyFile := filepath.Join(paths.ProcessPoliciesDir, "POLICY-CODE-009.yaml")
	longHelp := fmt.Sprintf(`Repair YAML files that have parsing errors by extracting fields and rewriting using instance builders.

This command is useful when YAML files become corrupted or have syntax errors that prevent normal parsing.
It uses Python to extract fields from the corrupted file, then uses instance builders to rewrite the file correctly.

Examples:
  # Repair a specific file
  %s system repair-yaml --file %s

  # Repair by object ID (finds the file automatically)
  %s system repair-yaml --id POLICY-CODE-009

  # Dry run to see what would be repaired
  %s system repair-yaml --id POLICY-CODE-009 --dry-run`, paths.CLICommandName, examplePolicyFile, paths.CLICommandName, paths.CLICommandName)

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemRepairYamlCommandBuilder(), &cobra.Command{
		Use:   "repair-yaml",
		Short: "Repair YAML files with parsing errors using instance builders",
		Long:  longHelp,
	})
	cli.BindAsyncProgress(cmd, cli.WithProcessor(func(cmd *cobra.Command, args []string, processor *cli.Processor) error {
		var err error
		_ = err

		logger := processor.Logger()
		projectRoot := processor.ProjectRoot()

		return RunRepairYAMLViaPipeline(cmd, projectRoot, filePath, id, kind, dryRun, logger)
	}))

	cmd.Flags().StringVar(&filePath, "file", "", "Path to YAML file to repair")
	cmd.Flags().StringVar(&id, "id", "", "Object ID (will find file automatically)")
	cmd.Flags().StringVar(&kind, "kind", "", "Object kind (auto-detected if not specified)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be repaired without making changes")

	return cmd
}

// extractFieldsFromCorruptedYAML extracts fields from a corrupted YAML file (pure Go implementation)
func extractFieldsFromCorruptedYAML(filePath string) (map[string]any, string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, "", errfmt.Newf("failed to read file").Wrap(err)
	}

	lines := strings.Split(string(data), "\n")

	// Find body field
	bodyStartIdx := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "body:" || strings.HasPrefix(strings.TrimSpace(line), "body:") {
			bodyStartIdx = i
			break
		}
	}

	instance := make(map[string]any)

	if bodyStartIdx == -1 {
		// No body field - try to parse normally (might work if corruption is elsewhere)
		if err := yaml.Unmarshal(data, &instance); err == nil {
			// Successfully parsed - extract kind
			extractedKind := ""
			if k, ok := instance[objects.FieldKeyKind].(string); ok {
				extractedKind = k
			}
			return instance, extractedKind, nil
		}
		// Still corrupted, return empty instance
		return instance, "", nil
	}

	// Parse metadata (everything before body)
	if bodyStartIdx > 0 {
		metadataYAML := strings.Join(lines[:bodyStartIdx], "\n")
		var metadata map[string]any
		if err := yaml.Unmarshal([]byte(metadataYAML), &metadata); err == nil {
			// Copy metadata fields to instance
			maps.Copy(instance, metadata)
		} else {
			// Even metadata parsing failed - try to extract key fields manually
			// Look for common fields like id, kind, schema_version in the first part
			for _, line := range lines[:bodyStartIdx] {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "id:") {
					if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
						instance[objects.FieldKeyID] = strings.Trim(strings.TrimSpace(parts[1]), `"'`)
					}
				} else if strings.HasPrefix(line, "kind:") {
					if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
						instance[objects.FieldKeyKind] = strings.Trim(strings.TrimSpace(parts[1]), `"'`)
					}
				} else if strings.HasPrefix(line, "schema_version:") {
					if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
						instance[objects.FieldKeySchemaVersion] = strings.Trim(strings.TrimSpace(parts[1]), `"'`)
					}
				}
			}
		}
	}

	// Extract body content manually
	bodyLines := []string{}
	inBody := false
	bodyIndent := 0

	for i := bodyStartIdx; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "body:") {
			inBody = true
			bodyIndent = len(line) - len(strings.TrimLeft(line, " \t"))
			continue
		}

		if inBody {
			// Skip the pipe indicator line itself (literal block scalar marker)
			if trimmed == "|" {
				continue
			}

			// Check if we've hit the next top-level field (same or less indentation)
			if trimmed != emptyValue && !strings.HasPrefix(trimmed, "|") {
				currentIndent := len(line) - len(strings.TrimLeft(line, " \t"))
				if currentIndent <= bodyIndent && strings.Contains(trimmed, ":") {
					// This looks like a new field, stop here
					break
				}
			}

			// Include the line in body content
			bodyLines = append(bodyLines, line)
		}
	}

	// Join body lines and clean up
	bodyContent := strings.Join(bodyLines, "\n")
	bodyContent = strings.TrimRight(bodyContent, "\n\r ")

	instance[objects.FieldKeyBody] = bodyContent

	// Extract kind
	extractedKind := ""
	if k, ok := instance[objects.FieldKeyKind].(string); ok {
		extractedKind = k
	}

	return instance, extractedKind, nil
}

// findObjectFileByID finds the file containing an object with the given ID
func findObjectFileByID(projectRoot, objectID string) (string, error) {
	processDir, err := resolveProcessDirForProject(projectRoot)
	if err != nil {
		processDir = datacell.ProcessPrimaryDir(projectRoot)
	}
	// Search in common object directories
	searchDirs := []string{processDir}

	for _, dir := range searchDirs {
		var foundPath string
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // Continue on error
			}
			if info.IsDir() {
				return nil
			}
			if appledouble.SkipPathInTreeWalk(path) {
				return nil
			}
			if !strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml") {
				return nil
			}

			// Quick check: does filename contain the ID?
			if strings.Contains(filepath.Base(path), objectID) {
				// Read file and check ID field
				data, readErr := os.ReadFile(path)
				if readErr != nil {
					return nil
				}

				// Try to parse (even if corrupted, we might be able to read the ID)
				var obj map[string]any
				if yaml.Unmarshal(data, &obj) == nil {
					if id, ok := obj[objects.FieldKeyID].(string); ok && id == objectID {
						foundPath = path
						return filepath.SkipAll // Found it
					}
				} else {
					// Even if YAML is corrupted, try to find ID in raw content
					content := string(data)
					if strings.Contains(content, "id: "+objectID) || strings.Contains(content, `id: "`+objectID+`"`) {
						foundPath = path
						return filepath.SkipAll
					}
				}
			}
			return nil
		})

		if foundPath != emptyValue {
			return foundPath, nil
		}
		if err != nil {
			return "", err
		}
	}

	return "", nil
}

// copyFileForRepair copies a file from src to dst (used for backup/restore)
func copyFileForRepair(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, paths.FilePerm644) //nolint:gosec // Repaired files - 0600 is acceptable for user-readable files
}
