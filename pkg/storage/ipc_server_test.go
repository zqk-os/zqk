package storage

import (
	"path/filepath"
	"testing"
)

func TestPrivilegedWriterDaemonTypeExists(t *testing.T) {
	t.Parallel()
	_ = PrivilegedWriterDaemon{}
}

func TestPrivilegedWriterAffinity(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	daemon := NewPrivilegedWriterDaemon(nil, tmpDir)

	// 1. Valid matching project root
	args := &IPCWriterArgs{ProjectRoot: tmpDir}
	if err := daemon.checkAffinity(args); err != nil {
		t.Errorf("expected matching project root to pass affinity check, got: %v", err)
	}

	// 2. Relative vs absolute path equivalence via clean
	cleanDir := filepath.Clean(tmpDir)
	argsClean := &IPCWriterArgs{ProjectRoot: cleanDir + "/."}
	if err := daemon.checkAffinity(argsClean); err != nil {
		t.Errorf("expected cleaned project root to match, got: %v", err)
	}

	// 3. Mismatched project root must fail
	otherDir := t.TempDir()
	argsMismatch := &IPCWriterArgs{ProjectRoot: otherDir}
	if err := daemon.checkAffinity(argsMismatch); err == nil {
		t.Errorf("expected affinity mismatch error, got nil")
	}

	// 4. Empty client ProjectRoot must fail when daemon has root configured
	argsEmpty := &IPCWriterArgs{ProjectRoot: ""}
	if err := daemon.checkAffinity(argsEmpty); err == nil {
		t.Errorf("expected error for empty client ProjectRoot, got nil")
	}

	// 5. Nil args must fail
	if err := daemon.checkAffinity(nil); err == nil {
		t.Errorf("expected error for nil args, got nil")
	}

	// 6. Unbound daemon without project root allows any
	unboundDaemon := NewPrivilegedWriterDaemon(nil, "")
	if err := unboundDaemon.checkAffinity(&IPCWriterArgs{ProjectRoot: tmpDir}); err != nil {
		t.Errorf("expected unbound daemon to allow, got: %v", err)
	}
}
