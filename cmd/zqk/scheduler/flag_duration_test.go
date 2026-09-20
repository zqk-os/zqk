package scheduler

import (
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestFlagDuration_StringAndDuration(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "t"}
	cmd.Flags().String("watch", "0", "")
	if got := flagDuration(cmd, "watch", time.Minute); got != 0 {
		t.Fatalf("string default 0: got %v", got)
	}
	_ = cmd.Flags().Set("watch", "10s")
	if got := flagDuration(cmd, "watch", 0); got != 10*time.Second {
		t.Fatalf("string 10s: got %v", got)
	}

	cmd2 := &cobra.Command{Use: "t2"}
	cmd2.Flags().Duration("stuck", 2*time.Minute, "")
	if got := flagDuration(cmd2, "stuck", 0); got != 2*time.Minute {
		t.Fatalf("duration default: got %v", got)
	}
	_ = cmd2.Flags().Set("stuck", "5m")
	if got := flagDuration(cmd2, "stuck", 0); got != 5*time.Minute {
		t.Fatalf("duration set: got %v", got)
	}
}
