package domain

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

type mockStorageWrapper struct {
	storagepkg.ObjectStorageProvider
	listErr   error
	createErr error
	updateErr error
}

func (m *mockStorageWrapper) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storagepkg.ListFilter) (*storagepkg.QueryResult, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.ObjectStorageProvider.List(ctx, secCtx, storageCtx, filter)
}

func (m *mockStorageWrapper) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	if m.createErr != nil {
		return m.createErr
	}
	return m.ObjectStorageProvider.Create(ctx, secCtx, obj)
}

func (m *mockStorageWrapper) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	return m.ObjectStorageProvider.Update(ctx, secCtx, id, updates)
}

func TestDomainCmd_MetadataAndHelp(t *testing.T) {
	cmd := NewDomainCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "domain", cmd.Use)
	assert.True(t, cmd.HasSubCommands())

	subCmds := make(map[string]*cobra.Command)
	for _, sub := range cmd.Commands() {
		subCmds[sub.Name()] = sub
	}
	assert.NotNil(t, subCmds["discover"], "missing discover command")
	assert.NotNil(t, subCmds["register"], "missing register command")

	// Verify register flags
	regCmd := subCmds["register"]
	for _, flagName := range []string{"domain-id", objects.KindNamespace, "spec-file", "dry-run"} {
		f := regCmd.Flags().Lookup(flagName)
		assert.NotNil(t, f, "expected flag %s on register", flagName)
	}
}

func TestDomainDiscover_ErrorHandling(t *testing.T) {
	testRoot, _ := setupIntegrationTestWithSpecsDomain(t, "domain-discover-error")
	storage, err := storagepkg.NewFileObjectStorageForTest(testRoot)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, testRoot, storage)
	t.Cleanup(func() {
		_ = storagepkg.RunProjectTestTeardown(storagepkg.TempProjectTeardown(testRoot, storage))
	})

	mockStore := &mockStorageWrapper{
		ObjectStorageProvider: storage,
		listErr:               errors.New("simulated storage list error"),
	}

	cmd := NewDiscoverCmd()
	cmd.SetArgs([]string{})
	cli.SetContext(cmd, cli.ContextForProjectRoot(testRoot))
	cmd.SetContext(cli.WithStorageProvider(context.Background(), mockStore))

	err = cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list domain_registry objects")
}

func TestDomainRegister_ErrorHandling(t *testing.T) {
	testRoot, _ := setupIntegrationTestWithSpecsDomain(t, "domain-register-errors")
	storage, err := storagepkg.NewFileObjectStorageForTest(testRoot)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, testRoot, storage)
	t.Cleanup(func() {
		_ = storagepkg.RunProjectTestTeardown(storagepkg.TempProjectTeardown(testRoot, storage))
	})

	// 1. List error during register
	mockStoreListErr := &mockStorageWrapper{
		ObjectStorageProvider: storage,
		listErr:               errors.New("simulated list error on register"),
	}
	cmdListErr := NewRegisterCmd()
	cmdListErr.SetArgs([]string{"--domain-id", "dom-err-1"})
	cli.SetContext(cmdListErr, cli.ContextForProjectRoot(testRoot))
	cmdListErr.SetContext(cli.WithStorageProvider(context.Background(), mockStoreListErr))
	err = cmdListErr.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list domain_registry")

	// 2. Create error on initial registry creation
	mockStoreCreateErr := &mockStorageWrapper{
		ObjectStorageProvider: storage,
		createErr:             errors.New("simulated create error"),
	}
	cmdCreateErr := NewRegisterCmd()
	cmdCreateErr.SetArgs([]string{"--domain-id", "dom-err-2"})
	cli.SetContext(cmdCreateErr, cli.ContextForProjectRoot(testRoot))
	cmdCreateErr.SetContext(cli.WithStorageProvider(context.Background(), mockStoreCreateErr))
	err = cmdCreateErr.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create domain_registry")

	// 3. Create a real registry first, then simulate update error on subsequent register
	cmdInitial := NewRegisterCmd()
	cmdInitial.SetArgs([]string{"--domain-id", "dom-seed"})
	cli.SetContext(cmdInitial, cli.ContextForProjectRoot(testRoot))
	cmdInitial.SetContext(cli.WithStorageProvider(context.Background(), storage))
	require.NoError(t, cmdInitial.Execute())

	mockStoreUpdateErr := &mockStorageWrapper{
		ObjectStorageProvider: storage,
		updateErr:             errors.New("simulated update error"),
	}
	cmdUpdateErr := NewRegisterCmd()
	cmdUpdateErr.SetArgs([]string{"--domain-id", "dom-err-3"})
	cli.SetContext(cmdUpdateErr, cli.ContextForProjectRoot(testRoot))
	cmdUpdateErr.SetContext(cli.WithStorageProvider(context.Background(), mockStoreUpdateErr))
	err = cmdUpdateErr.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update domain_registry")
}

func TestDomainCoverage_EntryVariations(t *testing.T) {
	testRoot, _ := setupIntegrationTestWithSpecsDomain(t, "domain-register-variations")
	storage, err := storagepkg.NewFileObjectStorageForTest(testRoot)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, testRoot, storage)
	t.Cleanup(func() {
		_ = storagepkg.RunProjectTestTeardown(storagepkg.TempProjectTeardown(testRoot, storage))
	})

	// 1. Discover empty registry
	discEmptyCmd := NewDiscoverCmd()
	discEmptyCmd.SetArgs([]string{})
	cli.SetContext(discEmptyCmd, cli.ContextForProjectRoot(testRoot))
	discEmptyCmd.SetContext(cli.WithStorageProvider(context.Background(), storage))
	outEmpty := captureStdoutDomain(t, func() {
		require.NoError(t, discEmptyCmd.Execute())
	})
	assert.True(t, strings.Contains(outEmpty, "No domain registries found"))

	// 2. Register with only domain-id
	cmd1 := NewRegisterCmd()
	cmd1.SetArgs([]string{"--domain-id", "minimal-domain"})
	cli.SetContext(cmd1, cli.ContextForProjectRoot(testRoot))
	cmd1.SetContext(cli.WithStorageProvider(context.Background(), storage))
	out1 := captureStdoutDomain(t, func() {
		require.NoError(t, cmd1.Execute())
	})
	assert.True(t, strings.Contains(out1, "Domain registered: minimal-domain"))
	assert.True(t, strings.Contains(out1, "DOMAIN-REG-001"))

	// 3. Register second domain with spec-file and namespace
	cmd2 := NewRegisterCmd()
	cmd2.SetArgs([]string{
		"--domain-id", "full-domain",
		"--namespace", "custom:domain:*",
		"--spec-file", "specs/domain.yaml",
	})
	cli.SetContext(cmd2, cli.ContextForProjectRoot(testRoot))
	cmd2.SetContext(cli.WithStorageProvider(context.Background(), storage))
	out2 := captureStdoutDomain(t, func() {
		require.NoError(t, cmd2.Execute())
	})
	assert.True(t, strings.Contains(out2, "Domain registered: full-domain"))
	assert.True(t, strings.Contains(out2, "DOMAIN-REG-001"))

	// 4. Discover and verify formatting
	discCmd := NewDiscoverCmd()
	discCmd.SetArgs([]string{})
	cli.SetContext(discCmd, cli.ContextForProjectRoot(testRoot))
	discCmd.SetContext(cli.WithStorageProvider(context.Background(), storage))
	discOut := captureStdoutDomain(t, func() {
		require.NoError(t, discCmd.Execute())
	})
	assert.True(t, strings.Contains(discOut, "Discovered 1 domain registr(y/ies):"))
	assert.True(t, strings.Contains(discOut, "DOMAIN-REG-001"))

	// 5. Verify stored domains
	regObj, err := storage.Read(context.Background(), pkgctx.NewSystemSecurityContext(), "DOMAIN-REG-001")
	require.NoError(t, err)
	domains, ok := regObj[objects.FieldKeyDomains].([]any)
	require.True(t, ok)
	assert.Len(t, domains, 2)
}
