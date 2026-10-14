package kernel

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/daemon/singleton"
	"github.com/zqk-os/zqk/pkg/kernel/steward"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestSystemHygieneProvider(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, fileutil.MkdirAll(filepath.Join(tmpDir, ".zqk", "state"), 0755))
	require.NoError(t, fileutil.MkdirAll(filepath.Join(tmpDir, ".zqk", "tmp"), 0755))

	p := &systemHygieneProvider{projectRoot: tmpDir}
	ctx := context.Background()

	locks, err := p.ReapStaleLocks(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 0, locks)

	tempFiles, err := p.ReapTempFiles(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 0, tempFiles)

	err = p.CompactWAL(ctx)
	assert.NoError(t, err)
}

type planContext = storage.StorageContext

type mockPlanStorage struct {
	storage.ObjectStorageProvider
	res *storage.QueryResult
	err error
}

func (m *mockPlanStorage) List(ctx context.Context, secCtx *pkgctx.SecurityContext, sCtx *planContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.res, nil
}

func TestKernelPlanProvider(t *testing.T) {
	ctx := context.Background()

	// 1. Error path
	errMock := &mockPlanStorage{err: errors.New("list failed")}
	pErr := &kernelPlanProvider{sp: errMock}
	_, err := pErr.ListCandidatePlans(ctx)
	assert.Error(t, err)

	// 2. Empty result
	emptyMock := &mockPlanStorage{res: &storage.QueryResult{}}
	pEmpty := &kernelPlanProvider{sp: emptyMock}
	plans, err := pEmpty.ListCandidatePlans(ctx)
	assert.NoError(t, err)
	assert.Empty(t, plans)

	// 3. Plans with planned/active and backlogs
	mock := &mockPlanStorage{
		res: &storage.QueryResult{
			Objects: []map[string]any{
				{
					objects.FieldKeyID:              "PRI-001",
					objects.FieldKeyStatus:          "planned",
					objects.FieldKeyBacklogItemRefs: []any{"BLI-001"},
				},
				{
					objects.FieldKeyID:              "PRI-002",
					objects.FieldKeyStatus:          "complete",
					objects.FieldKeyBacklogItemRefs: []any{"BLI-002"},
				},
			},
		},
	}
	p := &kernelPlanProvider{sp: mock}
	plans, err = p.ListCandidatePlans(ctx)
	assert.NoError(t, err)
	assert.Len(t, plans, 2)
	assert.True(t, plans[0].ShovelReady)
	assert.False(t, plans[1].ShovelReady)
}

func TestLogSweepSummary(t *testing.T) {
	logger := logging.GetLoggerFromProfile("")

	// 1. Nil sweep
	assert.NotPanics(t, func() {
		logSweepSummary(logger, nil)
	})

	// 2. Normal sweep without signals
	sweep := &steward.SweepResult{
		SweepID:         "SWEEP-001",
		LocksReaped:     2,
		TempFilesReaped: 3,
		WALCompacted:    true,
		Duration:        100 * time.Millisecond,
		Runway: steward.RunwayStatus{
			ShovelReadyBuffer: 3,
			TargetBuffer:      5,
			IsStarvationRisk:  false,
		},
	}
	assert.NotPanics(t, func() {
		logSweepSummary(logger, sweep)
	})

	// 3. Sweep with signals generated
	sweep.SignalsGenerated = []steward.AmbientSignal{
		{SignalID: "SIG-01", Priority: "P4", Message: "buffer low", Timestamp: time.Now()},
	}
	assert.NotPanics(t, func() {
		logSweepSummary(logger, sweep)
	})
}

func TestStewardDaemonCmdLifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, fileutil.MkdirAll(filepath.Join(tmpDir, ".zqk", "state"), 0755))

	// 1. Cancelled context shuts down cleanly
	cmd := newStewardDaemonCmd()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Pre-canceled

	cli.SetContext(cmd, cli.ContextForProjectRoot(tmpDir))
	cmd.SetArgs([]string{"--interval", "1"})
	err := cmd.ExecuteContext(ctx)
	assert.NoError(t, err)

	// 2. Already running lock held by singleton
	release, err := singleton.Guard(tmpDir, "steward")
	require.NoError(t, err)
	defer release()

	cmd2 := newStewardDaemonCmd()
	cli.SetContext(cmd2, cli.ContextForProjectRoot(tmpDir))
	err = cmd2.ExecuteContext(context.Background())
	assert.NoError(t, err)
}

func TestStewardSweepCmdExecution(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, fileutil.MkdirAll(filepath.Join(tmpDir, ".zqk", "state"), 0755))

	cmd := newStewardSweepCmd()
	cli.SetContext(cmd, cli.ContextForProjectRoot(tmpDir))
	err := cmd.ExecuteContext(context.Background())
	assert.NoError(t, err)
}
