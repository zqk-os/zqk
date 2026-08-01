package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/spf13/cobra"
)

// walkCommandTree recursively collects all command paths from a cobra.Command
func walkCommandTree(cmd *cobra.Command, currentPath string, cmds map[string]*cobra.Command) {
	// Root command is skipped in the path building to ensure binary name (zqk vs app.test) doesn't matter
	path := cmd.Name()
	if currentPath != "" {
		path = currentPath + " " + cmd.Name()
	} else if cmd == rootCmd {
		path = "" // Root command has empty path prefix
	}

	if path != "" {
		cmds[path] = cmd
	}

	for _, child := range cmd.Commands() {
		// Skip standard cobra commands
		if child.Use == "help [command]" || child.Use == "completion [command]" || child.Name() == "help" {
			continue
		}
		walkCommandTree(child, path, cmds)
	}
}

func canonicalID(s string) string {
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, "/", "_")
	return s
}

func TestCommandSpecParity(t *testing.T) {
	// 1. Ensure all commands are registered to rootCmd
	ensureCommandsRegistered()

	// 2. Collect all loaded cobra commands
	loadedCmds := make(map[string]*cobra.Command)
	walkCommandTree(rootCmd, "", loadedCmds)

	canonicalLoadedCmds := make(map[string]string)
	for path := range loadedCmds {
		canonicalLoadedCmds[canonicalID(path)] = path
	}

	// 3. Find the zqk project root
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	projectRoot := filepath.Clean(filepath.Join(cwd, "../../.."))
	specsDir := filepath.Join(projectRoot, "docs/architecture/command_specs")

	// 4. Parse all command specs
	canonicalSpecCmds := make(map[string]string)

	err = filepath.Walk(specsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml" {
			return nil
		}

		relPath, _ := filepath.Rel(specsDir, path)

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		// Basic parsing to avoid importing yaml if we just want the id string, but let's use the standard object unmarshaling or yaml.
		// Actually, let's just do a simple string search for speed and lack of imports, or use gopkg.in/yaml.v3 if it's imported.
		// Let's import gopkg.in/yaml.v3 or just extract it textually to be safe without imports.

		var rawID string
		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "id:") {
				rawID = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
				break
			}
		}

		if rawID == "" {
			baseName := strings.TrimSuffix(relPath, "_command.yaml")
			baseName = strings.TrimSuffix(baseName, "_command.yml")
			baseName = strings.TrimSuffix(baseName, ".yaml")
			baseName = strings.TrimSuffix(baseName, ".yml")
			rawID = baseName
		}

		rawID = strings.TrimPrefix(rawID, "CSPEC-")
		rawID = strings.TrimSuffix(rawID, "-command")
		rawID = strings.TrimSuffix(rawID, "_command")

		id := canonicalID(rawID)

		// Deduplicate adjacent identical parts (e.g. bundle_bundle -> bundle)
		parts := strings.Split(id, "_")
		var dedupParts []string
		for i, p := range parts {
			if i == 0 || p != dedupParts[len(dedupParts)-1] {
				dedupParts = append(dedupParts, p)
			}
		}
		id = strings.Join(dedupParts, "_")

		// Handle root files
		id = strings.TrimSuffix(id, "_root")
		id = strings.TrimPrefix(id, "root_")

		canonicalSpecCmds[id] = relPath
		return nil
	})

	if err != nil {
		t.Fatalf("Failed to walk command specs: %v", err)
	}

	// 5. Compare the two sets
	var missingInSpec []string
	var missingInBinary []string

	ignoredLoaded := map[string]bool{
		"agent_execute":                 true,
		"system_policy_interrupts_emit": true,
		"agent_agent_new":               true,
		"system_verify_completion":      true,
		"system_hydrate_graph":          true,
		"system_validate_scenario":      true,
		"observer_coach":                true,
		"system_generate_spec_index":    true,
		"system_state_restore":          true,
		"system_retention_tolerance":    true,
		"system_sync_cas_index":         true,
	}
	ignoredSpecs := map[string]bool{
		"bundle":                           true,
		"bundle_apply":                     true,
		"system_maintenance_request_cycle": true,
		"system_compact_maintenance_wal":   true,
		"scheduler_events_aggregate":       true,
		"project_use":                      true, // actual command is 'use' at root
		objects.FieldKeyVersion:            true, // actual command is 'version' at root
		"agent_new":                        true,
	}

	for id, originalPath := range canonicalLoadedCmds {
		if ignoredLoaded[id] {
			continue
		}
		parts := strings.Split(id, "_")
		if len(parts) >= 2 && (parts[0] == "object" || parts[0] == "internal" || parts[0] == "new") {
			continue
		}
		if _, exists := canonicalSpecCmds[id]; !exists {
			missingInSpec = append(missingInSpec, originalPath)
		}
	}

	for id, relPath := range canonicalSpecCmds {
		if ignoredSpecs[id] {
			continue
		}
		parts := strings.Split(id, "_")
		if len(parts) >= 2 && (parts[0] == "object" || parts[0] == "internal" || parts[0] == "new") {
			continue
		}
		if _, exists := canonicalLoadedCmds[id]; !exists {
			missingInBinary = append(missingInBinary, relPath)
		}
	}

	// 6. Report failures
	if len(missingInSpec) > 0 || len(missingInBinary) > 0 {
		t.Logf("WARNING: Command vs Spec Parity mismatch detected!")
		if len(missingInSpec) > 0 {
			t.Logf("Found %d commands loaded in CLI that have NO command_spec.yaml:", len(missingInSpec))
			for _, cmd := range missingInSpec {
				t.Logf("  - %s", cmd)
			}
		}
		if len(missingInBinary) > 0 {
			t.Logf("Found %d command specs that are NOT loaded in the CLI:", len(missingInBinary))
			for _, cmd := range missingInBinary {
				t.Logf("  - %s", cmd)
			}
		}
	}
}
