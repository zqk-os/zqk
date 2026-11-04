package docman

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

type interceptStorageProvider struct {
	storage.ObjectStorageProvider
	onCreate func()
}

func (s *interceptStorageProvider) Create(ctx context.Context, secCtx *storage.SecurityContext, obj map[string]any) error {
	err := s.ObjectStorageProvider.Create(ctx, secCtx, obj)
	if s.onCreate != nil {
		s.onCreate()
	}
	return err
}

type failListStorageProvider struct {
	storage.ObjectStorageProvider
	err error
}

func (s *failListStorageProvider) List(ctx context.Context, secCtx *storage.SecurityContext, storageCtx *storage.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.ObjectStorageProvider.List(ctx, secCtx, storageCtx, filter)
}

func createElevationTestWorkspace(t *testing.T) (string, func()) {
	t.Helper()
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		SkipSetupTestEnvironment: false,
		SkipFileStorage:          false,
	})
	tmpDir := proj.Root

	t.Setenv(zqkenv.ProjectRoot().Name(), tmpDir)
	t.Setenv(zqkenv.TestRoot().Name(), tmpDir)

	origDir, err := fileutil.Getwd()
	require.NoError(t, err)

	err = fileutil.Chdir(tmpDir)
	require.NoError(t, err)

	cleanup := func() {
		chdirErr := fileutil.Chdir(origDir)
		require.NoError(t, chdirErr)
		cli.UnregisterStorageForProjectRoot(tmpDir)
	}
	t.Cleanup(cleanup)

	return tmpDir, cleanup
}

func writeElevationTestFile(t *testing.T, baseDir, relPath, content string) string {
	t.Helper()
	fullPath := filepath.Join(baseDir, relPath)
	err := fileutil.MkdirAll(filepath.Dir(fullPath), paths.DirPerm755)
	require.NoError(t, err)
	err = fileutil.WriteFile(fullPath, []byte(content), paths.FilePerm644)
	require.NoError(t, err)
	return fullPath
}

func executeElevationCommandWithOutput(cmd *cobra.Command, args []string) (string, error) {
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(pkgctx.NewSystemContext(), &buf)
	cmd.SetContext(ctx)
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestDocman_Elevation_InitDocmanEnv_ProjectRootNotFound(t *testing.T) {
	emptyDir := t.TempDir()
	origDir, err := fileutil.Getwd()
	require.NoError(t, err)

	t.Setenv(zqkenv.ProjectRoot().Name(), "")
	t.Setenv(zqkenv.TestRoot().Name(), "")

	err = fileutil.Chdir(emptyDir)
	require.NoError(t, err)
	defer func() {
		chdirErr := fileutil.Chdir(origDir)
		require.NoError(t, chdirErr)
		cli.UnregisterStorageForProjectRoot(emptyDir)
	}()

	cmd := NewRegisterCmd()
	env, err := initDocmanEnv(cmd)
	require.Error(t, err)
	assert.Nil(t, env)
	assert.Contains(t, err.Error(), "project root not found")

	invoked := false
	wrapped := withDocmanEnv(func(c *cobra.Command, e *docmanEnv) error {
		invoked = true
		return nil
	})
	wrappedErr := wrapped(cmd, []string{})
	require.Error(t, wrappedErr)
	assert.False(t, invoked)
	assert.Contains(t, wrappedErr.Error(), "project root not found")
}

func TestDocman_Elevation_InitDocmanEnv_FlagsError(t *testing.T) {
	tmpDir, _ := createElevationTestWorkspace(t)

	// Test invalid subtrees flag type (causes resolveSubtreesAndShipped to fail)
	badSubtreesCmd := &cobra.Command{Use: "bad-subtrees"}
	badSubtreesCmd.Flags().Int("subtrees", 42, "invalid flag type for test")
	badSubtreesCmd.Flags().Bool("shipped-only", false, "shipped-only flag")

	env1, err1 := initDocmanEnv(badSubtreesCmd)
	require.Error(t, err1)
	assert.Nil(t, env1)
	assert.Contains(t, err1.Error(), "failed to get subtrees flag")

	// Test invalid shipped-only flag type
	badShippedCmd := &cobra.Command{Use: "bad-shipped"}
	badShippedCmd.Flags().StringSlice("subtrees", nil, "subtrees flag")
	badShippedCmd.Flags().Int("shipped-only", 123, "invalid flag type for test")

	env2, err2 := initDocmanEnv(badShippedCmd)
	require.Error(t, err2)
	assert.Nil(t, env2)
	assert.Contains(t, err2.Error(), "failed to get shipped-only flag")

	// Verify resolveSubtreesAndShipped directly with both invalid flag types
	_, _, flagErr1 := resolveSubtreesAndShipped(badSubtreesCmd)
	require.Error(t, flagErr1)
	assert.Contains(t, flagErr1.Error(), "failed to get subtrees flag")

	_, _, flagErr2 := resolveSubtreesAndShipped(badShippedCmd)
	require.Error(t, flagErr2)
	assert.Contains(t, flagErr2.Error(), "failed to get shipped-only flag")

	cli.UnregisterStorageForProjectRoot(tmpDir)
}

func TestDocman_Elevation_ResolveDocmanContext_Matrix(t *testing.T) {
	tmpDir, _ := createElevationTestWorkspace(t)
	defer cli.UnregisterStorageForProjectRoot(tmpDir)

	// Test with nil cmd context and custom context flag
	cmd := NewRegisterCmd()
	cmd.Flags().String("context", "", "context flag")
	cmd.SetContext(nil)
	err := cmd.Flags().Set("context", "ai-agent")
	require.NoError(t, err)

	cmdCtx, profile, projectRoot, sp, err := resolveDocmanContext(cmd)
	require.NoError(t, err)
	assert.NotNil(t, cmdCtx)
	assert.Equal(t, "ai-agent", profile)
	assert.Equal(t, tmpDir, projectRoot)
	assert.NotNil(t, sp)

	// Test with empty context flag (should default to Human)
	cmd2 := NewRegisterCmd()
	cmd2.Flags().String("context", "", "context flag")
	err = cmd2.Flags().Set("context", "")
	require.NoError(t, err)

	_, profile2, _, _, err := resolveDocmanContext(cmd2)
	require.NoError(t, err)
	assert.Equal(t, string(pkgctx.ProfileHuman), profile2)
}

func TestDocman_Elevation_ResolveSubtreesAndShipped_StringArrayAndMissing(t *testing.T) {
	// 1. Command with StringArray for subtrees (like verifyCmd)
	cmdArray := &cobra.Command{Use: "test-array"}
	cmdArray.Flags().StringArray("subtrees", []string{"docs/arch", "docs/guides"}, "subtrees")
	cmdArray.Flags().Bool("shipped-only", true, "shipped-only")

	subtrees, shipped, err := resolveSubtreesAndShipped(cmdArray)
	require.NoError(t, err)
	assert.True(t, shipped)
	assert.Equal(t, []string{"docs/arch", "docs/guides"}, subtrees)

	// 2. Command without subtrees flag
	cmdNoSubtrees := &cobra.Command{Use: "test-no-subtrees"}
	cmdNoSubtrees.Flags().Bool("shipped-only", false, "shipped-only")

	subtreesNone, shippedNone, err := resolveSubtreesAndShipped(cmdNoSubtrees)
	require.NoError(t, err)
	assert.False(t, shippedNone)
	assert.Nil(t, subtreesNone)
}

func TestDocman_Elevation_WithDocmanEnv_SuccessFlow(t *testing.T) {
	tmpDir, _ := createElevationTestWorkspace(t)

	called := false
	cmd := NewRegisterCmd()
	wrapped := withDocmanEnv(func(c *cobra.Command, env *docmanEnv) error {
		called = true
		assert.Equal(t, tmpDir, env.projectRoot)
		assert.NotNil(t, env.storageProvider)
		assert.NotNil(t, env.logger)
		return nil
	})

	err := wrapped(cmd, []string{"ignored-arg"})
	require.NoError(t, err)
	assert.True(t, called)
}

func TestDocman_Elevation_CommandConstructionAndMetadata(t *testing.T) {
	root := NewDocmanCmd()
	require.NotNil(t, root)
	assert.Equal(t, "docman", root.Use)
	assert.Contains(t, root.Short, "Documentation management operations")

	reg := NewRegisterCmd()
	require.NotNil(t, reg)
	assert.Equal(t, "register", reg.Use)
	assert.NotNil(t, reg.Flags().Lookup("dry-run"))
	assert.NotNil(t, reg.Flags().Lookup("update-existing"))
	assert.NotNil(t, reg.Flags().Lookup("subtrees"))
	assert.NotNil(t, reg.Flags().Lookup("shipped-only"))

	ver := NewVerifyCmd()
	require.NotNil(t, ver)
	assert.Equal(t, "verify", ver.Use)
	assert.NotNil(t, ver.Flags().Lookup("ids"))
	assert.NotNil(t, ver.Flags().Lookup("auto-seal"))
	assert.NotNil(t, ver.Flags().Lookup("strict"))
}

func TestDocman_Elevation_Register_AllDocsDefaultAndSkipFlow(t *testing.T) {
	tmpDir, _ := createElevationTestWorkspace(t)

	writeElevationTestFile(t, tmpDir, "docs/general/spec.md", "# Specification\nGeneral spec document.")
	writeElevationTestFile(t, tmpDir, "docs/manual/usage.md", "# Manual\nUsage instructions.")

	// Dry run across all docs/ (no subtrees or shipped-only flag)
	dryCmd := NewRegisterCmd()
	dryOut, err := executeElevationCommandWithOutput(dryCmd, []string{"--dry-run"})
	require.NoError(t, err)
	assert.Contains(t, dryOut, "Would create 2 doc_entry objects")
	assert.Contains(t, dryOut, "Would skip 0 files")

	// Real run across all docs/
	realCmd := NewRegisterCmd()
	realOut, err := executeElevationCommandWithOutput(realCmd, []string{})
	require.NoError(t, err)
	assert.Contains(t, realOut, "Created 2 doc_entry objects")
	assert.Contains(t, realOut, "Skipped 0 files")

	// Second real run: files are already registered, so they must be skipped
	skipCmd := NewRegisterCmd()
	skipOut, err := executeElevationCommandWithOutput(skipCmd, []string{})
	require.NoError(t, err)
	assert.Contains(t, skipOut, "Created 0 doc_entry objects")
	assert.Contains(t, skipOut, "Skipped 2 files (already registered)")
}

func TestDocman_Elevation_Register_InterruptedRegistrationBranch(t *testing.T) {
	tmpDir, _ := createElevationTestWorkspace(t)

	writeElevationTestFile(t, tmpDir, "docs/test/a.md", "# Doc A\nAlpha content.")
	writeElevationTestFile(t, tmpDir, "docs/test/b.md", "# Doc B\nBeta content.")

	factory, err := storage.NewStorageFactory(context.Background(), tmpDir)
	require.NoError(t, err)
	baseStorage := factory.GetStorageForKind(objects.KindDocEntry)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wrappedStorage := &interceptStorageProvider{
		ObjectStorageProvider: baseStorage,
		onCreate: func() {
			// Cancel context immediately after the first doc_entry is created in storage
			cancel()
		},
	}

	cmd := NewRegisterCmd()
	var outBuf bytes.Buffer
	ctx = pkgctx.WithCommandOutputWriter(ctx, &outBuf)
	cmd.SetContext(ctx)
	cmd.SetOut(&outBuf)
	cmd.SetErr(&outBuf)

	env := &docmanEnv{
		ctx:             ctx,
		profile:         string(pkgctx.ProfileHuman),
		projectRoot:     tmpDir,
		storageProvider: wrappedStorage,
		logger:          logging.GetLoggerFromProfile(string(pkgctx.ProfileHuman)),
		subtrees:        []string{"docs/test"},
		shippedOnly:     false,
	}

	runErr := runRegister(cmd, env)
	require.Error(t, runErr)
	assert.Contains(t, runErr.Error(), "failed to register documentation")
	assert.Contains(t, runErr.Error(), "created 1, skipped 0")
	assert.Contains(t, outBuf.String(), "Created 1 doc_entry objects before interruption")
}

func TestDocman_Elevation_Register_UpdateExistingFailure(t *testing.T) {
	tmpDir, _ := createElevationTestWorkspace(t)

	factory, err := storage.NewStorageFactory(context.Background(), tmpDir)
	require.NoError(t, err)
	baseStorage := factory.GetStorageForKind(objects.KindDocEntry)

	failingStorage := &failListStorageProvider{
		ObjectStorageProvider: baseStorage,
		err:                   errfmt.Errorf("simulated storage list error"),
	}

	cmd := NewRegisterCmd()
	err = cmd.Flags().Set("update-existing", "true")
	require.NoError(t, err)

	env := &docmanEnv{
		ctx:             context.Background(),
		profile:         string(pkgctx.ProfileHuman),
		projectRoot:     tmpDir,
		storageProvider: failingStorage,
		logger:          logging.GetLoggerFromProfile(string(pkgctx.ProfileHuman)),
		subtrees:        nil,
		shippedOnly:     false,
	}

	runErr := runRegister(cmd, env)
	require.Error(t, runErr)
	assert.Contains(t, runErr.Error(), "failed to update existing entries")
}

func TestDocman_Elevation_Verify_InvalidIDsFlag(t *testing.T) {
	tmpDir, _ := createElevationTestWorkspace(t)
	defer cli.UnregisterStorageForProjectRoot(tmpDir)

	// Command with bad ids flag type
	badCmd := &cobra.Command{Use: "bad-verify"}
	badCmd.Flags().Int("ids", 12345, "invalid ids flag type")
	badCmd.Flags().Bool("auto-seal", false, "auto-seal")
	badCmd.Flags().Bool("strict", true, "strict")

	env := &docmanEnv{
		ctx:             context.Background(),
		profile:         string(pkgctx.ProfileHuman),
		projectRoot:     tmpDir,
		storageProvider: nil,
		logger:          logging.GetLoggerFromProfile(string(pkgctx.ProfileHuman)),
	}

	err := runVerify(badCmd, env)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get ids flag")
}

func TestDocman_Elevation_Verify_NonExistentIDFailure(t *testing.T) {
	tmpDir, _ := createElevationTestWorkspace(t)
	defer cli.UnregisterStorageForProjectRoot(tmpDir)

	verifyCmd := NewVerifyCmd()
	_, err := executeElevationCommandWithOutput(verifyCmd, []string{"--ids", "DOC-NONEXISTENT-999"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "verification failed")
}

func TestDocman_Elevation_Verify_AllPassedZeroViolations(t *testing.T) {
	tmpDir, _ := createElevationTestWorkspace(t)

	writeElevationTestFile(t, tmpDir, "docs/architecture/clean.md", "# Clean Architecture\nClean doc content.")

	regCmd := NewRegisterCmd()
	_, err := executeElevationCommandWithOutput(regCmd, []string{"--subtrees=docs/architecture"})
	require.NoError(t, err)

	verifyCmd := NewVerifyCmd()
	out, err := executeElevationCommandWithOutput(verifyCmd, []string{"--subtrees=docs/architecture"})
	require.NoError(t, err)
	assert.Contains(t, out, "Scanned 1 doc_entry object(s):")
	assert.Contains(t, out, "Passed: 1")
	assert.Contains(t, out, "Drifted: 0")
	assert.Contains(t, out, "✓ All checked doc_entry objects cryptographically verified with 0 drift violations.")
}

func TestDocman_Elevation_Verify_DriftReportingNonStrict(t *testing.T) {
	tmpDir, _ := createElevationTestWorkspace(t)

	docPath := writeElevationTestFile(t, tmpDir, "docs/architecture/drift.md", "# Pristine\nPristine content.")

	regCmd := NewRegisterCmd()
	_, err := executeElevationCommandWithOutput(regCmd, []string{"--subtrees=docs/architecture"})
	require.NoError(t, err)

	// Tamper with file
	err = fileutil.WriteFile(docPath, []byte("# Tampered\nModified bytes causing hash drift."), paths.FilePerm644)
	require.NoError(t, err)

	// In non-strict mode, verify should report violations but return nil error
	verifyCmd := NewVerifyCmd()
	out, err := executeElevationCommandWithOutput(verifyCmd, []string{"--subtrees=docs/architecture", "--strict=false"})
	require.NoError(t, err)
	assert.Contains(t, out, "Drifted: 1")
	assert.Contains(t, out, "Violations:")
	assert.Contains(t, out, "cryptographic drift detected")
}

func TestDocman_Elevation_Verify_SubtreesFilter(t *testing.T) {
	tmpDir, _ := createElevationTestWorkspace(t)

	writeElevationTestFile(t, tmpDir, "docs/architecture/a.md", "# Arch A\nArch doc.")
	writeElevationTestFile(t, tmpDir, "docs/onboarding/b.md", "# Onboarding B\nOnboarding doc.")

	regCmd := NewRegisterCmd()
	_, err := executeElevationCommandWithOutput(regCmd, []string{})
	require.NoError(t, err)

	// Verify only docs/architecture
	verifyCmd := NewVerifyCmd()
	out, err := executeElevationCommandWithOutput(verifyCmd, []string{"--subtrees=docs/architecture"})
	require.NoError(t, err)
	assert.Contains(t, out, "Scanned 1 doc_entry object(s):")
	assert.Contains(t, out, "Passed: 1")
}
