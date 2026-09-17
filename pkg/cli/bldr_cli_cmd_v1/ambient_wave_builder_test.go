package bldr_cli_cmd_v1_test

import (
	"testing"

	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
)

func TestNewAmbientWaveCommandBuilder(t *testing.T) {
	cmd := bldr_cli_cmd_v1.NewAmbientWaveCommandBuilder()
	if cmd == nil {
		t.Fatal("expected command to be built, got nil")
	}
	if cmd.Use != "wave" {
		t.Errorf("expected Use to be 'wave', got %q", cmd.Use)
	}
}
