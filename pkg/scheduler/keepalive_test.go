package scheduler

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/paths"
)

func TestKeepAlive_WriteReadRemove(t *testing.T) {
	t.Parallel()
	testRoot := t.TempDir()

	// Test writing keep-alive file
	err := writeKeepAlive(testRoot)
	if err != nil {
		t.Fatalf("writeKeepAlive() error = %v", err)
	}

	// Verify keep-alive file exists
	keepAlivePath := getKeepAliveFilePath(testRoot)
	if _, err := os.Stat(keepAlivePath); os.IsNotExist(err) {
		t.Error("Keep-alive file should exist after writeKeepAlive()")
	}

	// Test reading keep-alive file
	timestamp, err := readKeepAlive(testRoot)
	if err != nil {
		t.Fatalf("readKeepAlive() error = %v", err)
	}

	// Verify timestamp is recent (within last minute)
	now := time.Now().UTC()
	age := now.Sub(timestamp)
	if age < 0 || age > 1*time.Minute {
		t.Errorf("readKeepAlive() timestamp = %v, expected to be within last minute (age: %v)", timestamp, age)
	}

	// Test removing keep-alive file
	err = removeKeepAlive(testRoot)
	if err != nil {
		t.Fatalf("removeKeepAlive() error = %v", err)
	}

	// Verify keep-alive file is removed
	if _, err := os.Stat(keepAlivePath); !os.IsNotExist(err) {
		t.Error("Keep-alive file should not exist after removeKeepAlive()")
	}
}

func TestKeepAlive_ReadNonExistent(t *testing.T) {
	t.Parallel()
	testRoot := t.TempDir()

	_, err := readKeepAlive(testRoot)
	if err == nil {
		t.Error("readKeepAlive() should return error for non-existent file")
	}
}

func TestIsSchedulerAlive_NotRunning(t *testing.T) {
	t.Parallel()
	testRoot := t.TempDir()

	isAlive, lastKeepAlive, err := IsSchedulerAlive(testRoot)
	if err != nil {
		t.Fatalf("IsSchedulerAlive() error = %v", err)
	}

	if isAlive {
		t.Error("IsSchedulerAlive() should return false when scheduler is not running")
	}

	if !lastKeepAlive.IsZero() {
		t.Error("IsSchedulerAlive() should return zero time when scheduler is not running")
	}
}

func TestIsSchedulerAlive_RunningWithFreshKeepAlive(t *testing.T) {
	t.Parallel()
	testRoot := t.TempDir()

	// Write PID file with current process PID
	err := writePIDFile(testRoot)
	if err != nil {
		t.Fatalf("writePIDFile() error = %v", err)
	}

	// Write fresh keep-alive file
	err = writeKeepAlive(testRoot)
	if err != nil {
		t.Fatalf("writeKeepAlive() error = %v", err)
	}

	// Check if scheduler is alive
	isAlive, lastKeepAlive, err := IsSchedulerAlive(testRoot)
	if err != nil {
		t.Fatalf("IsSchedulerAlive() error = %v", err)
	}

	if !isAlive {
		t.Error("IsSchedulerAlive() should return true when PID file exists, process is running, and keep-alive is fresh")
	}

	if lastKeepAlive.IsZero() {
		t.Error("IsSchedulerAlive() should return non-zero timestamp when scheduler is alive")
	}

	// Verify timestamp is recent
	now := time.Now().UTC()
	age := now.Sub(lastKeepAlive)
	if age < 0 || age > 1*time.Minute {
		t.Errorf("IsSchedulerAlive() lastKeepAlive = %v, expected to be within last minute (age: %v)", lastKeepAlive, age)
	}

	// Cleanup
	_ = removePIDFile(testRoot)
	_ = removeKeepAlive(testRoot)
}

func TestIsSchedulerAlive_StaleKeepAlive(t *testing.T) {
	t.Parallel()
	testRoot := t.TempDir()

	// Write PID file with current process PID
	err := writePIDFile(testRoot)
	if err != nil {
		t.Fatalf("writePIDFile() error = %v", err)
	}

	// Write stale keep-alive file (3 minutes ago)
	keepAlivePath := getKeepAliveFilePath(testRoot)
	schedulerDir := filepath.Dir(keepAlivePath)
	if err := os.MkdirAll(schedulerDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create scheduler directory: %v", err)
	}

	staleTime := time.Now().UTC().Add(-3 * time.Minute)
	staleTimestamp := staleTime.Format(time.RFC3339)
	if err := os.WriteFile(keepAlivePath, []byte(staleTimestamp), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write stale keep-alive file: %v", err)
	}

	// Check if scheduler is alive - should detect stale keep-alive
	isAlive, lastKeepAlive, err := IsSchedulerAlive(testRoot)
	if err != nil {
		t.Fatalf("IsSchedulerAlive() error = %v", err)
	}

	if isAlive {
		t.Error("IsSchedulerAlive() should return false for stale keep-alive (older than DefaultKeepAliveTimeout)")
	}

	// Should still return the stale timestamp
	if lastKeepAlive.IsZero() {
		t.Error("IsSchedulerAlive() should return the stale timestamp even if scheduler is not alive")
	}

	// Verify timestamp matches what we wrote
	if !lastKeepAlive.Truncate(time.Second).Equal(staleTime.Truncate(time.Second)) {
		t.Errorf("IsSchedulerAlive() lastKeepAlive = %v, want %v", lastKeepAlive, staleTime)
	}

	// Cleanup
	_ = removePIDFile(testRoot)
	_ = removeKeepAlive(testRoot)
}

func TestIsSchedulerAlive_StalePIDFile(t *testing.T) {
	t.Parallel()
	testRoot := t.TempDir()

	// Write PID file with non-existent PID
	pidFilePath := getPIDFilePath(testRoot)
	schedulerDir := filepath.Dir(pidFilePath)
	if err := os.MkdirAll(schedulerDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create scheduler directory: %v", err)
	}

	// Use a PID that definitely doesn't exist
	if err := os.WriteFile(pidFilePath, []byte("999999999"), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write PID file: %v", err)
	}

	// Write keep-alive file
	err := writeKeepAlive(testRoot)
	if err != nil {
		t.Fatalf("writeKeepAlive() error = %v", err)
	}

	// Check if scheduler is alive - should detect stale PID file and clean up keep-alive
	isAlive, lastKeepAlive, err := IsSchedulerAlive(testRoot)
	if err != nil {
		t.Fatalf("IsSchedulerAlive() error = %v", err)
	}

	if isAlive {
		t.Error("IsSchedulerAlive() should return false for stale PID file")
	}

	if !lastKeepAlive.IsZero() {
		t.Error("IsSchedulerAlive() should return zero time when PID file is stale")
	}

	// Verify stale keep-alive file was cleaned up
	keepAlivePath := getKeepAliveFilePath(testRoot)
	if _, err := os.Stat(keepAlivePath); !os.IsNotExist(err) {
		t.Error("Stale keep-alive file should be removed by IsSchedulerAlive() when PID file is stale")
	}
}

func TestRemoveKeepAlive_Exported(t *testing.T) {
	t.Parallel()
	testRoot := t.TempDir()

	// Write keep-alive file
	err := writeKeepAlive(testRoot)
	if err != nil {
		t.Fatalf("writeKeepAlive() error = %v", err)
	}

	// Test exported RemoveKeepAlive function
	err = RemoveKeepAlive(testRoot)
	if err != nil {
		t.Fatalf("RemoveKeepAlive() error = %v", err)
	}

	// Verify keep-alive file is removed
	keepAlivePath := getKeepAliveFilePath(testRoot)
	if _, err := os.Stat(keepAlivePath); !os.IsNotExist(err) {
		t.Error("Keep-alive file should not exist after RemoveKeepAlive()")
	}
}

func TestKeepAlive_UpdateInterval(t *testing.T) {
	t.Parallel()
	testRoot := t.TempDir()

	// Write initial keep-alive
	err := writeKeepAlive(testRoot)
	if err != nil {
		t.Fatalf("writeKeepAlive() error = %v", err)
	}

	timestamp1, err := readKeepAlive(testRoot)
	if err != nil {
		t.Fatalf("readKeepAlive() error = %v", err)
	}

	// Wait a bit (simulating DefaultKeepAliveInterval)
	time.Sleep(200 * time.Millisecond)

	// Write updated keep-alive
	err = writeKeepAlive(testRoot)
	if err != nil {
		t.Fatalf("writeKeepAlive() error = %v", err)
	}

	timestamp2, err := readKeepAlive(testRoot)
	if err != nil {
		t.Fatalf("readKeepAlive() error = %v", err)
	}

	// Verify timestamp was updated (or at least file was rewritten)
	// Note: If written in the same second, timestamps might be equal, but file should exist
	if timestamp2.Before(timestamp1) {
		t.Errorf("Keep-alive timestamp should not go backwards: timestamp1 = %v, timestamp2 = %v", timestamp1, timestamp2)
	}
	// File should exist and be readable
	if _, err := readKeepAlive(testRoot); err != nil {
		t.Errorf("Keep-alive file should be readable after update: %v", err)
	}

	// Cleanup
	_ = removeKeepAlive(testRoot)
}
