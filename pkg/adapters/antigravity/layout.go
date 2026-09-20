package antigravity

import (
	"fmt"
	"path/filepath"
	"strings"
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
