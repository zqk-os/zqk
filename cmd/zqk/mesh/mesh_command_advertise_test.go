package mesh

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

type cycle95MeshStubStorage struct {
	storage.ObjectStorageProvider
	createErr  error
	listErr    error
	listResult *storage.QueryResult
}

func (s *cycle95MeshStubStorage) Create(ctx context.Context, secCtx *storage.SecurityContext, obj map[string]any) error {
	if s.createErr != nil {
		return s.createErr
	}
	return s.ObjectStorageProvider.Create(ctx, secCtx, obj)
}

func (s *cycle95MeshStubStorage) List(ctx context.Context, secCtx *storage.SecurityContext, storageCtx *storage.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	if s.listResult != nil {
		return s.listResult, nil
	}
	return s.ObjectStorageProvider.List(ctx, secCtx, storageCtx, filter)
}

func executeMeshCmdWithStorage(t *testing.T, proj testkit.IsolatedTempProject, cmd *cobra.Command, args []string, prov storage.ObjectStorageProvider) (string, error) {
	t.Helper()
	cli.SetContext(cmd, cli.ContextForProjectRoot(proj.Root))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	if prov != nil {
		ctx = cli.WithStorageProvider(ctx, prov)
	}
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(ctx)
	return buf.String(), err
}

func TestMesh_SubcommandProperties(t *testing.T) {
	advCmd := NewAdvertiseCmd()
	require.NotNil(t, advCmd)
	assert.Equal(t, "advertise <resource_type>", advCmd.Use)
	assert.NotEmpty(t, advCmd.Short)
	assert.NotNil(t, advCmd.Flags().Lookup("amount"))
	assert.NotNil(t, advCmd.Flags().Lookup("units"))
	assert.NotNil(t, advCmd.Flags().Lookup("resource-id"))

	leaseCmd := NewLeaseCmd()
	require.NotNil(t, leaseCmd)
	assert.Equal(t, "lease <resource_id>", leaseCmd.Use)
	assert.NotEmpty(t, leaseCmd.Short)
	assert.NotNil(t, leaseCmd.Flags().Lookup("from"))
	assert.NotNil(t, leaseCmd.Flags().Lookup("duration"))

	mktCmd := NewMarketCmd()
	require.NotNil(t, mktCmd)
	assert.Equal(t, "market", mktCmd.Use)
	assert.NotEmpty(t, mktCmd.Short)
	assert.NotNil(t, mktCmd.Flags().Lookup("type"))
}

func TestAdvertiseCmd_OmitAmount(t *testing.T) {
	proj := setupMeshTestProject(t)
	cmd := NewAdvertiseCmd()
	out, err := executeMeshCmd(t, proj, cmd, []string{"compute", "--units", "nodes"})
	require.NoError(t, err)
	assert.Contains(t, out, "Broadcasting compute capacity: 0.0 nodes")
}

func TestAdvertiseCmd_FlagErrors(t *testing.T) {
	proj := setupMeshTestProject(t)
	bareCmd := &cobra.Command{Use: "advertise"}
	cli.SetContext(bareCmd, cli.ContextForProjectRoot(proj.Root))
	err := runAdvertise(bareCmd, []string{"compute"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "flag \"amount\"")
}

func TestAdvertiseCmd_StorageFailure(t *testing.T) {
	proj := setupMeshTestProject(t)
	stub := &cycle95MeshStubStorage{
		ObjectStorageProvider: proj.FileStorage,
		createErr:             errors.New("disk failure on create"),
	}

	cmd := NewAdvertiseCmd()
	_, err := executeMeshCmdWithStorage(t, proj, cmd, []string{"compute", "--amount", "4"}, stub)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create advertisement")
	assert.Contains(t, err.Error(), "disk failure on create")
}

func TestLeaseCmd_FlagErrors(t *testing.T) {
	proj := setupMeshTestProject(t)
	bareCmd := &cobra.Command{Use: "lease"}
	cli.SetContext(bareCmd, cli.ContextForProjectRoot(proj.Root))
	err := runLease(bareCmd, []string{"res-1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "flag \"from\"")
}

func TestLeaseCmd_StorageFailure(t *testing.T) {
	proj := setupMeshTestProject(t)
	stub := &cycle95MeshStubStorage{
		ObjectStorageProvider: proj.FileStorage,
		createErr:             errors.New("disk failure on lease creation"),
	}

	cmd := NewLeaseCmd()
	_, err := executeMeshCmdWithStorage(t, proj, cmd, []string{"res-123", "--from", "KER-COMMUNITY-001"}, stub)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create lease")
	assert.Contains(t, err.Error(), "disk failure on lease creation")
}

func TestMarketCmd_FlagErrors(t *testing.T) {
	proj := setupMeshTestProject(t)
	bareCmd := &cobra.Command{Use: "market"}
	cli.SetContext(bareCmd, cli.ContextForProjectRoot(proj.Root))
	err := runMarket(bareCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "flag \"type\"")
}

func TestMarketCmd_StorageFailure(t *testing.T) {
	proj := setupMeshTestProject(t)
	stub := &cycle95MeshStubStorage{
		ObjectStorageProvider: proj.FileStorage,
		listErr:               errors.New("database connection refused"),
	}

	cmd := NewMarketCmd()
	_, err := executeMeshCmdWithStorage(t, proj, cmd, []string{}, stub)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list market")
	assert.Contains(t, err.Error(), "database connection refused")
}

func TestMarketCmd_Int64QuantityAndFallback(t *testing.T) {
	proj := setupMeshTestProject(t)

	mockResult := &storage.QueryResult{
		Objects: []map[string]any{
			{
				objects.FieldKeyID:                "ADV-MOCK-001",
				objects.FieldKeyTitle:             "Offer: gpu",
				objects.FieldKeyProviderKernelRef: "KER-PROVIDER-01",
				objects.FieldKeyResourceType:      "gpu",
				objects.FieldKeyQuantity:          int64(16),
				objects.FieldKeyUnits:             "cards",
				objects.FieldKeyResourceID:        "nvidia-h100",
			},
			{
				objects.FieldKeyID:                "ADV-MOCK-002",
				objects.FieldKeyTitle:             "Offer: unquantified",
				objects.FieldKeyProviderKernelRef: "KER-PROVIDER-02",
				objects.FieldKeyResourceType:      "agent_skill",
				objects.FieldKeyQuantity:          "unknown-type",
				objects.FieldKeyUnits:             "units",
			},
		},
	}

	stub := &cycle95MeshStubStorage{
		ObjectStorageProvider: proj.FileStorage,
		listResult:            mockResult,
	}

	cmd := NewMarketCmd()
	out, err := executeMeshCmdWithStorage(t, proj, cmd, []string{}, stub)
	require.NoError(t, err)
	assert.Contains(t, out, "SOVEREIGN MESH MARKETPLACE")
	assert.Contains(t, out, "gpu")
	assert.Contains(t, out, "16.0 cards")
	assert.Contains(t, out, "nvidia-h100")
	assert.Contains(t, out, "agent_skill")
	assert.Contains(t, out, "0.0 units")
}
