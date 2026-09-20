package cli_test

import (
	"testing"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

func TestFlagBag_FirstErrorWins(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{Use: "t"}
	cmd.Flags().String("ok", "v", "")
	var f clipkg.FlagBag
	if got := f.String(cmd, "ok"); got != "v" {
		t.Fatalf("ok=%q", got)
	}
	_ = f.String(cmd, "missing")
	if f.Err() == nil {
		t.Fatal("expected error for missing flag")
	}
	first := f.Err()
	_ = f.Bool(cmd, "also-missing")
	if f.Err() != first {
		t.Fatalf("error mutated: %v vs %v", f.Err(), first)
	}
}
