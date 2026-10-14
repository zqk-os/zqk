package mesh

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_v2"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
)

func setupMeshTestProject(t *testing.T) testkit.IsolatedTempProject {
	t.Helper()
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		SeedSchemaPlane: true,
	})
	specsDir := filepath.Join(proj.Root, paths.ProcessInternalObjectSpecsDir)
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir specs: %v", err)
	}
	if err := builders.NewSpecGenerator(specsDir).GenerateAllSpecs(); err != nil {
		t.Fatalf("GenerateAllSpecs: %v", err)
	}

	skillLeaseSpecPath := filepath.Join(specsDir, "skill_lease.yaml")
	if _, err := fileutil.Stat(skillLeaseSpecPath); err != nil {
		specContent := `schema_version: "2.0.0"
kind: skill_lease
extends: base_object
description: "Represents an active lease of a skill, agent seat, or resource in the sovereign mesh."
visibility: public
traits:
  - base_object_traits
fields:
  provider_kernel_ref:
    type: string
  consumer_kernel_ref:
    type: string
  resource_ref:
    type: string
  token_id:
    type: string
  expires_at:
    type: string
`
		if err := fileutil.WriteFile(skillLeaseSpecPath, []byte(specContent), paths.FilePerm644); err != nil {
			t.Fatalf("write skill_lease spec: %v", err)
		}
	}

	t.Chdir(proj.Root)
	validation.ResetGlobalIDPrefixesConfig()
	objects.GetGlobalSpecLoader().ClearCache()
	objects.GetGlobalFieldRegistry().TryReloadFromFindSpecsDir()
	objects.ResetGlobalKindMapperForTesting()
	return proj
}

func executeMeshCmd(t *testing.T, proj testkit.IsolatedTempProject, cmd *cobra.Command, args []string) (string, error) {
	t.Helper()
	cli.SetContext(cmd, cli.ContextForProjectRoot(proj.Root))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(ctx)
	return buf.String(), err
}

func TestResolveLocalKernelID(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)

	kernelID, err := resolveLocalKernelID(proj.Root)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(kernelID, "KER-"), "kernel ID should start with KER-")

	emptyID, err := resolveLocalKernelID("")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(emptyID, "KER-"), "empty root kernel ID should start with KER-")
}

func TestGenerateToken(t *testing.T) {
	tok1 := generateToken()
	tok2 := generateToken()

	assert.True(t, strings.HasPrefix(tok1, "TOK-"))
	assert.True(t, strings.HasPrefix(tok2, "TOK-"))
	assert.NotEqual(t, tok1, tok2, "consecutive tokens should be distinct")
}

func TestAdvertiseCmd_Success(t *testing.T) {
	proj := setupMeshTestProject(t)

	cmd := NewAdvertiseCmd()
	out, err := executeMeshCmd(t, proj, cmd, []string{"compute", "--amount", "8.0", "--units", "cores", "--resource-id", "comp-worker-1"})

	require.NoError(t, err)
	assert.Contains(t, out, "Broadcasting compute capacity: 8.0 cores")
}

func TestAdvertiseCmd_DefaultUnits(t *testing.T) {
	proj := setupMeshTestProject(t)

	cmd := NewAdvertiseCmd()
	out, err := executeMeshCmd(t, proj, cmd, []string{"agent_seat", "--amount", "2"})

	require.NoError(t, err)
	assert.Contains(t, out, "Broadcasting agent_seat capacity: 2.0 instances")
}

func TestAdvertiseCmd_InvalidAmount(t *testing.T) {
	proj := setupMeshTestProject(t)

	cmd := NewAdvertiseCmd()
	_, err := executeMeshCmd(t, proj, cmd, []string{"agent_seat", "--amount", "not-a-number"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid amount")
}

func TestLeaseCmd_Success(t *testing.T) {
	proj := setupMeshTestProject(t)

	cmd := NewLeaseCmd()
	out, err := executeMeshCmd(t, proj, cmd, []string{"qa-verifier", "--from", "KER-REMOTE-001", "--duration", "2h"})

	require.NoError(t, err)
	assert.Contains(t, out, "Lease established for qa-verifier")
	assert.Contains(t, out, "Authorization Token: TOK-")
}

func TestLeaseCmd_ZeroDuration(t *testing.T) {
	proj := setupMeshTestProject(t)

	cmd := NewLeaseCmd()
	out, err := executeMeshCmd(t, proj, cmd, []string{"sandbox-01", "--from", "KER-REMOTE-002", "--duration", "0s"})

	require.NoError(t, err)
	assert.Contains(t, out, "Lease established for sandbox-01")
}

func TestMarketCmd_Empty(t *testing.T) {
	proj := setupMeshTestProject(t)

	cmd := NewMarketCmd()
	out, err := executeMeshCmd(t, proj, cmd, []string{})

	require.NoError(t, err)
	assert.Contains(t, out, "No resources currently advertised in the mesh market")
}

func TestMarketCmd_WithAdvertisements(t *testing.T) {
	proj := setupMeshTestProject(t)

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := storage.WithSyncCreateForKind(context.Background(), objects.KindCapacityAdvertisement)

	// Seed item 1: float64 quantity with resource ID
	item1 := map[string]any{
		objects.FieldKeyKind:              objects.KindCapacityAdvertisement,
		objects.FieldKeyTitle:             "Offer: compute (4.5 cores)",
		objects.FieldKeyProviderKernelRef: "KER-REMOTE-COMP",
		objects.FieldKeyResourceType:      "compute",
		objects.FieldKeyQuantity:          4.5,
		objects.FieldKeyUnits:             "cores",
		objects.FieldKeyResourceID:        "comp-node-42",
		objects.FieldKeyStatus:            objects.ObjectStatusImplemented,
	}
	require.NoError(t, proj.FileStorage.Create(ctx, secCtx, item1))

	// Seed item 2: int quantity without resource ID
	item2 := map[string]any{
		objects.FieldKeyKind:              objects.KindCapacityAdvertisement,
		objects.FieldKeyTitle:             "Offer: agent_seat (2 seats)",
		objects.FieldKeyProviderKernelRef: "KER-REMOTE-AGENTS",
		objects.FieldKeyResourceType:      "agent_seat",
		objects.FieldKeyQuantity:          2,
		objects.FieldKeyUnits:             "seats",
		objects.FieldKeyStatus:            objects.ObjectStatusImplemented,
	}
	require.NoError(t, proj.FileStorage.Create(ctx, secCtx, item2))

	// Seed item 3: int64 quantity
	item3 := map[string]any{
		objects.FieldKeyKind:              objects.KindCapacityAdvertisement,
		objects.FieldKeyTitle:             "Offer: storage (100 GB)",
		objects.FieldKeyProviderKernelRef: "KER-REMOTE-STORAGE",
		objects.FieldKeyResourceType:      "storage",
		objects.FieldKeyQuantity:          int64(100),
		objects.FieldKeyUnits:             "GB",
		objects.FieldKeyResourceID:        "vol-nvme-01",
		objects.FieldKeyStatus:            objects.ObjectStatusImplemented,
	}
	require.NoError(t, proj.FileStorage.Create(ctx, secCtx, item3))

	// Run market query without filter
	cmd := NewMarketCmd()
	out, err := executeMeshCmd(t, proj, cmd, []string{})

	require.NoError(t, err)
	assert.Contains(t, out, "SOVEREIGN MESH MARKETPLACE")
	assert.Contains(t, out, "compute")
	assert.Contains(t, out, "4.5 cores")
	assert.Contains(t, out, "comp-node-42")
	assert.Contains(t, out, "agent_seat")
	assert.Contains(t, out, "2.0 seats")
	assert.Contains(t, out, "storage")
	assert.Contains(t, out, "100.0 GB")
	assert.Contains(t, out, "vol-nvme-01")

	// Run market query with --type compute filter
	cmdFiltered := NewMarketCmd()
	outFiltered, err := executeMeshCmd(t, proj, cmdFiltered, []string{"--type", "compute"})

	require.NoError(t, err)
	assert.Contains(t, outFiltered, "compute")
	assert.NotContains(t, outFiltered, "agent_seat")
	assert.NotContains(t, outFiltered, "vol-nvme-01")
}
