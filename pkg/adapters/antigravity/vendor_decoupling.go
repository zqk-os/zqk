package antigravity

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// DefaultAgentAPIBinarySubpath defines the default relative location of the
// vendor-provided agentapi helper binary under the user's home directory.
var DefaultAgentAPIBinarySubpath = filepath.Join(".gemini", "antigravity-cli", "bin", "agentapi")

// DefaultBrainSubpath defines the default glob pattern under user's home
// directory where Antigravity conversation transcripts are persisted.
var DefaultBrainSubpath = filepath.Join(".gemini", "antigravity-cli", "brain")

// LookPathAgentAPI searches for the agentapi binary in system PATH first,
// and falls back to the canonical vendor installation path in the user home directory.
func LookPathAgentAPI() string {
	if p, err := exec.LookPath("agentapi"); err == nil && strings.TrimSpace(p) != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		home = strings.TrimSpace(os.Getenv("HOME"))
	}
	if home == "" {
		return ""
	}
	cand := filepath.Join(home, DefaultAgentAPIBinarySubpath)
	if fileutil.IsRegularFile(cand) {
		return cand
	}
	return ""
}
