package bldr_cli_cmd_v1_test

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
)

func TestNewTestDashboardCommandBuilder_ReturnsNonNil(t *testing.T) {
	t.Parallel()
	cmd := bldr_cli_cmd_v1.NewTestDashboardCommandBuilder()
	if cmd == nil {
		t.Fatal("expected non-nil cobra.Command from NewTestDashboardCommandBuilder")
	}
}

func TestNewTestDashboardCommandBuilder_HasName(t *testing.T) {
	t.Parallel()
	cmd := bldr_cli_cmd_v1.NewTestDashboardCommandBuilder()
	if got := cmd.Name(); got != "dashboard" {
		t.Errorf("expected command name %q, got %q", "dashboard", got)
	}
}

func TestNewTestDashboardCommandBuilder_HasShortDescription(t *testing.T) {
	t.Parallel()
	cmd := bldr_cli_cmd_v1.NewTestDashboardCommandBuilder()
	if cmd.Short == "" {
		t.Error("expected non-empty short description on dashboard command")
	}
}

func TestNewTestDashboardCommandBuilder_HasFlags(t *testing.T) {
	t.Parallel()
	cmd := bldr_cli_cmd_v1.NewTestDashboardCommandBuilder()
	if cmd.Flags().Lookup("watch") == nil {
		t.Error("expected 'watch' flag on dashboard command")
	}
	if cmd.Flags().Lookup("all") == nil {
		t.Error("expected 'all' flag on dashboard command")
	}
	if cmd.Flags().Lookup("status") == nil {
		t.Error("expected 'status' flag on dashboard command")
	}
	if cmd.Flags().Lookup("test-case") == nil {
		t.Error("expected 'test-case' flag on dashboard command")
	}
}
