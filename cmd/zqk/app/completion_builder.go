package app

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
)

const (
	shellBash = "bash"
	shellZsh  = "zsh"
	shellFish = "fish"
)

// ShellConfig describes one supported shell for completion (name, install help line, generator).
type ShellConfig struct {
	Name          string
	InstallLine   string // format string: first %s = command name for invocation, second %s = command name for path (e.g. /etc/bash_completion.d/%s)
	GenCompletion func(root *cobra.Command, w io.Writer) error
}

// CompletionBuilder builds a cobra completion command from a list of shell configs.
type CompletionBuilder struct {
	CommandName string
	Root        *cobra.Command
	Shells      []ShellConfig
}

// NewCompletionBuilder returns a builder for the completion command.
func NewCompletionBuilder(commandName string, root *cobra.Command) *CompletionBuilder {
	return &CompletionBuilder{
		CommandName: commandName,
		Root:        root,
		Shells:      nil,
	}
}

// WithShells sets the list of supported shells and returns the builder.
func (b *CompletionBuilder) WithShells(shells []ShellConfig) *CompletionBuilder {
	b.Shells = shells
	return b
}

// CompletionArgsBuilder generates ValidArgs and shell lookup from shell configs.
type CompletionArgsBuilder struct {
	Shells []ShellConfig
}

// NewCompletionArgsBuilder returns a builder that generates completion args from the given shells.
func NewCompletionArgsBuilder(shells []ShellConfig) *CompletionArgsBuilder {
	return &CompletionArgsBuilder{Shells: shells}
}

// ValidArgs returns the list of shell names for cobra.ValidArgs.
func (b *CompletionArgsBuilder) ValidArgs() []string {
	out := make([]string, len(b.Shells))
	for i, s := range b.Shells {
		out[i] = s.Name
	}
	return out
}

// ShellByName returns a map of shell name -> config for RunE dispatch.
func (b *CompletionArgsBuilder) ShellByName() map[string]ShellConfig {
	m := make(map[string]ShellConfig, len(b.Shells))
	for _, s := range b.Shells {
		m[s.Name] = s
	}
	return m
}

// DefaultShells returns the standard bash, zsh, and fish configs.
func DefaultShells() []ShellConfig {
	return []ShellConfig{
		{
			Name:        shellBash,
			InstallLine: "  Bash:  %s completion bash | sudo tee /etc/bash_completion.d/%s  (or ~/.local/share/bash-completion/completions/)",
			GenCompletion: func(root *cobra.Command, w io.Writer) error {
				return root.GenBashCompletion(w)
			},
		},
		{
			Name:        shellZsh,
			InstallLine: "  Zsh:   %s completion zsh > ~/.zsh/completions/_%s",
			GenCompletion: func(root *cobra.Command, w io.Writer) error {
				return root.GenZshCompletion(w)
			},
		},
		{
			Name:        shellFish,
			InstallLine: "  Fish:  %s completion fish > ~/.config/fish/completions/%s.fish",
			GenCompletion: func(root *cobra.Command, w io.Writer) error {
				return root.GenFishCompletion(w, true)
			},
		},
	}
}

// Build returns the completion cobra command.
func (b *CompletionBuilder) Build() *cobra.Command {
	argsBuilder := NewCompletionArgsBuilder(b.Shells)
	validArgs := argsBuilder.ValidArgs()
	shellByName := argsBuilder.ShellByName()

	use := "completion [" + strings.Join(validArgs, "|") + "]"

	// Preallocate to avoid append reallocations (header + per-shell lines + footer).
	longParts := make([]string, 0, 5+len(b.Shells)+2)
	longParts = append(longParts,
		fmt.Sprintf("Generate a shell completion script for %s.", b.CommandName),
		"",
		"Supported shells: "+strings.Join(validArgs, ", ")+".",
		"",
		"Installation:",
	)
	for _, s := range b.Shells {
		longParts = append(longParts, fmt.Sprintf(s.InstallLine, b.CommandName, b.CommandName))
	}
	longParts = append(longParts, "", "After installing, restart your shell or source your config.")
	long := strings.Join(longParts, "\n")

	cmd := &cobra.Command{
		Use:       use,
		Short:     "Generate shell completion script",
		Long:      long,
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs: validArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			shell := args[0]
			cfg, ok := shellByName[shell]
			if !ok {
				return nil
			}
			err := cfg.GenCompletion(b.Root, cli.CommandOutputWriter(cmd, nil))
			if err != nil {
				return fmt.Errorf("GenCompletion failed: %w", err)
			}
			return nil
		},
	}
	return cmd
}
