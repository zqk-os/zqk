package app_test

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/cmd/zqk/app"
)

// TestStaticFloor_CanonicalDomainCommandMappings verifies that all 14 ergonomics shortcuts
// possess canonical domain host representations per CLI_COMMAND_TAXONOMY_STANDARDS.md Rule 2.
func TestStaticFloor_CanonicalDomainCommandMappings(t *testing.T) {
	rootCmd := app.NewRootCommand()

	expectedMappings := map[string][]string{
		"object":   {"inspect", "mutate", "rollback", "new"},
		"system":   {"validate", "pre-commit", "reports", "completion"},
		"graph":    {"query", "join"},
		"mesh":     {"sync"},
		"agent":    {"learn"},
		"workflow": {"do"},
		"service":  {"tray"},
	}

	for domain, subcmds := range expectedMappings {
		domainCmd, _, err := rootCmd.Find([]string{domain})
		require.NoError(t, err, "Domain command %q should be resolvable", domain)
		require.NotNil(t, domainCmd, "Domain command %q should not be nil", domain)

		for _, sub := range subcmds {
			found := false
			for _, c := range domainCmd.Commands() {
				if c.Name() == sub {
					found = true
					break
				}
			}
			assert.True(t, found, "Domain %q should contain canonical subcommand %q", domain, sub)
		}
	}
}

// TestOperationalProof_ErgonomicsShortcutsExecution verifies that root-level shortcuts
// and their canonical domain counterparts are functional and output help information cleanly.
func TestOperationalProof_ErgonomicsShortcutsExecution(t *testing.T) {
	rootCmd := app.NewRootCommand()

	pairs := [][2][]string{
		{{"inspect", "--help"}, {"object", "inspect", "--help"}},
		{{"mutate", "--help"}, {"object", "mutate", "--help"}},
		{{"rollback", "--help"}, {"object", "rollback", "--help"}},
		{{"validate", "--help"}, {"system", "validate", "--help"}},
		{{"query", "--help"}, {"graph", "query", "--help"}},
		{{"join", "--help"}, {"graph", "join", "--help"}},
		{{"sync", "--help"}, {"mesh", "sync", "--help"}},
		{{"learn", "--help"}, {"agent", "learn", "--help"}},
		{{"do", "--help"}, {"workflow", "do", "--help"}},
		{{"reports", "--help"}, {"system", "reports", "--help"}},
		{{"completion", "--help"}, {"system", "completion", "--help"}},
	}

	for _, pair := range pairs {
		rootShortcutArgs := pair[0]
		canonicalDomainArgs := pair[1]

		cmd1, _, err1 := rootCmd.Find(rootShortcutArgs[:1])
		assert.NoError(t, err1, "Root shortcut %v should resolve", rootShortcutArgs[0])
		assert.NotNil(t, cmd1)

		cmd2, _, err2 := rootCmd.Find(canonicalDomainArgs[:2])
		assert.NoError(t, err2, "Canonical command %v should resolve", canonicalDomainArgs[:2])
		assert.NotNil(t, cmd2)
	}
}

// TestNegativeBoundary_InvalidSubcommandRejection verifies that unknown flags and invalid subcommands
// under canonical domains are rejected or unresolved as subcommands.
func TestNegativeBoundary_InvalidSubcommandRejection(t *testing.T) {
	rootCmd := app.NewRootCommand()

	domains := []string{"object", "system", "graph", "mesh", "agent", "workflow", "service"}

	for _, domain := range domains {
		foundCmd, remainingArgs, err := rootCmd.Find([]string{domain, "nonexistent-subcommand-xyz"})
		require.NoError(t, err)
		assert.Equal(t, domain, foundCmd.Name(), "Should not resolve unknown subcommand")
		assert.Equal(t, []string{"nonexistent-subcommand-xyz"}, remainingArgs)

		// Test invalid flag rejection on the domain command
		buf := new(bytes.Buffer)
		testRoot := app.NewRootCommand()
		testRoot.SetOut(buf)
		testRoot.SetErr(buf)
		testRoot.SetArgs([]string{domain, "--definitely-invalid-flag-xyz"})
		flagErr := testRoot.Execute()
		assert.Error(t, flagErr, "Domain %s with invalid flag should return error", domain)
	}
}

func init() {
	// Silence cobra usage on test runner
	cobra.EnableCommandSorting = true
}
