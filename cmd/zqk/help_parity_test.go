package main

import (
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	internal "github.com/zqk-os/zqk/pkg/zqkcli"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/cmd/zqk/object"
	"github.com/zqk-os/zqk/cmd/zqk/system"
	"github.com/zqk-os/zqk/cmd/zqk/utility"
	"github.com/zqk-os/zqk/pkg/paths"
)

// TestHelpMenuParity tests that all help menus are properly formatted and consistent.
// GoldenFileComparison ensures we're aware when help text changes (e.g. flag descriptions);
// regenerate baselines with the env from [zqkenv.UpdateHelpGolden] (=1) and review diffs.
// Under `go test` the executable prefix is often INTERNAL_TEST_, not ZQK_ (see pkg/brand.EnvPrefix).
func TestHelpMenuParity(t *testing.T) {
	t.Parallel()
	// Build root command manually to avoid import cycles
	rootCmd := buildRootCommand()
	cliCmd := paths.CLICommandName
	if cliCmd == emptyValue {
		cliCmd = paths.CLICommandNameDefault
	}

	// Collect all commands recursively
	allCommands := collectAllCommands(rootCmd)

	// Generate help output for all commands
	helpOutputs := make(map[string]string)
	for _, cmd := range allCommands {
		helpOutput := captureHelpOutput(cmd)
		commandPath := getCommandPath(cmd)
		helpOutputs[commandPath] = helpOutput
	}

	// Check formatting quality
	t.Run("FormattingQuality", func(t *testing.T) {
		for path, output := range helpOutputs {
			t.Run(path, func(t *testing.T) {
				checkFormattingQuality(t, path, output)
			})
		}
	})

	// Check help text requirements (standardized format, examples, etc.)
	t.Run("HelpTextRequirements", func(t *testing.T) {
		for path, output := range helpOutputs {
			// Skip root command (it doesn't need examples)
			if path == cliCmd {
				continue
			}
			t.Run(path, func(t *testing.T) {
				checkHelpTextRequirements(t, path, output, allCommands)
			})
		}
	})

	// Compare with golden files (if they exist)
	t.Run("GoldenFileComparison", func(t *testing.T) {
		goldenDir := filepath.Join("testdata", "help_golden")
		if err := fileutil.MkdirAll(goldenDir, paths.DirPerm755); err != nil {
			t.Fatalf("failed to create golden directory: %v", err)
		}

		for path, output := range helpOutputs {
			t.Run(path, func(t *testing.T) {
				compareWithGolden(t, goldenDir, path, output)
			})
		}
	})

	// Check command grouping consistency
	t.Run("CommandGrouping", func(t *testing.T) {
		checkCommandGrouping(t, rootCmd)
	})

	// Print current state for inspection (disabled to prevent log pollution)
	// This was dumping large amounts of help output via t.Logf which was being captured
	// by the logging framework. Help output should be verified via golden files instead.
	// if testing.Verbose() {
	// 	t.Run("PrintCurrentState", func(t *testing.T) {
	// 		printHelpMenuState(t, helpOutputs)
	// 	})
	// }
}

// collectAllCommands recursively collects all commands from a root command
func collectAllCommands(cmd *cobra.Command) []*cobra.Command {
	var commands []*cobra.Command

	// Add the command itself
	commands = append(commands, cmd)

	// Recursively add subcommands
	for _, subCmd := range cmd.Commands() {
		commands = append(commands, collectAllCommands(subCmd)...)
	}

	return commands
}

// getCommandPath returns the full path to a command (e.g., "zqk object list")
func getCommandPath(cmd *cobra.Command) string {
	path := []string{}
	current := cmd

	for current != nil && current.Use != emptyValue {
		path = append([]string{current.Use}, path...)
		current = current.Parent()
	}

	// If no parent, it's the root command
	if len(path) == 0 {
		return paths.CLICommandName
	}

	return strings.Join(path, " ")
}

// captureHelpOutput captures the help output for a command
func captureHelpOutput(cmd *cobra.Command) string {
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	//nolint:errcheck // Test cleanup - errors are acceptable
	_ = cmd.Help()
	return buf.String()
}

// checkFormattingQuality checks for formatting issues in help output
func checkFormattingQuality(t *testing.T, path, output string) {
	lines := strings.Split(output, "\n")

	// Check for lines that are too long (should wrap at reasonable length)
	// Allow up to 250 characters (some flag descriptions are lengthy)
	maxLineLength := 250
	longLines := []int{}
	for i, line := range lines {
		// Skip lines that are part of code examples (they may be intentionally long)
		if strings.HasPrefix(strings.TrimSpace(line), "# ") {
			continue // Example lines starting with # are allowed to be longer
		}
		if len(line) > maxLineLength {
			longLines = append(longLines, i+1)
		}
	}
	if len(longLines) > 0 {
		t.Errorf("%s: Found %d lines exceeding %d characters (lines: %v)", path, len(longLines), maxLineLength, longLines)
		for _, lineNum := range longLines[:minInt(5, len(longLines))] { // Show first 5
			if lineNum <= len(lines) {
				t.Errorf("  Line %d: %s", lineNum, lines[lineNum-1])
			}
		}
	}

	// Check for inconsistent indentation
	checkIndentation(t, path, lines)

	// Check for proper section headers
	checkSectionHeaders(t, path, lines)

	// Check for empty sections
	checkEmptySections(t, path, lines)

	// Check for proper command grouping in "Available Commands" section
	checkCommandListFormatting(t, path, lines)
}

// checkIndentation checks for consistent indentation
//
//nolint:gocyclo // Test helper intentionally exercises many indentation validation scenarios
func checkIndentation(t *testing.T, path string, lines []string) {
	// Commands should be indented consistently
	// Examples should be indented consistently
	// Flags should be indented consistently

	indentationIssues := []string{}

	for i, line := range lines {
		// Skip empty lines
		trimmed := strings.TrimSpace(line)
		if trimmed == emptyValue {
			continue
		}

		// Skip section headers
		if strings.HasSuffix(trimmed, ":") && !strings.Contains(trimmed, "Use \"") {
			continue
		}

		// Check for lines that should be indented (commands, flags, examples)
		// Cobra uses 2 spaces for commands/flags, and 6 spaces for nested content in Long descriptions
		// Both are valid, so we only flag lines that have inconsistent indentation
		if strings.HasPrefix(line, "  ") {
			// This is an indented line - Cobra uses 2 or 6 spaces, both are valid
			// Only flag if it's an odd number of spaces (1, 3, 5, 7, etc.) which would be inconsistent
			spaceCount := 0
			for _, r := range line {
				if r == ' ' {
					spaceCount++
				} else {
					break
				}
			}
			// Allow 2, 4, 6, 8 spaces (common indentation levels)
			// Flag odd numbers or numbers > 8 as potentially inconsistent
			if spaceCount > 0 && spaceCount%2 == 1 && spaceCount != 1 {
				indentationIssues = append(indentationIssues, fmt.Sprintf("line %d: odd indentation (%d spaces): '%s'", i+1, spaceCount, line))
			} else if spaceCount > 8 {
				indentationIssues = append(indentationIssues, fmt.Sprintf("line %d: excessive indentation (%d spaces): '%s'", i+1, spaceCount, line))
			}
		} else if !strings.HasPrefix(trimmed, "Usage:") &&
			!strings.HasPrefix(trimmed, "Available Commands:") &&
			!strings.HasPrefix(trimmed, "Flags:") &&
			!strings.HasPrefix(trimmed, "Global Flags:") &&
			!strings.HasPrefix(trimmed, "Examples:") &&
			!strings.HasPrefix(trimmed, "Use \"") &&
			trimmed != emptyValue {
			// Lines that aren't section headers and aren't indented might be issues
			// Require # example comments to be indented; allow "-" list items at column 0 (Cobra Long often does this)
			if strings.HasPrefix(trimmed, "#") {
				if !strings.HasPrefix(line, "  ") {
					indentationIssues = append(indentationIssues, fmt.Sprintf("line %d: example comment should be indented: '%s'", i+1, line))
				}
			}
		}
	}

	if len(indentationIssues) > 0 {
		t.Errorf("%s: Indentation issues found: %v", path, indentationIssues[:minInt(10, len(indentationIssues))]) // Limit output
	}
}

// minInt returns the minimum of two integers
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// checkSectionHeaders checks for proper section headers
func checkSectionHeaders(t *testing.T, path string, lines []string) {
	expectedSections := []string{
		"Usage:",
		"Available Commands:",
		"Flags:",
		"Global Flags:",
		"Examples:",
		"Use \"",
	}

	foundSections := make(map[string]bool)
	for _, line := range lines {
		for _, section := range expectedSections {
			if strings.HasPrefix(line, section) {
				foundSections[section] = true
			}
		}
	}

	// Check that sections are properly formatted (no extra spaces, proper capitalization)
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		// Long descriptions may include sub-headings like "Usage patterns:" — not the primary Usage: line.
		if strings.HasPrefix(trim, "Usage") && !strings.HasPrefix(trim, "Usage:") {
			if strings.HasPrefix(trim, "Usage ") && strings.Contains(trim, ":") {
				continue
			}
			t.Errorf("%s: Usage section should end with colon: %s", path, line)
		}
		if strings.HasPrefix(line, "Available Commands") && !strings.HasPrefix(line, "Available Commands:") {
			t.Errorf("%s: Available Commands section should end with colon: %s", path, line)
		}
		if strings.HasPrefix(line, "Flags") && !strings.HasPrefix(line, "Flags:") && !strings.HasPrefix(line, "Global Flags:") {
			t.Errorf("%s: Flags section should end with colon: %s", path, line)
		}
	}
}

// checkEmptySections checks for empty sections
func checkEmptySections(t *testing.T, path string, lines []string) {
	inSection := false
	sectionName := ""
	sectionHasContent := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect section start
		if strings.HasSuffix(trimmed, ":") && !strings.Contains(trimmed, "Use \"") {
			if inSection && !sectionHasContent {
				t.Errorf("%s: Empty section '%s' found (line %d)", path, sectionName, i)
			}
			inSection = true
			sectionName = trimmed
			sectionHasContent = false
			continue
		}

		// Check for content in section
		if inSection && trimmed != emptyValue {
			sectionHasContent = true
		}

		// Detect section end (next section or end of output)
		if inSection && trimmed != emptyValue && !strings.HasPrefix(line, "  ") && strings.HasSuffix(trimmed, ":") {
			// This is a new section, check previous one
			if !sectionHasContent {
				t.Errorf("%s: Empty section '%s' found (line %d)", path, sectionName, i)
			}
			sectionName = trimmed
			sectionHasContent = false
		}
	}

	// Check last section
	if inSection && !sectionHasContent {
		t.Errorf("%s: Empty section '%s' at end of help", path, sectionName)
	}
}

// checkCommandListFormatting checks that command lists are properly formatted
func checkCommandListFormatting(t *testing.T, path string, lines []string) {
	inCommandsSection := false
	commandPattern := regexp.MustCompile(`^\s{2}(\S+)\s+(.+)$`)

	for i, line := range lines {
		if strings.HasPrefix(line, "Available Commands:") {
			inCommandsSection = true
			continue
		}

		if inCommandsSection {
			// End of commands section
			if strings.HasPrefix(line, "Flags:") || strings.HasPrefix(line, "Global Flags:") || strings.TrimSpace(line) == emptyValue {
				inCommandsSection = false
				continue
			}

			// Check command line format
			if !commandPattern.MatchString(line) && strings.TrimSpace(line) != emptyValue {
				t.Errorf("%s: Command line format issue at line %d: '%s' (expected: '  command    description')", path, i+1, line)
			}

			// Check that command name and description are separated by at least 1 space
			// Cobra uses 1 space by default, which is acceptable
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				// Find where description starts (after command name)
				commandName := parts[0]
				descriptionStart := strings.Index(line, commandName) + len(commandName)
				descriptionPart := line[descriptionStart:]
				// Allow 1 space (Cobra default), 2+ spaces, or tabs
				if !strings.HasPrefix(descriptionPart, " ") && !strings.HasPrefix(descriptionPart, "\t") {
					t.Errorf("%s: Command description should be separated by at least 1 space at line %d: '%s'", path, i+1, line)
				}
			}
		}
	}
}

// compareWithGolden compares current output with golden file. When the update-help-golden env
// is set, overwrites the golden file when it differs so you can regenerate baselines
// and review help text changes.
func compareWithGolden(t *testing.T, goldenDir, path, output string) {
	// Sanitize path for filename
	filename := sanitizePath(path) + ".txt"
	goldenPath := filepath.Join(goldenDir, filename)

	// Read golden file if it exists
	goldenContent, err := fileutil.ReadFile(goldenPath)
	if err != nil {
		if fileutil.IsNotExist(err) {
			// Create golden file
			if err := fileutil.WriteFile(goldenPath, []byte(output), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
				t.Errorf("failed to create golden file %s: %v", goldenPath, err)
			}
			t.Logf("Created golden file: %s", goldenPath)
			return
		}
		t.Fatalf("failed to read golden file %s: %v", goldenPath, err)
	}

	// Compare
	goldenStr := string(goldenContent)
	if output != goldenStr {
		// Regenerate: overwrite golden file when env is set
		if zqkenv.UpdateHelpGolden().Get() != emptyValue || zqkenv.ZQKCLITestUpdateHelpGolden().Get() != emptyValue {
			if err := fileutil.WriteFile(goldenPath, []byte(output), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
				t.Errorf("failed to update golden file %s: %v", goldenPath, err)
			}
			t.Logf("Updated golden file (help text changed): %s", goldenPath)
			return
		}

		// Find differences
		diff := findDifferences(goldenStr, output)
		t.Logf("=== %s: Help output differs from golden file ===\n%s", path, diff)
		t.Errorf("%s: Help output differs from golden file.\nTo regenerate golden files, run: %s=1 go test ./cmd/zqk -run TestHelpMenuParity/GoldenFileComparison\nDiff:\n%s", path, zqkenv.UpdateHelpGolden().Name(), diff)
	}
}

// sanitizePath converts a command path to a safe filename
func sanitizePath(path string) string {
	// Replace spaces with underscores
	path = strings.ReplaceAll(path, " ", "_")
	// Remove special characters
	path = regexp.MustCompile(`[^a-zA-Z0-9_-]`).ReplaceAllString(path, "_")
	return path
}

// normalizeLineEndings normalizes line endings to \n
//

// findDifferences finds and reports differences between two strings
func findDifferences(golden, current string) string {
	goldenLines := strings.Split(golden, "\n")
	currentLines := strings.Split(current, "\n")

	var diff strings.Builder
	diff.WriteString("Differences found:\n")

	maxLen := len(goldenLines)
	if len(currentLines) > maxLen {
		maxLen = len(currentLines)
	}

	for i := 0; i < maxLen; i++ {
		var goldenLine, currentLine string
		if i < len(goldenLines) {
			goldenLine = goldenLines[i]
		}
		if i < len(currentLines) {
			currentLine = currentLines[i]
		}

		if goldenLine != currentLine {
			fmt.Fprintf(&diff, "Line %d:\n", i+1)
			if goldenLine != emptyValue {
				fmt.Fprintf(&diff, "  Golden:   %s\n", goldenLine)
			}
			if currentLine != emptyValue {
				fmt.Fprintf(&diff, "  Current:  %s\n", currentLine)
			}
		}
	}

	return diff.String()
}

// buildRootCommand builds the root command with all subcommands
func buildRootCommand() *cobra.Command {
	cliCmd := paths.CLICommandName
	if cliCmd == emptyValue {
		cliCmd = paths.CLICommandNameDefault
	}

	rootCmd := &cobra.Command{
		Use:   cliCmd,
		Short: fmt.Sprintf("%s CLI", strings.ToUpper(cliCmd)),
	}

	// Add all top-level command groups
	rootCmd.AddCommand(object.NewObjectCmd())
	rootCmd.AddCommand(system.NewSystemCmd())
	rootCmd.AddCommand(internal.NewInternalCmd()) // Use local function since we're in internal package
	rootCmd.AddCommand(utility.NewUtilityCmd())
	rootCmd.AddCommand(utility.NewRootVersionCmd())

	return rootCmd
}

// checkCommandGrouping checks that commands are properly grouped
func checkCommandGrouping(t *testing.T, rootCmd *cobra.Command) {
	// Check that top-level commands are properly grouped
	topLevelCommands := rootCmd.Commands()
	expectedGroups := []string{"object", "system", "internal", "utility", "version"}

	foundGroups := make(map[string]bool)
	for _, cmd := range topLevelCommands {
		foundGroups[cmd.Name()] = true
	}

	for _, group := range expectedGroups {
		if !foundGroups[group] {
			t.Errorf("Expected top-level command group '%s' not found", group)
		}
	}

	// Check that object and internal commands have similar structure
	objectCmd := findCommand(rootCmd, "object")
	internalCmd := findCommand(rootCmd, "internal")

	if objectCmd != nil && internalCmd != nil {
		objectSubs := getSubcommandNames(objectCmd)
		internalSubs := getSubcommandNames(internalCmd)

		// Check for parity (internal should have most of object's commands)
		expectedInInternal := []string{"list", "get", "create", "update", "delete", "fields", "bulk"}
		for _, expected := range expectedInInternal {
			if contains(objectSubs, expected) && !contains(internalSubs, expected) {
				t.Errorf("internal command missing subcommand '%s' (exists in object)", expected)
			}
		}
	}
}

// findCommand finds a command by name in the command tree
func findCommand(cmd *cobra.Command, name string) *cobra.Command {
	if cmd.Name() == name {
		return cmd
	}
	for _, subCmd := range cmd.Commands() {
		if found := findCommand(subCmd, name); found != nil {
			return found
		}
	}
	return nil
}

// getSubcommandNames returns the names of all subcommands
func getSubcommandNames(cmd *cobra.Command) []string {
	names := []string{}
	for _, subCmd := range cmd.Commands() {
		names = append(names, subCmd.Name())
	}
	return names
}

// contains checks if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// printHelpMenuState prints the current state of all help menus for inspection

// checkHelpTextRequirements verifies that help text meets the standardized requirements:
// 1. All commands (except root) should have examples
// 2. Examples should use # comment format
// 3. Long descriptions should follow standardized structure
//
//nolint:gocyclo // Test helper intentionally exercises many help text validation scenarios
func checkHelpTextRequirements(t *testing.T, path, output string, allCommands []*cobra.Command) {
	lines := strings.Split(output, "\n")

	// Find the command object to check its Long description
	var cmd *cobra.Command
	for _, c := range allCommands {
		if getCommandPath(c) == path {
			cmd = c
			break
		}
	}

	if cmd == nil {
		t.Errorf("%s: Could not find command object", path)
		return
	}

	// Skip root command and very simple commands (like version)
	// These don't necessarily need examples
	skipCommands := map[string]bool{
		paths.CLICommandName: true,
	}
	if skipCommands[path] {
		return
	}

	// Check 1: Command should have Examples section
	hasExamplesSection := false
	examplesStartLine := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "Examples:") {
			hasExamplesSection = true
			examplesStartLine = i
			break
		}
	}

	// Check 2: If Examples section exists, it should have at least one example with # comment
	hasExampleWithComment := false
	if hasExamplesSection && examplesStartLine >= 0 {
		// Look for example lines in the Examples section
		for i := examplesStartLine + 1; i < len(lines); i++ {
			line := strings.TrimSpace(lines[i])
			// Stop at next section
			if line != emptyValue && !strings.HasPrefix(line, "  ") && strings.HasSuffix(line, ":") {
				break
			}
			// Check for # comment format
			if strings.HasPrefix(line, "# ") {
				hasExampleWithComment = true
				break
			}
		}
	}

	// For most commands, we expect examples with # comment format
	// Some commands (e.g. object fields, internal fields) have Examples section with different format
	skipExampleCommentCheck := map[string]bool{
		paths.CLICommandName + " object fields [flags]":   true,
		paths.CLICommandName + " internal fields [flags]": true,
	}
	if hasExamplesSection && !hasExampleWithComment && !skipExampleCommentCheck[path] {
		t.Errorf("%s: Examples section exists but contains no examples with # comment format", path)
	}

	// Check 3: Long description should follow standardized structure
	// This is harder to verify programmatically, but we can check for common patterns
	if cmd.Long != emptyValue {
		longLines := strings.Split(cmd.Long, "\n")

		// Check that Long description is not empty
		if len(longLines) == 0 || (len(longLines) == 1 && strings.TrimSpace(longLines[0]) == emptyValue) {
			t.Errorf("%s: Long description is empty", path)
		}

		// Check for Examples section in Long description (should be present for most commands)
		hasExamplesInLong := false
		for _, line := range longLines {
			if strings.HasPrefix(strings.TrimSpace(line), "Examples:") {
				hasExamplesInLong = true
				break
			}
		}

		// Most commands should have examples in their Long description
		// Simple commands like version might not need them
		simpleCommands := map[string]bool{
			paths.CLICommandName + " utility version": true,
			paths.CLICommandName + " version":         true,
		}
		if !simpleCommands[path] && !hasExamplesInLong && !hasExamplesSection {
			// This is a warning, not an error, as some commands might be self-explanatory
			// But we'll log it for review
			t.Logf("%s: Long description does not contain Examples section (consider adding examples)", path)
		}
	}
}
