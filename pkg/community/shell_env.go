package community

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// ShellType identifies a supported terminal shell family.
type ShellType string

const (
	ShellZsh   ShellType = "zsh"
	ShellBash  ShellType = "bash"
	ShellFish  ShellType = "fish"
	ShellPosix ShellType = "sh"
)

const (
	markerStart = "# >>> zqk shell environment initialization >>>"
	markerEnd   = "# <<< zqk shell environment initialization <<<"
)

// ShellProfile defines configuration details for a specific shell environment.
type ShellProfile struct {
	Shell      ShellType `json:"shell"`
	ConfigFile string    `json:"config_file"`
	BinDir     string    `json:"bin_dir"`
}

// InjectionResult records the outcome of a PATH injection or removal operation.
type InjectionResult struct {
	Shell             ShellType `json:"shell"`
	TargetFile        string    `json:"target_file"`
	Modified          bool      `json:"modified"`
	BackupFile        string    `json:"backup_file,omitempty"`
	AlreadyConfigured bool      `json:"already_configured"`
}

// DetectShell inspects the SHELL environment string or process name and classifies the shell.
func DetectShell(shellPath string) ShellType {
	base := strings.ToLower(filepath.Base(strings.TrimSpace(shellPath)))
	switch {
	case strings.Contains(base, "zsh"):
		return ShellZsh
	case strings.Contains(base, "bash"):
		return ShellBash
	case strings.Contains(base, "fish"):
		return ShellFish
	default:
		return ShellPosix
	}
}

// GetDefaultConfigFile returns the standard user configuration file path for the shell.
func GetDefaultConfigFile(shell ShellType, homeDir string) string {
	switch shell {
	case ShellZsh:
		return filepath.Join(homeDir, ".zshrc")
	case ShellBash:
		return filepath.Join(homeDir, ".bashrc")
	case ShellFish:
		return filepath.Join(homeDir, ".config", "fish", "config.fish")
	default:
		return filepath.Join(homeDir, ".profile")
	}
}

// FormatShellSnippet generates the shell-specific PATH injection block.
func FormatShellSnippet(shell ShellType, binDir string) string {
	cleanedBin := filepath.Clean(binDir)
	switch shell {
	case ShellFish:
		return fmt.Sprintf("%s\nfish_add_path %s\n%s\n", markerStart, cleanedBin, markerEnd)
	default:
		return fmt.Sprintf("%s\nexport PATH=\"%s:$PATH\"\n%s\n", markerStart, cleanedBin, markerEnd)
	}
}

// InjectPath safely and idempotently appends or updates the zqk PATH snippet in the target profile file.
func InjectPath(profile ShellProfile) (*InjectionResult, error) {
	if strings.TrimSpace(profile.BinDir) == "" {
		return nil, fmt.Errorf("bin directory path cannot be empty")
	}
	if strings.TrimSpace(profile.ConfigFile) == "" {
		return nil, fmt.Errorf("configuration file path cannot be empty")
	}

	result := &InjectionResult{
		Shell:      profile.Shell,
		TargetFile: profile.ConfigFile,
	}

	configDir := filepath.Dir(profile.ConfigFile)
	if err := fileutil.EnsureDir(configDir); err != nil {
		return nil, fmt.Errorf("failed to create config directory %s: %w", configDir, err)
	}

	var existingContent string
	data, err := fileutil.ReadFile(profile.ConfigFile)
	if err == nil {
		existingContent = string(data)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to read config file %s: %w", profile.ConfigFile, err)
	}

	snippet := FormatShellSnippet(profile.Shell, profile.BinDir)

	// Check if already injected and unchanged
	if startIdx, _, blockEnd, ok := findInjectionBlock(existingContent); ok {
		currentBlock := existingContent[startIdx:blockEnd]
		if strings.TrimSpace(currentBlock) == strings.TrimSpace(snippet) {
			result.AlreadyConfigured = true
			result.Modified = false
			return result, nil
		}

		// Replace existing block
		newContent := existingContent[:startIdx] + snippet + existingContent[blockEnd:]
		return writeWithBackup(profile.ConfigFile, existingContent, newContent, result)
	}

	// Not yet present; append snippet
	var newContent string
	if len(existingContent) > 0 && !strings.HasSuffix(existingContent, "\n") {
		newContent = existingContent + "\n\n" + snippet
	} else if len(existingContent) > 0 {
		newContent = existingContent + "\n" + snippet
	} else {
		newContent = snippet
	}

	return writeWithBackup(profile.ConfigFile, existingContent, newContent, result)
}

// RemovePathInjection strips any previously injected zqk environment block from the config file.
func RemovePathInjection(profile ShellProfile) (*InjectionResult, error) {
	if strings.TrimSpace(profile.ConfigFile) == "" {
		return nil, fmt.Errorf("configuration file path cannot be empty")
	}

	result := &InjectionResult{
		Shell:      profile.Shell,
		TargetFile: profile.ConfigFile,
	}

	data, err := fileutil.ReadFile(profile.ConfigFile)
	if err != nil {
		if os.IsNotExist(err) {
			result.Modified = false
			return result, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	content := string(data)
	startIdx, _, blockEnd, ok := findInjectionBlock(content)
	if !ok {
		result.Modified = false
		return result, nil
	}

	newContent := strings.TrimRight(content[:startIdx], "\n")
	remainder := strings.TrimLeft(content[blockEnd:], "\n")
	if len(remainder) > 0 {
		if len(newContent) > 0 {
			newContent = newContent + "\n\n" + remainder
		} else {
			newContent = remainder
		}
	}
	if len(newContent) > 0 && !strings.HasSuffix(newContent, "\n") {
		newContent += "\n"
	}

	return writeWithBackup(profile.ConfigFile, content, newContent, result)
}

// VerifyPathInjected checks if the given configuration file contains an active zqk PATH injection block.
func VerifyPathInjected(profile ShellProfile) (bool, error) {
	data, err := fileutil.ReadFile(profile.ConfigFile)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed reading profile file: %w", err)
	}

	content := string(data)
	if startIdx, endIdx, _, ok := findInjectionBlock(content); ok {
		cleanedBin := filepath.Clean(profile.BinDir)
		block := content[startIdx:endIdx]
		if strings.Contains(block, cleanedBin) {
			return true, nil
		}
	}

	return false, nil
}

func findInjectionBlock(content string) (startIdx, endIdx, blockEnd int, found bool) {
	startIdx = strings.Index(content, markerStart)
	endIdx = strings.Index(content, markerEnd)
	if startIdx != -1 && endIdx != -1 && endIdx > startIdx {
		blockEnd = endIdx + len(markerEnd)
		if blockEnd < len(content) && content[blockEnd] == '\n' {
			blockEnd++
		}
		return startIdx, endIdx, blockEnd, true
	}
	return -1, -1, -1, false
}

func writeWithBackup(targetPath, oldContent, newContent string, result *InjectionResult) (*InjectionResult, error) {
	if len(oldContent) > 0 {
		backupPath := fmt.Sprintf("%s.bak-%d", targetPath, time.Now().UnixNano())
		if err := fileutil.WriteStandardFile(backupPath, []byte(oldContent)); err != nil {
			return nil, fmt.Errorf("failed to write backup file %s: %w", backupPath, err)
		}
		result.BackupFile = backupPath
	}

	if err := fileutil.WriteDurableStandardFile(targetPath, []byte(newContent)); err != nil {
		return nil, fmt.Errorf("failed to atomically update config file: %w", err)
	}

	result.Modified = true
	return result, nil
}
