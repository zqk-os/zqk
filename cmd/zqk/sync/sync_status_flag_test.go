package sync

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	syncpkg "github.com/zqk-os/zqk/pkg/adapters/sync"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

type mockSyncStorage struct {
	storage.ObjectStorageProvider
	items   []map[string]any
	listErr error
}

func newMockSyncStorage(items []map[string]any, listErr error) *mockSyncStorage {
	return &mockSyncStorage{
		ObjectStorageProvider: storage.NewNoopObjectStorage(),
		items:                 items,
		listErr:               listErr,
	}
}

func (m *mockSyncStorage) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	var res []map[string]any
	for _, it := range m.items {
		if filter.Kind == "" || it[objects.FieldKeyKind] == filter.Kind {
			res = append(res, it)
		}
	}
	return &storage.QueryResult{Objects: res}, nil
}

func buildPopulatedSyncBacklogItems() []map[string]any {
	return []map[string]any{
		{
			objects.FieldKeyID:           "BLI-COMMUNITY-001",
			objects.FieldKeyKind:         objects.KindBacklogItem,
			objects.FieldKeyOriginSystem: string(syncpkg.SourceGitHub),
		},
		{
			objects.FieldKeyID:           PrefixBliGitHub + "002",
			objects.FieldKeyKind:         objects.KindBacklogItem,
			objects.FieldKeyOriginSystem: "tracker",
		},
		{
			objects.FieldKeyID:           "BLI-COMMUNITY-003",
			objects.FieldKeyKind:         objects.KindBacklogItem,
			objects.FieldKeyOriginSystem: string(syncpkg.SourceLinear),
		},
		{
			objects.FieldKeyID:           PrefixBliLinear + "004",
			objects.FieldKeyKind:         objects.KindBacklogItem,
			objects.FieldKeyOriginSystem: "tracker",
		},
		{
			objects.FieldKeyID:           "BLI-COMMUNITY-005",
			objects.FieldKeyKind:         objects.KindBacklogItem,
			objects.FieldKeyOriginSystem: "local",
		},
	}
}

func TestSyncStatus_PopulatedItems(t *testing.T) {
	isolateSyncTest(t)
	items := buildPopulatedSyncBacklogItems()
	mockStore := newMockSyncStorage(items, nil)

	cmd := newSyncStatusCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetContext(cli.WithStorageProvider(context.Background(), mockStore))

	err := cmd.Execute()
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, MsgSyncStatusHeader)
	assert.Contains(t, out, "GitHub Issues Synchronized: 2")
	assert.Contains(t, out, "Linear Tickets Synchronized: 2")
	assert.Contains(t, out, "Total Synced Work Units:    4")
}

func TestSyncStatus_StorageListError(t *testing.T) {
	isolateSyncTest(t)
	mockStore := newMockSyncStorage(nil, errors.New("simulated storage list error"))

	cmd := newSyncStatusCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetContext(cli.WithStorageProvider(context.Background(), mockStore))

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "simulated storage list error")
}

func TestSyncStatus_FlagParsingError(t *testing.T) {
	isolateSyncTest(t)
	cmd := newSyncStatusCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--unrecognized-option"})

	err := cmd.Execute()
	require.Error(t, err)
}

func TestGitHubSync_FlagParsingErrors(t *testing.T) {
	isolateSyncTest(t)

	// Cobra unknown flag rejection
	cmd := newGitHubSyncCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--unrecognized-option"})
	err := cmd.Execute()
	require.Error(t, err)

	// FlagBag getter error path
	cmdRun := newGitHubSyncCmd()
	cmdRun.ResetFlags()
	errRun := cmdRun.RunE(cmdRun, nil)
	require.Error(t, errRun)
}

func TestLinearSync_FlagParsingErrors(t *testing.T) {
	isolateSyncTest(t)

	// Cobra unknown flag rejection
	cmd := newLinearSyncCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--unrecognized-option"})
	err := cmd.Execute()
	require.Error(t, err)

	// FlagBag getter error path
	cmdRun := newLinearSyncCmd()
	cmdRun.ResetFlags()
	errRun := cmdRun.RunE(cmdRun, nil)
	require.Error(t, errRun)
}

func TestResolveKernelSyncStore_SecurityContextVariants(t *testing.T) {
	// 0. Proc nil returns ErrStorageUnavailable
	storeNilProc, errNilProc := resolveKernelSyncStore(nil)
	require.Error(t, errNilProc)
	assert.Nil(t, storeNilProc)
	assert.Equal(t, ErrStorageUnavailable, errNilProc.Error())

	// executeSyncOperation with nil proc returns ErrStorageUnavailable
	cmdNilProc, bufNilProc := newCapturingSyncCmd()
	require.NotNil(t, bufNilProc)
	errNilProcExec := executeSyncOperation(cmdNilProc, nil, syncExecutionParams{})
	require.Error(t, errNilProcExec)
	assert.Contains(t, errNilProcExec.Error(), ErrStorageUnavailable)

	// 1. Proc with nil SecurityContext fallthrough to SystemSecurityContext
	procNilSec := &cli.Processor{}
	storeNilSec, errNilSec := resolveKernelSyncStore(procNilSec)
	require.NoError(t, errNilSec)
	require.NotNil(t, storeNilSec)

	// 2. Proc with initialized SecurityContext
	isolateSyncTest(t)
	cmd := &cobra.Command{}
	sec := &storage.SecurityContext{AccountID: "ACC-COMMUNITY-001"}
	cmd.SetContext(pkgctx.WithSecurityContext(context.Background(), sec))
	cmd.SetContext(cli.WithStorageProvider(cmd.Context(), storage.NewNoopObjectStorage()))

	procWithSec, errProc := cli.NewProcessor(cmd)
	require.NoError(t, errProc)
	storeWithSec, errStore := resolveKernelSyncStore(procWithSec)
	require.NoError(t, errStore)
	require.NotNil(t, storeWithSec)
}

func TestPrintSyncDryRun_LinearSimulation(t *testing.T) {
	cmd, buf := newCapturingSyncCmd()
	printSyncDryRun(cmd, MsgDryRunLinearFmt, "TEAM-CORE", "push")

	out := buf.String()
	assert.Contains(t, out, "[DRY-RUN] Linear Ticket synchronization simulation for team: TEAM-CORE")
	assert.Contains(t, out, "Direction: push | Target: Knowledge Kernel CAS")
	assert.Contains(t, out, "Verified connection and payload schema compatibility (0 conflicts).")
}

func TestExecuteSyncOperation_Branches(t *testing.T) {
	// Branch 1: DryRun simulation
	cmdDry, bufDry := newCapturingSyncCmd()
	procDry := &cli.Processor{}
	errDry := executeSyncOperation(cmdDry, procDry, syncExecutionParams{
		target:      "zqk-os/zqk",
		dryRun:      true,
		direction:   "pull",
		dryRunFmt:   MsgDryRunGitHubFmt,
		requiredErr: MsgErrGitHubTokenRequired,
		completeFmt: MsgGitHubSyncCompletedFmt,
	})
	require.NoError(t, errDry)
	assert.Contains(t, bufDry.String(), "[DRY-RUN]")

	// Branch 2: Missing credential returns required error
	cmdErr, bufErr := newCapturingSyncCmd()
	require.NotNil(t, bufErr)
	procErr := &cli.Processor{}
	errMissing := executeSyncOperation(cmdErr, procErr, syncExecutionParams{
		target:      "zqk-os/zqk",
		dryRun:      false,
		direction:   "pull",
		credential:  "",
		dryRunFmt:   MsgDryRunGitHubFmt,
		requiredErr: MsgErrGitHubTokenRequired,
		completeFmt: MsgGitHubSyncCompletedFmt,
	})
	require.Error(t, errMissing)
	assert.Equal(t, MsgErrGitHubTokenRequired, errMissing.Error())

	// Branch 3: Valid execution
	cmdOk, bufOk := newCapturingSyncCmd()
	procOk := &cli.Processor{}
	errOk := executeSyncOperation(cmdOk, procOk, syncExecutionParams{
		target:      "TEAM-CORE",
		dryRun:      false,
		direction:   "bidirectional",
		credential:  "valid-token-secret",
		dryRunFmt:   MsgDryRunLinearFmt,
		requiredErr: MsgErrLinearAPIKeyRequired,
		completeFmt: MsgLinearSyncCompletedFmt,
	})
	require.NoError(t, errOk)
	assert.Contains(t, bufOk.String(), "Linear synchronization completed for team TEAM-CORE")
}
