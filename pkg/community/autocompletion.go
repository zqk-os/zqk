package community

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

// SupportedShells returns the list of shells supported for autocompletion.
var SupportedShells = []string{"bash", "zsh", "fish", "powershell"}

// CompletionConfig holds configuration options for generating autocompletion scripts.
type CompletionConfig struct {
	Shell          string
	IncludeDesc    bool
	NoDescriptions bool
}

// IsShellSupported checks if the provided shell name is supported.
func IsShellSupported(shell string) bool {
	s := strings.ToLower(strings.TrimSpace(shell))
	for _, supported := range SupportedShells {
		if s == supported {
			return true
		}
	}
	return false
}

// GenerateCompletion writes completion script for the specified shell to the provided writer.
func GenerateCompletion(cmd *cobra.Command, cfg CompletionConfig, w io.Writer) error {
	if cmd == nil {
		return fmt.Errorf("root command cannot be nil")
	}
	if w == nil {
		return fmt.Errorf("writer cannot be nil")
	}

	shell := strings.ToLower(strings.TrimSpace(cfg.Shell))
	if shell == "" {
		return fmt.Errorf("shell name must not be empty")
	}

	switch shell {
	case "bash":
		return cmd.GenBashCompletionV2(w, !cfg.NoDescriptions)
	case "zsh":
		if cfg.NoDescriptions {
			return cmd.GenZshCompletionNoDesc(w)
		}
		return cmd.GenZshCompletion(w)
	case "fish":
		return cmd.GenFishCompletion(w, !cfg.NoDescriptions)
	case "powershell":
		if cfg.NoDescriptions {
			return cmd.GenPowerShellCompletion(w)
		}
		return cmd.GenPowerShellCompletionWithDesc(w)
	default:
		return fmt.Errorf("unsupported shell %q; supported shells are: %s", shell, strings.Join(SupportedShells, ", "))
	}
}

// ShellInstallInstructions returns instructions for installing shell completion scripts for a given binary name.
func ShellInstallInstructions(binaryName, shell string) (string, error) {
	if binaryName == "" {
		binaryName = "zqk"
	}
	shell = strings.ToLower(strings.TrimSpace(shell))
	switch shell {
	case "bash":
		return fmt.Sprintf("# Bash installation:\n# Add to ~/.bashrc:\nsource <(%s completion bash)", binaryName), nil
	case "zsh":
		return fmt.Sprintf("# Zsh installation:\n# Add to ~/.zshrc:\nsource <(%s completion zsh)", binaryName), nil
	case "fish":
		return fmt.Sprintf("# Fish installation:\n# Run:\n%s completion fish | source", binaryName), nil
	case "powershell":
		return fmt.Sprintf("# PowerShell installation:\n# Add to $PROFILE:\n%s completion powershell | Out-String | Invoke-Expression", binaryName), nil
	default:
		return "", fmt.Errorf("unsupported shell %q for install instructions", shell)
	}
}
