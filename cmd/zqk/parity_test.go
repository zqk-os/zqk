package main

import (
	"github.com/zqk-os/zqk/pkg/execwrap"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	internal "github.com/zqk-os/zqk/pkg/zqkcli"

	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/zqk-os/zqk/cmd/zqk/object"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
)

// TestCommandParity verifies that internal commands have feature parity with object commands
func TestCommandParity(t *testing.T) {
	t.Parallel()
	// Get all subcommands from object command group
	objectCmd := object.NewObjectCmd()
	internalCmd := internal.NewInternalCmd()

	// Expected common commands that should exist in both
	expectedCommands := []string{
		"list",
		"get",
		"create",
		"update",
		"delete",
		"fields",
		"bulk",
	}

	// Verify all expected commands exist in both groups
	for _, cmdName := range expectedCommands {
		t.Run("Command_"+cmdName, func(t *testing.T) {
			// Check object command
			objectSubCmd := findSubCommand(objectCmd, cmdName)
			if objectSubCmd == nil {
				t.Errorf("object command missing subcommand: %s", cmdName)
				return
			}

			// Check internal command
			internalSubCmd := findSubCommand(internalCmd, cmdName)
			if internalSubCmd == nil {
				t.Errorf("internal command missing subcommand: %s", cmdName)
				return
			}

			// Verify both commands have similar structure
			verifyCommandStructure(t, objectSubCmd, internalSubCmd, cmdName)
		})
	}

	// Verify object has count command (internal doesn't need it, but document it)
	t.Run("Command_count", func(t *testing.T) {
		objectCountCmd := findSubCommand(objectCmd, "count")
		if objectCountCmd == nil {
			t.Error("object command missing count subcommand")
		}
		// Note: internal doesn't have count, which is acceptable
		// as count is more of a query operation than a management operation
	})
}

// TestFlagParity verifies that commands with the same name have similar flags
func TestFlagParity(t *testing.T) {
	t.Parallel()
	objectCmd := object.NewObjectCmd()
	internalCmd := internal.NewInternalCmd()

	// Commands that should have flag parity
	commandsToCheck := []string{"list", "get", "create", "update", "delete", "fields"}

	for _, cmdName := range commandsToCheck {
		t.Run("Flags_"+cmdName, func(t *testing.T) {
			objectSubCmd := findSubCommand(objectCmd, cmdName)
			internalSubCmd := findSubCommand(internalCmd, cmdName)

			if objectSubCmd == nil || internalSubCmd == nil {
				t.Skipf("Skipping %s - command not found in both groups", cmdName)
				return
			}

			// Get flags from both commands
			objectFlags := getCommandFlags(objectSubCmd)
			internalFlags := getCommandFlags(internalSubCmd)

			// Verify common flags exist in both
			commonFlags := getCommonFlagsForCommand(cmdName)
			for _, flagName := range commonFlags {
				hasObjectFlag := hasFlag(objectFlags, flagName)
				hasInternalFlag := hasFlag(internalFlags, flagName)

				if hasObjectFlag && !hasInternalFlag {
					t.Errorf("internal %s missing flag: %s (exists in object)", cmdName, flagName)
				}
				// Note: internal may have additional flags (like --built-in, --internal, --all)
				// which is acceptable
			}
		})
	}
}

// TestBulkSubcommandParity verifies that bulk subcommands match between object and internal
//
//nolint:gocyclo // Test function intentionally exercises many parity scenarios
func TestBulkSubcommandParity(t *testing.T) {
	t.Parallel()
	objectCmd := object.NewObjectCmd()
	internalCmd := internal.NewInternalCmd()

	objectBulkCmd := findSubCommand(objectCmd, "bulk")
	internalBulkCmd := findSubCommand(internalCmd, "bulk")

	if objectBulkCmd == nil {
		t.Fatal("object command missing bulk subcommand")
	}
	if internalBulkCmd == nil {
		t.Fatal("internal command missing bulk subcommand")
	}

	expectedBulkSubcommands := []string{"create", "update", "get", "delete"}

	for _, subCmdName := range expectedBulkSubcommands {
		t.Run("Bulk_"+subCmdName, func(t *testing.T) {
			objectSubCmd := findSubCommand(objectBulkCmd, subCmdName)
			internalSubCmd := findSubCommand(internalBulkCmd, subCmdName)

			if objectSubCmd == nil {
				t.Errorf("object bulk missing subcommand: %s", subCmdName)
			}
			if internalSubCmd == nil {
				t.Errorf("internal bulk missing subcommand: %s", subCmdName)
			}

			if objectSubCmd != nil && internalSubCmd != nil {
				// Verify both have similar flags
				objectFlags := getCommandFlags(objectSubCmd)
				internalFlags := getCommandFlags(internalSubCmd)

				// create: file-based in both; update: object uses --filter/--set, internal still uses --file
				if subCmdName == "create" {
					if !hasFlag(objectFlags, "file") || !hasFlag(internalFlags, "file") {
						t.Errorf("bulk %s missing --file flag in either object or internal", subCmdName)
					}
					if !hasFlag(objectFlags, "dry-run") || !hasFlag(internalFlags, "dry-run") {
						t.Errorf("bulk %s missing --dry-run flag in either object or internal", subCmdName)
					}
				}
				if subCmdName == "update" {
					if !hasFlag(objectFlags, "filter") || !hasFlag(objectFlags, "set") {
						t.Errorf("bulk update (object): expected --filter and --set flags")
					}
					if !hasFlag(objectFlags, "dry-run") || !hasFlag(internalFlags, "dry-run") {
						t.Errorf("bulk update missing --dry-run flag in either object or internal")
					}
					if !hasFlag(internalFlags, "file") {
						t.Errorf("bulk update (internal): expected --file (internal bulk update remains file-based)")
					}
				}

				// Both should have --ids and --file flags for get/delete
				if subCmdName == "get" || subCmdName == "delete" {
					if !hasFlag(objectFlags, "ids") || !hasFlag(internalFlags, "ids") {
						t.Errorf("bulk %s missing --ids flag in either object or internal", subCmdName)
					}
					if !hasFlag(objectFlags, "file") || !hasFlag(internalFlags, "file") {
						t.Errorf("bulk %s missing --file flag in either object or internal", subCmdName)
					}
				}

				// Both should have --cascade and --unlink-references for delete
				if subCmdName == "delete" {
					if !hasFlag(objectFlags, "cascade") || !hasFlag(internalFlags, "cascade") {
						t.Errorf("bulk delete missing --cascade flag in either object or internal")
					}
					if !hasFlag(objectFlags, "unlink-references") || !hasFlag(internalFlags, "unlink-references") {
						t.Errorf("bulk delete missing --unlink-references flag in either object or internal")
					}
				}
			}
		})
	}
}

// TestHelpTextParity verifies that help text is consistent between object and internal commands
func TestHelpTextParity(t *testing.T) {
	t.Parallel()
	objectCmd := object.NewObjectCmd()
	internalCmd := internal.NewInternalCmd()

	commandsToCheck := []string{"list", "get", "create", "update", "delete", "fields"}

	for _, cmdName := range commandsToCheck {
		t.Run("Help_"+cmdName, func(t *testing.T) {
			objectSubCmd := findSubCommand(objectCmd, cmdName)
			internalSubCmd := findSubCommand(internalCmd, cmdName)

			if objectSubCmd == nil || internalSubCmd == nil {
				t.Skipf("Skipping %s - command not found", cmdName)
				return
			}

			// Both should have Short and Long descriptions
			if objectSubCmd.Short == emptyValue {
				t.Errorf("object %s missing Short description", cmdName)
			}
			if internalSubCmd.Short == emptyValue {
				t.Errorf("internal %s missing Short description", cmdName)
			}

			if objectSubCmd.Long == emptyValue {
				t.Errorf("object %s missing Long description", cmdName)
			}
			if internalSubCmd.Long == emptyValue {
				t.Errorf("internal %s missing Long description", cmdName)
			}

			// Prefer Examples in Long for consistency (log only; do not fail)
			if !strings.Contains(objectSubCmd.Long, "Examples:") {
				t.Logf("object %s Long description missing Examples section (consider adding)", cmdName)
			}
			if !strings.Contains(internalSubCmd.Long, "Examples:") {
				t.Logf("internal %s Long description missing Examples section (consider adding)", cmdName)
			}
		})
	}
}

// TestFunctionalParity verifies that commands work functionally the same way
func TestFunctionalParity(t *testing.T) {
	// Not t.Parallel(): PrepareIsolatedTempProject uses t.Setenv(ZQK_TEST_ROOT).
	tmpDir, cliBinary := setupCLITestEnvironmentForParity(t)

	// Test that both object and internal can list objects
	t.Run("List_Functional", func(t *testing.T) {
		// Test object list
		cmd := execwrap.Command(cliBinary, "object", "list", "--help")
		cmd.Env = envForIsolatedCLIProject(tmpDir)
		cmd.Dir = tmpDir
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("object list --help failed: %v\n%s", err, output)
		}

		// Test internal list
		cmd = execwrap.Command(cliBinary, "internal", "list", "--help")
		cmd.Env = envForIsolatedCLIProject(tmpDir)
		cmd.Dir = tmpDir
		output, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("internal list --help failed: %v\n%s", err, output)
		}

		// Both should support --format flag
		if !strings.Contains(string(output), "--format") {
			t.Error("internal list missing --format flag")
		}
	})

	// Test that both object and internal kind-based fields work
	t.Run("Fields_Functional", func(t *testing.T) {
		// Test object <kind> fields
		cmd := execwrap.Command(cliBinary, "object", "backlog_item", "fields")
		cmd.Env = envForIsolatedCLIProject(tmpDir)
		cmd.Dir = tmpDir
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("object backlog_item fields failed: %v\n%s", err, output)
		}

		// Test internal <kind> fields
		cmd = execwrap.Command(cliBinary, "internal", "object_spec", "fields")
		cmd.Env = envForIsolatedCLIProject(tmpDir)
		cmd.Dir = tmpDir
		output, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("internal object_spec fields failed: %v\n%s", err, output)
		}

		// Both should produce field output
		outputStr := string(output)
		if !strings.Contains(outputStr, "Fields") && !strings.Contains(outputStr, "Common") {
			t.Error("internal object_spec fields output format unexpected")
		}
	})
}

// Helper functions

func findSubCommand(parent *cobra.Command, name string) *cobra.Command {
	for _, cmd := range parent.Commands() {
		if cmd.Name() == name {
			return cmd
		}
	}
	return nil
}

func getCommandFlags(cmd *cobra.Command) []string {
	flags := []string{}
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		flags = append(flags, flag.Name)
	})
	return flags
}

func hasFlag(flags []string, flagName string) bool {
	for _, flag := range flags {
		if flag == flagName {
			return true
		}
	}
	return false
}

func getCommonFlagsForCommand(cmdName string) []string {
	commonFlags := []string{"format", "output", "verbose", "quiet", "timeout", "columns"}

	switch cmdName {
	case "list":
		return append(commonFlags, "filter", "sort-by", "sort-asc", "offset", "limit", "group-by", "group-limit", "count")
	case "create":
		return append(commonFlags, "file", "data", "dry-run")
	case "update":
		return append(commonFlags, "file", "data", "field", "dry-run")
	case "delete":
		return append(commonFlags, "cascade", "dry-run", "unlink-references")
	case "fields":
		return append(commonFlags, "filterable", "sortable", "groupable", "specialized-only", "list-kinds")
	}

	return commonFlags
}

func verifyCommandStructure(t *testing.T, objectCmd, internalCmd *cobra.Command, cmdName string) {
	// Both should have the same Use pattern (or similar)
	if objectCmd.Use == emptyValue {
		t.Errorf("object %s missing Use field", cmdName)
	}
	if internalCmd.Use == emptyValue {
		t.Errorf("internal %s missing Use field", cmdName)
	}

	// Both should have Short and Long descriptions
	if objectCmd.Short == emptyValue {
		t.Errorf("object %s missing Short description", cmdName)
	}
	if internalCmd.Short == emptyValue {
		t.Errorf("internal %s missing Short description", cmdName)
	}
}

func setupCLITestEnvironmentForParity(t *testing.T) (tmpDir, cliBinary string) {
	t.Helper()
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "internal.functional_parity",
		AppendStagesBeforeStorage: func(root string) []testkit.NamedTestStep {
			return []testkit.NamedTestStep{
				{
					Name: "copy_object_specs_for_parity",
					Fn: func() error {
						specsDir := filepath.Join(root, paths.ProcessInternalObjectSpecsDir)
						if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
							return err
						}
						projectRoot := findProjectRootForParityTest(t)
						sourceSpecsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
						return copySpecFilesForParity(sourceSpecsDir, specsDir)
					},
				},
				{
					Name: "legacy_kind_directories",
					Fn: func() error {
						legacyDirs := []string{"backlog", "goals", "milestones", "workstreams", "priority_plans", "criteria", "requirements"}
						for _, dirName := range legacyDirs {
							kindDir := datacell.CellCASPrimaryDir(root, dirName)
							if err := fileutil.MkdirAll(kindDir, paths.DirPerm755); err != nil {
								return fmt.Errorf("create kind dir %s: %w", kindDir, err)
							}
						}
						return nil
					},
				},
			}
		},
	})
	tmpDir = proj.Root

	// Build the CLI binary
	projectRoot := findProjectRootForParityTest(t)
	cliBinary = filepath.Join(tmpDir, "zqk-admin")
	buildCmd := execwrap.Command("go", "build", "-o", cliBinary, "./cmd/zqk")
	buildCmd.Dir = projectRoot
	// buildCmd.Env = os.Environ() removed to preserve WireExecForIsolatedProject env
	if err := buildCmd.Run(); err != nil {
		if buildErr, ok := err.(*exec.ExitError); ok {
			t.Fatalf("failed to build CLI (dir=%s): %v\n%s", projectRoot, err, string(buildErr.Stderr))
		}
		t.Fatalf("failed to build CLI (dir=%s): %v", projectRoot, err)
	}

	return tmpDir, cliBinary
}

func findProjectRootForParityTest(t *testing.T) string {
	// Start from current directory and walk up to find go.mod
	dir, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}

	for {
		if _, err := fileutil.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find project root (go.mod)")
		}
		dir = parent
	}
}

func copySpecFilesForParity(sourceDir, destDir string) error {
	entries, err := fileutil.ReadDir(sourceDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		srcPath := filepath.Join(sourceDir, e.Name())
		dstPath := filepath.Join(destDir, e.Name())
		if err := copyFile(srcPath, dstPath); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := fileutil.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := fileutil.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
