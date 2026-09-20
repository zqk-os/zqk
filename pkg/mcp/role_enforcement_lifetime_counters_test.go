package mcp

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestRoleEnforcement_LifetimeCounters(t *testing.T) {
	t.Parallel()

	enfInit, ovInit, _ := GetRoleEnforcementStats()

	config := &ServerConfig{}
	config.MCPServer.Security.EnforcedRole = "admin"

	clientInfo := map[string]any{
		objects.FieldKeyClientID: "test-client",
	}

	out, err := enforceRoleEnforcement(clientInfo, config, "", nil)
	if err != nil {
		t.Fatalf("enforceRoleEnforcement failed: %v", err)
	}

	roles, ok := out[objects.FieldKeyRoles].([]any)
	if !ok || len(roles) != 1 || roles[0] != "admin" {
		t.Fatalf("expected enforced role 'admin', got %v", out[objects.FieldKeyRoles])
	}

	enfAfter, ovAfter, errAfter := GetRoleEnforcementStats()
	if enfAfter <= enfInit {
		t.Errorf("expected enforcementsTotal to increment, got before=%d after=%d", enfInit, enfAfter)
	}
	if ovAfter <= ovInit {
		t.Errorf("expected roleOverridesTotal to increment, got before=%d after=%d", ovInit, ovAfter)
	}

	// 2. Error path test: AllowedRoles filtering with disallowed role
	errConfig := &ServerConfig{}
	errConfig.MCPServer.Security.AllowedRoles = []string{"developer"}
	disallowedClientInfo := map[string]any{
		objects.FieldKeyClientID: "test-client-2",
		objects.FieldKeyRoles:    []string{"unauthorized_role"},
	}

	_, errDisallowed := enforceRoleEnforcement(disallowedClientInfo, errConfig, "", nil)
	if errDisallowed == nil {
		t.Fatalf("expected error for disallowed role")
	}

	_, _, errFinal := GetRoleEnforcementStats()
	if errFinal <= errAfter {
		t.Fatalf("expected roleErrorsTotal to increment, got init=%d after=%d", errAfter, errFinal)
	}
}
