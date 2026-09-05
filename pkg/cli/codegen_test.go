package cli

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestGenerateCommandBuilderFromYAML(t *testing.T) {
	t.Parallel()
	wd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	testdataPath := filepath.Join(wd, "testdata")
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "command_builders")

	tests := []struct {
		name           string
		specFile       string
		wantUseSnippet string // if set, generated code must contain this NewCommandBuilder call
	}{
		{
			name:     "get command spec",
			specFile: "get_command.yaml",
		},
		{
			name:     "delete command spec (CRUD)",
			specFile: "delete_command.yaml",
		},
		{
			name:           "empty name falls back to command file stem",
			specFile:       "empty_name_command.yaml",
			wantUseSnippet: `NewCommandBuilder("empty-name")`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			specPath := filepath.Join(testdataPath, tt.specFile)

			if err := GenerateCommandBuilderFromYAML(specPath, outputDir); err != nil {
				t.Fatalf("failed to generate command builder: %v", err)
			}

			baseName := filepath.Base(specPath)
			commandName := strings.TrimSuffix(baseName, "_command.yaml")
			commandName = strings.TrimSuffix(commandName, "_command.yml")
			commandName = strings.TrimSuffix(commandName, ".yaml")
			commandName = strings.TrimSuffix(commandName, ".yml")

			outputFile := filepath.Join(outputDir, "bldr_cli_cmd_v1", fmt.Sprintf("%s_command_builder.go", commandName))
			if _, err := fileutil.Stat(outputFile); fileutil.IsNotExist(err) {
				t.Fatalf("output file was not created: %s", outputFile)
			}

			generated, err := fileutil.ReadFile(outputFile)
			if err != nil {
				t.Fatalf("failed to read generated file: %v", err)
			}
			generatedStr := string(generated)

			if !strings.Contains(generatedStr, "package bldr_cli_cmd_v1") {
				t.Error("generated code should contain 'package bldr_cli_cmd_v1'")
			}
			if !strings.Contains(generatedStr, "github.com/lanceman/zqk/pkg/cli") {
				t.Error("generated code should import pkg/cli")
			}
			if !strings.Contains(generatedStr, "func New") {
				t.Error("generated code should contain constructor function")
			}
			if strings.Contains(generatedStr, `NewCommandBuilder("")`) {
				t.Fatalf("codegen emitted empty Use stub in %s", filepath.Base(outputFile))
			}
			if strings.Contains(generatedStr, `NewCRUDCommandBuilder("",`) {
				t.Fatalf("codegen emitted empty CRUD Use stub in %s", filepath.Base(outputFile))
			}
			if tt.wantUseSnippet != "" && !strings.Contains(generatedStr, tt.wantUseSnippet) {
				t.Fatalf("expected %q in generated code, got:\n%s", tt.wantUseSnippet, generatedStr)
			}
		})
	}
}

// criticalCommandBuildersMustHaveUse are leaf builders whose cobra Use comes
// only from generated code (no hand Use overlay). Empty Use here breaks swarm
// CLI surfaces (see [REDACTED-ID] / #1102 object list).
//
// Many other bldr_cli_cmd_v1 files still have NewCommandBuilder("") and are
// intentional: ApplyBuilder overlays Use in cmd/zqk. Do not ban all empties.
//
// TRACK: [REDACTED-ID] — broaden this list when rematerializing
// stub specs; remove when empty-Use stubs are no longer the ApplyBuilder pattern.
var criticalCommandBuildersMustHaveUse = []string{
	"object_list_command_builder.go",
	"object_get_command_builder.go",
	"object_create_command_builder.go",
	"object_update_command_builder.go",
	"object_delete_command_builder.go",
	"object_count_command_builder.go",
	"start_here_command_builder.go",
}

func TestCriticalCommandBuilders_NonEmptyUse(t *testing.T) {
	t.Parallel()
	wd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	buildersDir := filepath.Join(wd, "bldr_cli_cmd_v1")
	if _, err := fileutil.Stat(buildersDir); fileutil.IsNotExist(err) {
		buildersDir = filepath.Join(wd, "command_builders", "bldr_cli_cmd_v1")
	}

	for _, name := range criticalCommandBuildersMustHaveUse {
		path := filepath.Join(buildersDir, name)
		content, err := fileutil.ReadFile(path)
		if err != nil {
			t.Errorf("missing critical builder %s: %v", name, err)
			continue
		}
		s := string(content)
		if strings.Contains(s, `NewCommandBuilder("")`) || strings.Contains(s, `NewCRUDCommandBuilder("",`) {
			t.Errorf("%s has empty Use stub — regenerate from .zqk/cli/specs or restore builder (object list regression class)", name)
		}
	}
}
