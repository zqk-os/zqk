package testing

import (
	"testing"
)

func TestGetMinTimeoutSecondsForPackage_Defaults(t *testing.T) {
	dir := t.TempDir()

	if got := GetMinTimeoutSecondsForPackage(dir, "cmd/zqk/object"); got != 600 {
		t.Errorf("cmd/zqk/object: got %d, want 600", got)
	}
	if got := GetMinTimeoutSecondsForPackage(dir, "cmd/zqk"); got != 600 {
		t.Errorf("cmd/zqk: got %d, want 600", got)
	}
	if got := GetMinTimeoutSecondsForPackage(dir, "pkg/storage"); got != 600 {
		t.Errorf("pkg/storage: got %d, want 600", got)
	}
	if got := GetMinTimeoutSecondsForPackage(dir, "pkg/other"); got != 0 {
		t.Errorf("pkg/other: got %d, want 0", got)
	}
	if got := GetMinTimeoutSecondsForPackage(dir, "./cmd/zqk/object"); got != 600 {
		t.Errorf("./cmd/zqk/object: got %d, want 600", got)
	}
}
