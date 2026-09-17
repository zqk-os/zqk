package paths

import (
	"path/filepath"
	"testing"
)

func TestSchedulerPIDFilePath(t *testing.T) {
	t.Parallel()
	got := SchedulerPIDFilePath("/proj")
	want := filepath.Join("/proj", ProjectDataDir, SchedulerDir, SchedulerPIDFile)
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
