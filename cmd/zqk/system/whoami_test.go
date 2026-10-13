package system

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func setupWhoamiProject(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	testkit.RegisterTempProjectTeardown(t, tmpDir, nil)
	return tmpDir
}

func executeWhoamiCmd(ctx context.Context, projectRoot string, args ...string) (string, error) {
	cmd := NewWhoamiCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = pkgctx.WithCommandOutputWriter(ctx, &buf)
	cmd.SetContext(ctx)
	if projectRoot != "" {
		cli.SetContext(cmd, cli.ContextForProjectRoot(projectRoot))
	}
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestNewWhoamiCmd_StaticFloorInvariant(t *testing.T) {
	t.Parallel()

	cmd := NewWhoamiCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "whoami", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
}

func TestWhoami_NegativeAdversarialBoundary_NoSecurityContext(t *testing.T) {
	t.Parallel()
	tmpDir := setupWhoamiProject(t)

	_, err := executeWhoamiCmd(context.Background(), tmpDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unauthorized: no security context")
}

func TestWhoami_OperationalDynamicProof_SecurityContext(t *testing.T) {
	t.Parallel()
	tmpDir := setupWhoamiProject(t)

	sec := &pkgctx.SecurityContext{
		AccountID:   "ACC-TEST-USER-1",
		Roles:       []string{"operator", "admin"},
		Permissions: []string{"read", "write"},
		PersonaID:   "PER-TEST-PERSONA",
	}
	ctx := pkgctx.WithSecurityContext(context.Background(), sec)

	// Format: Table
	tableOut, err := executeWhoamiCmd(ctx, tmpDir)
	require.NoError(t, err)
	assert.Contains(t, tableOut, "Current User Account")
	assert.Contains(t, tableOut, "ACC-TEST-USER-1")
	assert.Contains(t, tableOut, "admin")
	assert.Contains(t, tableOut, "PER-TEST-PERSONA")

	// Format: JSON
	jsonOut, err := executeWhoamiCmd(ctx, tmpDir, "--format", "json")
	require.NoError(t, err)
	assert.Contains(t, jsonOut, `"account_id": "ACC-TEST-USER-1"`)
	assert.Contains(t, jsonOut, `"persona_ref": "PER-TEST-PERSONA"`)

	// Format: YAML
	yamlOut, err := executeWhoamiCmd(ctx, tmpDir, "--format", "yaml")
	require.NoError(t, err)
	assert.Contains(t, yamlOut, "account_id: ACC-TEST-USER-1")
	assert.Contains(t, yamlOut, "persona_ref: PER-TEST-PERSONA")
}

func TestWhoami_OperationalDynamicProof_MCPEnvironment(t *testing.T) {
	tmpDir := setupWhoamiProject(t)

	zqkenv.ApplyIsolatedStorageEnv(t.Setenv)
	t.Setenv("ZQK_MCP_ACCOUNT_ID", "ACC-MCP-001")
	t.Setenv("ZQK_MCP_ROLES", "dev,reviewer")
	t.Setenv("ZQK_MCP_PERMISSIONS", "kernel:read,kernel:write")

	out, err := executeWhoamiCmd(context.Background(), tmpDir, "--format", "json")
	require.NoError(t, err)
	assert.Contains(t, out, `"account_id": "ACC-MCP-001"`)
	assert.Contains(t, out, `"kernel:read"`)
	assert.Contains(t, out, `"dev"`)
}

func TestWhoami_OperationalDynamicProof_Helpers(t *testing.T) {
	t.Parallel()

	// MCP Roles parsing
	assert.Nil(t, parseMCPRoles(""))
	roles := parseMCPRoles("admin, planner , doer")
	assert.Equal(t, []string{"admin", "planner", "doer"}, roles)

	// MCP Permissions parsing
	assert.Nil(t, parseMCPPermissions(""))
	perms := parseMCPPermissions("read, write, execute ")
	assert.Equal(t, []string{"read", "write", "execute"}, perms)

	// Table Formatter Helpers
	data := map[string]any{
		objects.FieldKeyAccountID:   "ACC-FORMAT-1",
		"lane":                      "admin",
		objects.FieldKeyPersonaRef:  "PER-TEST",
		objects.FieldKeyDisplayName: "Test Operator",
		objects.FieldKeyUsername:    "testuser",
		objects.FieldKeyEmail:       "test@example.com",
		objects.FieldKeyRoles:       []string{"role-a", "role-b"},
		objects.FieldKeyPermissions: []string{"perm-x"},
		objects.FieldKeyContext: map[string]any{
			objects.FieldKeySource: "cli",
		},
	}

	assert.Contains(t, formatWhoamiTableAccount(data), "ACC-FORMAT-1")
	assert.Contains(t, formatWhoamiTableLane(data), "admin")
	assert.Contains(t, formatWhoamiTableName(data), "Test Operator")
	assert.Contains(t, formatWhoamiTableUserInfo(data), "testuser")
	assert.Contains(t, formatWhoamiTableUserInfo(data), "test@example.com")
	assert.Contains(t, formatWhoamiTablePersona(data), "PER-TEST")
	assert.Contains(t, formatWhoamiTableRoles(data), "role-a")
	assert.Contains(t, formatWhoamiTablePermissions(data), "perm-x")
	assert.Contains(t, formatWhoamiTableContext(data), "CLI")

	// Empty data formatting
	emptyData := map[string]any{}
	assert.Empty(t, formatWhoamiTableName(emptyData))
	assert.Empty(t, formatWhoamiTableUserInfo(emptyData))
	assert.Empty(t, formatWhoamiTablePersona(emptyData))
	assert.Empty(t, formatWhoamiTableContext(emptyData))
	assert.Empty(t, formatWhoamiTableRoles(emptyData))
	assert.Empty(t, formatWhoamiTablePermissions(emptyData))
}
