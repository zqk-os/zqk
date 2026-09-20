package system

import (
	"context"
	"strings"
	"testing"
)

func TestNewRunAutonomyInboxCmd(t *testing.T) {
	t.Parallel()
	cmd := NewRunAutonomyInboxCmd()
	if cmd == nil {
		t.Fatal("NewRunAutonomyInboxCmd() returned nil")
	}

	if !strings.HasPrefix(cmd.Use, "run-autonomy-inbox") {
		t.Errorf("Expected command use to start with 'run-autonomy-inbox', got '%s'", cmd.Use)
	}
}

func TestEvaluatePendingPullRequests(t *testing.T) {
	t.Parallel()
	err := evaluatePendingPullRequests(context.Background())
	if err != nil {
		t.Errorf("evaluatePendingPullRequests failed: %v", err)
	}
}
