package antigravity

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/zqkenv"
)

const (
	// VendorID is the stable adapter name returned by InboxAdapter.Vendor.
	VendorID = "antigravity"

	// systemGeneratedDir is Antigravity CLI's per-conversation generated-state folder.
	systemGeneratedDir = ".system_generated"
	messagesDir        = "messages"
	undeliveredDir     = "undelivered"
)

// ConversationRootFromTranscript walks ancestors of an Antigravity transcript
// until it finds a ".system_generated" directory, then returns that directory's
// parent (the conversation / brain folder).
func ConversationRootFromTranscript(transcriptPath string) (string, error) {
	p := filepath.Clean(strings.TrimSpace(transcriptPath))
	if p == "" || p == "." {
		return "", fmt.Errorf("antigravity: empty transcript path")
	}
	for {
		dir := filepath.Dir(p)
		if dir == p {
			return "", fmt.Errorf("antigravity: transcript is not under %s: %s", systemGeneratedDir, transcriptPath)
		}
		if filepath.Base(dir) == systemGeneratedDir {
			return filepath.Dir(dir), nil
		}
		p = dir
	}
}

// IsBrainTranscript reports whether path sits under an Antigravity
// ".system_generated" conversation tree.
func IsBrainTranscript(path string) bool {
	_, err := ConversationRootFromTranscript(path)
	return err == nil
}

// ConversationID is the Antigravity conversation folder name (opaque vendor id).
func ConversationID(conversationRoot string) string {
	base := filepath.Base(conversationRoot)
	if base == "" || base == "." || base == string(filepath.Separator) {
		return ""
	}
	return base
}

func undeliveredInboxDir(conversationRoot string) string {
	return filepath.Join(conversationRoot, systemGeneratedDir, messagesDir, undeliveredDir)
}

// ResolveActiveTranscript returns the path to the active Antigravity transcript.
// It checks the AG_TRANSCRIPT_PATH environment variable first, and if unset,
// searches the default Antigravity brain directory for the most recently modified transcript.
func ResolveActiveTranscript() string {
	if p := zqkenv.AGTranscriptPath().Get(); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}

	pattern := filepath.Join(home, ".gemini", "antigravity-cli", "brain", "*", systemGeneratedDir, "logs", "transcript.jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return ""
	}

	var newestPath string
	var newestTime time.Time
	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil {
			continue
		}
		if newestPath == "" || info.ModTime().After(newestTime) {
			newestPath = m
			newestTime = info.ModTime()
		}
	}
	return newestPath
}
