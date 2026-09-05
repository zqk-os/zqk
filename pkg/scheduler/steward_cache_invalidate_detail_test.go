package scheduler

import (
	"strings"
	"testing"
)

func TestStewardCacheInvalidateDetail_jobIDPrefixAndCap(t *testing.T) {
	t.Parallel()
	got := stewardCacheInvalidateDetail("SCH-abc", "short reason")
	if want := "job_id=SCH-abc|short reason"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	longReason := strings.Repeat("x", stewardInvalidateReasonMax)
	got = stewardCacheInvalidateDetail("SCH-xyz", longReason)
	if len(got) != stewardInvalidateReasonMax {
		t.Fatalf("len=%d want %d", len(got), stewardInvalidateReasonMax)
	}
	const prefix = "job_id=SCH-xyz|"
	if len(got) < len(prefix) || got[:len(prefix)] != prefix {
		t.Fatalf("want prefix %q on %q", prefix, got)
	}
}
