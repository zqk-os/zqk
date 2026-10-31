package mcp

import (
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

// TRACK: BLI-1790155068391099000-c86bd852 / CRIT-1790155068391100000-e5a685ec
func TestEnforceRoleEnforcement_AccountLookupFailClosed(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cfg := &ServerConfig{}
	cfg.MCPServer.Security.EnforceAccountRoles = true
	clientInfo := map[string]any{
		clientInfoAccountID:         "ACC-MISSING-NO-FILE",
		objects.FieldKeyRoles:       []any{"developer"},
		objects.FieldKeyPermissions: []any{"write:*", "read:*"},
	}
	got, err := enforceRoleEnforcement(clientInfo, cfg, tmpDir, nil)
	if err == nil {
		t.Fatalf("expected fail-closed deny on account lookup miss, got clientInfo=%v", got)
	}
	if !strings.Contains(err.Error(), "account") && !strings.Contains(err.Error(), "ACC-MISSING") {
		t.Fatalf("error should name account lookup failure, got: %v", err)
	}
	if got != nil {
		t.Fatalf("fail-closed must not return partial clientInfo, got %v", got)
	}
}
