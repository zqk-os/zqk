package system

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

type sysStorageCtx = pkgctx.StorageContext

type testStubStorage struct {
	storage.NoopObjectStorage
	readFn func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error)
	listFn func(ctx context.Context, secCtx *pkgctx.SecurityContext, sCtx *sysStorageCtx, filter storage.ListFilter) (*storage.QueryResult, error)
}

func (s *testStubStorage) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if s.readFn != nil {
		return s.readFn(ctx, secCtx, id)
	}
	return nil, storage.ErrNoopObjectStorage
}

func (s *testStubStorage) List(ctx context.Context, secCtx *pkgctx.SecurityContext, sCtx *sysStorageCtx, f storage.ListFilter) (*storage.QueryResult, error) {
	if s.listFn != nil {
		return s.listFn(ctx, secCtx, sCtx, f)
	}
	return nil, storage.ErrNoopObjectStorage
}

func newTestSystemLogger() logging.Logger {
	return logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewTextFormatter(pkgctx.NewSystemContext()))
}

func setupValidateDirectProof(t *testing.T) (*cobra.Command, logging.Logger) {
	t.Helper()
	t.Parallel()
	return NewValidateCmd(), newTestSystemLogger()
}

func executeValidateCmd(projectRoot string, args ...string) (string, error) {
	cmd := NewValidateCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd.SetContext(ctx)
	if projectRoot != "" {
		cli.SetContext(cmd, cli.ContextForProjectRoot(projectRoot))
	}
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestNewValidateCmd_StaticFloorInvariant(t *testing.T) {
	t.Parallel()
	cmd := NewValidateCmd()
	require.NotNil(t, cmd)

	assert.True(t, strings.HasPrefix(cmd.Use, "validate"))
	assert.NotEmpty(t, cmd.Short)
	assert.NotNil(t, cmd.Flags().Lookup("all"))
	assert.NotNil(t, cmd.Flags().Lookup("kind"))
	assert.NotNil(t, cmd.Flags().Lookup("fix"))
	assert.NotNil(t, cmd.Flags().Lookup("quiet"))
}

func TestValidate_NegativeBoundary_NotZQKProject(t *testing.T) {
	t.Parallel()

	out, err := executeValidateCmd("/nonexistent/invalid/zqk/project/path")
	require.Error(t, err)
	assert.NotNil(t, out)
	assert.Contains(t, err.Error(), "failed to initialize storage factory")
}

func TestValidate_OperationalProof_AgainstRepoObjects(t *testing.T) {
	repoRoot := systemFindProjectRoot()
	if repoRoot == "" {
		t.Skip("repo root not found")
	}

	// 1. Single existing object
	outSingle, errSingle := executeValidateCmd(repoRoot, "BLI-1791926625066508000-d95cc3cd")
	require.NoError(t, errSingle)
	assert.NotNil(t, outSingle)

	// 2. Single existing object with JSON and YAML format
	outJSON, errJSON := executeValidateCmd(repoRoot, "BLI-1791926625066508000-d95cc3cd", "-f", "json")
	require.NoError(t, errJSON)
	assert.Contains(t, outJSON, `"valid": true`)

	outYAML, errYAML := executeValidateCmd(repoRoot, "BLI-1791926625066508000-d95cc3cd", "-f", "yaml")
	require.NoError(t, errYAML)
	assert.Contains(t, outYAML, "valid: true")

	// 3. Specific kind with text, JSON, and YAML formats
	outKindText, errKindText := executeValidateCmd(repoRoot, "--kind", "milestone")
	require.NoError(t, errKindText)
	assert.NotNil(t, outKindText)

	outKindJSON, errKindJSON := executeValidateCmd(repoRoot, "--kind", "milestone", "-f", "json")
	require.NoError(t, errKindJSON)
	assert.Contains(t, outKindJSON, `"valid": true`)

	outKindYAML, errKindYAML := executeValidateCmd(repoRoot, "--kind", "milestone", "-f", "yaml")
	require.NoError(t, errKindYAML)
	assert.Contains(t, outKindYAML, "valid: true")

	// 4. All objects with text, JSON, and YAML formats
	outAllText, errAllText := executeValidateCmd(repoRoot, "--all")
	require.NoError(t, errAllText)
	assert.NotNil(t, outAllText)

	outAllJSON, errAllJSON := executeValidateCmd(repoRoot, "--all", "-f", "json")
	require.NoError(t, errAllJSON)
	assert.Contains(t, outAllJSON, `"valid": true`)

	// 5. Default with no args (same as --all)
	outDefault, errDefault := executeValidateCmd(repoRoot)
	require.NoError(t, errDefault)
	assert.NotNil(t, outDefault)

	// 6. Nonexistent object
	outMissing, errMissing := executeValidateCmd(repoRoot, "BLI-DOES-NOT-EXIST-000")
	require.Error(t, errMissing)
	assert.NotNil(t, outMissing)
}

func TestValidateObject_DirectUnitProofs(t *testing.T) {
	cmd, logger := setupValidateDirectProof(t)

	// 1. Valid object
	validStore := &testStubStorage{
		readFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			return map[string]any{
				objects.FieldKeyID:   id,
				objects.FieldKeyKind: "backlog_item",
			}, nil
		},
	}
	valid, err := validateObject(cmd, validStore, "BLI-OK", false, false, logger)
	require.NoError(t, err)
	assert.True(t, valid)

	// 2. Read error
	errStore := &testStubStorage{
		readFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			return nil, errors.New("storage read failure")
		},
	}
	validErr, errRead := validateObject(cmd, errStore, "BLI-FAIL", false, false, logger)
	require.Error(t, errRead)
	assert.False(t, validErr)
	assert.Contains(t, errRead.Error(), "failed to get object")

	// 3. Obj nil
	nilStore := &testStubStorage{
		readFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			return nil, nil
		},
	}
	validNil, errNil := validateObject(cmd, nilStore, "BLI-NIL", false, false, logger)
	require.Error(t, errNil)
	assert.False(t, validNil)
	assert.Contains(t, errNil.Error(), "not found")

	// 4. Missing ID
	missingIDStore := &testStubStorage{
		readFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			return map[string]any{
				objects.FieldKeyKind: "backlog_item",
			}, nil
		},
	}
	validMissingID, errMissingID := validateObject(cmd, missingIDStore, "BLI-NOID", false, false, logger)
	require.Error(t, errMissingID)
	assert.False(t, validMissingID)
	assert.Contains(t, errMissingID.Error(), "missing required field: id")

	// 5. Missing Kind
	missingKindStore := &testStubStorage{
		readFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			return map[string]any{
				objects.FieldKeyID: id,
			}, nil
		},
	}
	validMissingKind, errMissingKind := validateObject(cmd, missingKindStore, "BLI-NOKIND", false, false, logger)
	require.Error(t, errMissingKind)
	assert.False(t, validMissingKind)
	assert.Contains(t, errMissingKind.Error(), "missing required field: kind")
}

func TestValidateKind_DirectUnitProofs(t *testing.T) {
	cmd, logger := setupValidateDirectProof(t)

	// 1. List error
	errStore := &testStubStorage{
		listFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, sCtx *sysStorageCtx, filter storage.ListFilter) (*storage.QueryResult, error) {
			return nil, errors.New("list error")
		},
	}
	vN, invN, err := validateKind(cmd, errStore, "backlog_item", false, false, logger)
	require.Error(t, err)
	assert.Equal(t, 0, vN)
	assert.Equal(t, 0, invN)

	// 2. Objects missing ID
	missingIDStore := &testStubStorage{
		listFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, sCtx *sysStorageCtx, filter storage.ListFilter) (*storage.QueryResult, error) {
			return &storage.QueryResult{
				Objects: []map[string]any{
					{objects.FieldKeyKind: "backlog_item"},
				},
			}, nil
		},
	}
	vN2, invN2, err2 := validateKind(cmd, missingIDStore, "backlog_item", false, false, logger)
	require.Error(t, err2)
	assert.Equal(t, 0, vN2)
	assert.Equal(t, 1, invN2)

	// 3. Kind mismatch
	mismatchStore := &testStubStorage{
		listFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, sCtx *sysStorageCtx, filter storage.ListFilter) (*storage.QueryResult, error) {
			return &storage.QueryResult{
				Objects: []map[string]any{
					{
						objects.FieldKeyID:   "BLI-MISMATCH",
						objects.FieldKeyKind: "goal",
					},
				},
			}, nil
		},
	}
	vN3, invN3, err3 := validateKind(cmd, mismatchStore, "backlog_item", false, false, logger)
	require.Error(t, err3)
	assert.Equal(t, 0, vN3)
	assert.Equal(t, 1, invN3)

	// 4. Clean valid objects
	validStore := &testStubStorage{
		listFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, sCtx *sysStorageCtx, filter storage.ListFilter) (*storage.QueryResult, error) {
			return &storage.QueryResult{
				Objects: []map[string]any{
					{
						objects.FieldKeyID:   "BLI-CLEAN",
						objects.FieldKeyKind: "backlog_item",
					},
				},
			}, nil
		},
	}
	vN4, invN4, err4 := validateKind(cmd, validStore, "backlog_item", false, false, logger)
	require.NoError(t, err4)
	assert.Equal(t, 1, vN4)
	assert.Equal(t, 0, invN4)
}

func TestValidateAll_DirectUnitProofs(t *testing.T) {
	cmd, logger := setupValidateDirectProof(t)

	// 1. All valid
	validStore := &testStubStorage{
		listFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, sCtx *sysStorageCtx, filter storage.ListFilter) (*storage.QueryResult, error) {
			return &storage.QueryResult{
				Objects: []map[string]any{
					{
						objects.FieldKeyID:   "ITEM-" + filter.Kind,
						objects.FieldKeyKind: filter.Kind,
					},
				},
			}, nil
		},
	}
	tValid, tInvalid, err := validateAll(cmd, validStore, false, false, logger)
	require.NoError(t, err)
	assert.True(t, tValid > 0)
	assert.Equal(t, 0, tInvalid)

	// 2. Has issues
	issueStore := &testStubStorage{
		listFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, sCtx *sysStorageCtx, filter storage.ListFilter) (*storage.QueryResult, error) {
			if filter.Kind == objects.KindBacklogItem {
				return &storage.QueryResult{
					Objects: []map[string]any{
						{objects.FieldKeyKind: "backlog_item"}, // missing id
					},
				}, nil
			}
			return &storage.QueryResult{Objects: []map[string]any{}}, nil
		},
	}
	tValid2, tInvalid2, err2 := validateAll(cmd, issueStore, false, false, logger)
	require.Error(t, err2)
	assert.Equal(t, 0, tValid2)
	assert.Equal(t, 1, tInvalid2)
}
