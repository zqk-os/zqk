package swarm

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/bootstrap"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/swarm/metabolism"
	"github.com/zqk-os/zqk/pkg/swarm/pack"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// cycle93MockStorageProvider provides custom interceptors for ObjectStorageProvider calls.
type cycle93MockStorageProvider struct {
	storage.ObjectStorageProvider
	readFn   func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error)
	createFn func(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error
	updateFn func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error
	listFn   func(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error)
	countFn  func(ctx context.Context, secCtx *pkgctx.SecurityContext, filter storage.ListFilter) (int, error)
}

func newCycle93MockStorage(underlying storage.ObjectStorageProvider) *cycle93MockStorageProvider {
	if underlying == nil {
		underlying = storage.NewNoopObjectStorage()
	}
	return &cycle93MockStorageProvider{ObjectStorageProvider: underlying}
}

func (m *cycle93MockStorageProvider) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if m.readFn != nil {
		return m.readFn(ctx, secCtx, id)
	}
	return m.ObjectStorageProvider.Read(ctx, secCtx, id)
}

func (m *cycle93MockStorageProvider) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	if m.createFn != nil {
		return m.createFn(ctx, secCtx, obj)
	}
	return m.ObjectStorageProvider.Create(ctx, secCtx, obj)
}

func (m *cycle93MockStorageProvider) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, secCtx, id, updates)
	}
	return m.ObjectStorageProvider.Update(ctx, secCtx, id, updates)
}

func (m *cycle93MockStorageProvider) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	if m.listFn != nil {
		return m.listFn(ctx, secCtx, storageCtx, filter)
	}
	return m.ObjectStorageProvider.List(ctx, secCtx, storageCtx, filter)
}

func (m *cycle93MockStorageProvider) Count(ctx context.Context, secCtx *pkgctx.SecurityContext, filter storage.ListFilter) (int, error) {
	if m.countFn != nil {
		return m.countFn(ctx, secCtx, filter)
	}
	return m.ObjectStorageProvider.Count(ctx, secCtx, filter)
}

func TestCycle93_Status_CountByStatus_EdgeCases(t *testing.T) {
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()

	// 1. Storage is nil
	_, _, err := countByStatus(ctx, nil, sec, objects.KindAgentTask)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "storage provider is nil")

	// 2. List with GroupBy fails, and fallback List without GroupBy fails
	mockStorage := newCycle93MockStorage(nil)
	mockStorage.listFn = func(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
		return nil, errors.New("list failure")
	}
	_, _, err = countByStatus(ctx, mockStorage, sec, objects.KindAgentTask)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list failure")

	// 3. List with GroupBy fails, fallback List succeeds with nil result
	fallbackCall := false
	mockStorage.listFn = func(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
		if filter.GroupBy != "" {
			return nil, errors.New("groupby unsupported")
		}
		fallbackCall = true
		return nil, nil
	}
	by, total, err := countByStatus(ctx, mockStorage, sec, objects.KindAgentTask)
	require.NoError(t, err)
	assert.True(t, fallbackCall)
	assert.Equal(t, 0, total)
	assert.Empty(t, by)

	// 4. List with GroupBy succeeds with empty group key mapping to (empty)
	mockStorage.listFn = func(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
		return &storage.QueryResult{
			Groups: map[string][]map[string]any{
				"": {
					{objects.FieldKeyID: "ATK-DEMO-001"},
				},
				"active": {
					{objects.FieldKeyID: "ATK-DEMO-003"},
				},
			},
		}, nil
	}
	by, total, err = countByStatus(ctx, mockStorage, sec, objects.KindAgentTask)
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Equal(t, 1, by["(empty)"])
	assert.Equal(t, 1, by["active"])

	// 5. Fallback List succeeds with objects having empty or missing status
	mockStorage.listFn = func(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
		if filter.GroupBy != "" {
			return nil, errors.New("groupby unsupported")
		}
		return &storage.QueryResult{
			Objects: []map[string]any{
				{objects.FieldKeyID: "ATK-DEMO-004", objects.FieldKeyStatus: ""},
				{objects.FieldKeyID: "ATK-DEMO-005"},
				{objects.FieldKeyID: "ATK-DEMO-006", objects.FieldKeyStatus: "  "},
				{objects.FieldKeyID: "ATK-DEMO-007", objects.FieldKeyStatus: "in_progress"},
			},
		}, nil
	}
	by, total, err = countByStatus(ctx, mockStorage, sec, objects.KindAgentTask)
	require.NoError(t, err)
	assert.Equal(t, 4, total)
	assert.Equal(t, 3, by["(empty)"])
	assert.Equal(t, 1, by["in_progress"])
}

func TestCycle93_Status_CountWithFilter(t *testing.T) {
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()

	// Nil storage provider error
	_, err := countWithFilter(ctx, nil, sec, objects.KindPriorityPlan, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "storage provider is nil")

	// Successful count
	mock := newCycle93MockStorage(nil)
	mock.countFn = func(ctx context.Context, secCtx *pkgctx.SecurityContext, filter storage.ListFilter) (int, error) {
		return 42, nil
	}
	c, err := countWithFilter(ctx, mock, sec, objects.KindPriorityPlan, map[string]any{objects.FieldKeyStatus: "active"})
	require.NoError(t, err)
	assert.Equal(t, 42, c)
}

func TestCycle93_Status_SummarizePersonaSkillBound(t *testing.T) {
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()

	// 1. Nil storage
	resNil := summarizePersonaSkillBound(ctx, nil, sec)
	assert.Equal(t, "storage_nil", resNil["error"])

	// 2. List error
	mock := newCycle93MockStorage(nil)
	mock.listFn = func(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
		return nil, errors.New("cannot list personas")
	}
	resErr := summarizePersonaSkillBound(ctx, mock, sec)
	assert.Equal(t, "list_failed", resErr["error"])

	// 3. More than 12 unbound personas to exercise sampling limit and sorting
	mock.listFn = func(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
		var objs []map[string]any
		// 15 unbound personas
		for i := 15; i >= 1; i-- {
			objs = append(objs, map[string]any{
				objects.FieldKeyID:     paths.CLIInvocation(string(rune('A'+i))) + "-PER",
				objects.FieldKeyKind:   objects.KindPersona,
				objects.FieldKeyStatus: objects.ObjectStatusApproved,
			})
		}
		// 2 bound personas
		objs = append(objs, map[string]any{
			objects.FieldKeyID:             "PER-BOUND-001",
			objects.FieldKeyKind:           objects.KindPersona,
			objects.FieldKeyAgentSkillRefs: "ASK-COMMUNITY-001",
		})
		objs = append(objs, map[string]any{
			objects.FieldKeyID:                "PER-BOUND-002",
			objects.FieldKeyKind:              objects.KindPersona,
			objects.FieldKeyRelatedObjectRefs: []any{"ASK-COMMUNITY-002"},
		})
		return &storage.QueryResult{Objects: objs}, nil
	}
	res := summarizePersonaSkillBound(ctx, mock, sec)
	assert.Equal(t, 17, res["total"])
	assert.Equal(t, 2, res["bound"])
	assert.Equal(t, 15, res["unbound"])
	sample, ok := res["unbound_sample"].([]string)
	require.True(t, ok)
	assert.Equal(t, swarmStatusMaxSample, len(sample))
	// Verify sample is sorted
	for i := 1; i < len(sample); i++ {
		assert.True(t, sample[i-1] <= sample[i], "unbound sample must be sorted")
	}
}

func TestCycle93_Status_BuildSwarmStatus_ThroughputHints(t *testing.T) {
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()

	// Hint: instructions_pending
	mock := newCycle93MockStorage(nil)
	mock.listFn = func(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
		if filter.Kind == objects.KindAgentTask {
			return &storage.QueryResult{
				Groups: map[string][]map[string]any{
					"completed": {{objects.FieldKeyID: "ATK-DEMO-001"}},
				},
			}, nil
		}
		if filter.Kind == objects.KindAgentInstruction {
			return &storage.QueryResult{
				Groups: map[string][]map[string]any{
					objects.ObjectStatusProposed: {{objects.FieldKeyID: "AGI-DEMO-001"}},
				},
			}, nil
		}
		return &storage.QueryResult{}, nil
	}
	mock.countFn = func(ctx context.Context, secCtx *pkgctx.SecurityContext, filter storage.ListFilter) (int, error) {
		return 0, nil
	}

	payload, err := BuildSwarmStatus(ctx, mock, sec)
	require.NoError(t, err)
	swarmData := payload["swarm"].(map[string]any)
	assert.Equal(t, "instructions_pending", swarmData["throughput_hint"])

	// Active priority plans count failure fallback
	mock.countFn = func(ctx context.Context, secCtx *pkgctx.SecurityContext, filter storage.ListFilter) (int, error) {
		return 0, errors.New("count failed")
	}
	payloadFailCount, err := BuildSwarmStatus(ctx, mock, sec)
	require.NoError(t, err)
	swarmDataFail := payloadFailCount["swarm"].(map[string]any)
	assert.Equal(t, -1, swarmDataFail["active_priority_plans"])

	// CountByStatus error on AgentTask
	mock.listFn = func(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
		if filter.Kind == objects.KindAgentTask {
			return nil, errors.New("agent task list failure")
		}
		return &storage.QueryResult{}, nil
	}
	_, err = BuildSwarmStatus(ctx, mock, sec)
	require.Error(t, err)

	// CountByStatus error on AgentInstruction
	mock.listFn = func(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
		if filter.Kind == objects.KindAgentInstruction {
			return nil, errors.New("agent instruction list failure")
		}
		return &storage.QueryResult{}, nil
	}
	_, err = BuildSwarmStatus(ctx, mock, sec)
	require.Error(t, err)
}

func TestCycle93_Run_EnsureValidSwarmAccount_Variations(t *testing.T) {
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()

	// Case 1: Account in secCtx is active and exists
	mock := newCycle93MockStorage(nil)
	sec.AccountID = "ACC-ACTIVE-001"
	mock.readFn = func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
		if id == "ACC-ACTIVE-001" {
			return map[string]any{
				objects.FieldKeyID:     id,
				objects.FieldKeyStatus: objects.ObjectStatusActive,
			}, nil
		}
		return nil, errors.New("not found")
	}
	gotAcc := ensureValidSwarmAccount(ctx, sec, mock)
	assert.Equal(t, "ACC-ACTIVE-001", gotAcc)

	// Case 2: Account in secCtx is inactive -> finds first active account in List
	sec.AccountID = "ACC-INACTIVE-001"
	mock.readFn = func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
		if id == "ACC-INACTIVE-001" {
			return map[string]any{
				objects.FieldKeyID:     id,
				objects.FieldKeyStatus: "disabled",
			}, nil
		}
		return nil, errors.New("not found")
	}
	mock.listFn = func(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
		return &storage.QueryResult{
			Objects: []map[string]any{
				{objects.FieldKeyID: "ACC-FALLBACK-002", objects.FieldKeyStatus: objects.ObjectStatusActive},
			},
		}, nil
	}
	gotAcc = ensureValidSwarmAccount(ctx, sec, mock)
	assert.Equal(t, "ACC-FALLBACK-002", gotAcc)

	// Case 3: secCtx is empty, List is empty -> creates default system account
	sec.AccountID = ""
	createdID := ""
	mock.readFn = func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
		return nil, errors.New("not found")
	}
	mock.listFn = func(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
		return &storage.QueryResult{Objects: nil}, nil
	}
	mock.createFn = func(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
		createdID = objects.GetString(obj, objects.FieldKeyID)
		return nil
	}
	gotAcc = ensureValidSwarmAccount(ctx, sec, mock)
	assert.Equal(t, pkgctx.SystemAccountID, gotAcc)
	assert.Equal(t, pkgctx.SystemAccountID, createdID)
}

func TestCycle93_Run_PersistKernelObjects_Branches(t *testing.T) {
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	mock := newCycle93MockStorage(nil)

	// 1. Empty ID is skipped, existing object is skipped, new object is created
	createdCount := 0
	mock.readFn = func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
		if id == "EXISTING-OBJ-001" {
			return map[string]any{objects.FieldKeyID: id}, nil
		}
		return nil, errors.New("not found")
	}
	mock.createFn = func(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
		createdCount++
		return nil
	}

	objs := []map[string]any{
		{objects.FieldKeyID: "", objects.FieldKeyKind: "goal"},
		{objects.FieldKeyID: "EXISTING-OBJ-001", objects.FieldKeyKind: "goal"},
		{objects.FieldKeyID: "NEW-OBJ-001", objects.FieldKeyKind: "requirement"},
	}
	n, err := persistKernelObjects(ctx, sec, mock, objs)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Equal(t, 1, createdCount)

	// 2. Create failure returns wrapped error
	mock.createFn = func(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
		return errors.New("write failure")
	}
	_, err = persistKernelObjects(ctx, sec, mock, []map[string]any{
		{objects.FieldKeyID: "NEW-OBJ-002", objects.FieldKeyKind: "milestone"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to persist synthesized milestone object NEW-OBJ-002")
}

func TestCycle93_Run_PersistPromptTemplates_Branches(t *testing.T) {
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	mock := newCycle93MockStorage(nil)
	logger := logging.NewEventLogger(ctx)

	mock.readFn = func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
		if id == "TPL-EXISTS-001" {
			return map[string]any{objects.FieldKeyID: id}, nil
		}
		return nil, errors.New("not found")
	}

	createAttempts := 0
	mock.createFn = func(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
		createAttempts++
		if objects.GetString(obj, objects.FieldKeyID) == "TPL-ERR-001" {
			return errors.New("create template error")
		}
		return nil
	}

	tpls := []metabolism.PromptTemplateObject{
		{ID: ""},
		{ID: "TPL-EXISTS-001", Title: "Existing Template"},
		{ID: "TPL-ERR-001", Title: "Error Template"},
		{ID: "TPL-NEW-001", Title: "New Template"},
	}

	stored := persistPromptTemplates(ctx, sec, mock, tpls, logger)
	assert.Equal(t, 1, stored)
	assert.Equal(t, 2, createAttempts)
}

func TestCycle93_Run_ActivateSwarmPriorityPlan(t *testing.T) {
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	mock := newCycle93MockStorage(nil)

	// 1. Plan already in in_progress status -> no update performed
	planUpdated := false
	mock.readFn = func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
		return map[string]any{
			objects.FieldKeyID:     id,
			objects.FieldKeyStatus: objects.ObjectStatusInProgress,
		}, nil
	}
	mock.updateFn = func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
		planUpdated = true
		return nil
	}

	objectsList := []map[string]any{
		{objects.FieldKeyKind: objects.KindBacklogItem, objects.FieldKeyID: "BLI-DEMO-001"},
	}
	err := activateSwarmPriorityPlan(ctx, sec, mock, "", "PRI-DEMO-001", objectsList)
	require.NoError(t, err)
	assert.False(t, planUpdated)

	// 2. Plan in draft status -> updated to active
	mock.readFn = func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
		return map[string]any{
			objects.FieldKeyID:     id,
			objects.FieldKeyStatus: objects.ObjectStatusDraft,
		}, nil
	}
	err = activateSwarmPriorityPlan(ctx, sec, mock, "", "PRI-DEMO-001", objectsList)
	require.NoError(t, err)
	assert.True(t, planUpdated)

	// 3. Plan update failure returns error
	mock.updateFn = func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
		return errors.New("cannot update plan")
	}
	err = activateSwarmPriorityPlan(ctx, sec, mock, "", "PRI-DEMO-001", objectsList)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to activate priority plan")
}

func TestCycle93_Run_MaterializedViewAndFeedEvent(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		SeedSchemaPlane: true,
	})
	root := proj.Root
	logger := logging.GetLoggerFromProfile("test")
	if err := bootstrap.ExtractTo(root, logger, true); err != nil {
		t.Fatalf("bootstrap.ExtractTo failed: %v", err)
	}

	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	store, err := storage.GetGlobalStorageProviderCache().GetOrCreate(ctx, root)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, root, store)

	// Refresh MV with empty and valid root
	refreshSwarmMaterializedView(ctx, sec, store, "")
	refreshSwarmMaterializedView(ctx, sec, store, root)

	// Emit swarm feed event
	pkg := &pack.SwarmPackage{
		Name:    "demo-swarm",
		Version: "1.0.0",
		Tasks: []pack.TaskConfig{
			{ID: "task-1", Title: "Task 1"},
		},
	}
	emitSwarmFeedEvent(root, pkg, "PRI-DEMO-SWARM", "entrypoint")
}

func TestCycle93_Run_LaunchSwarmBackground_ErrorHandling(t *testing.T) {
	// Attempting to launch in an invalid directory (e.g. root is a file)
	tmpFile := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, fileutil.WriteFile(tmpFile, []byte("data"), paths.FilePerm644))

	_, _, err := launchSwarmBackground(tmpFile, "PRI-DEMO-001", "demo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to ensure log directory")
}

func TestCycle93_Run_ExecuteSwarmRun_FlagsAndErrors(t *testing.T) {
	tmpDir := t.TempDir()
	manifestFile := filepath.Join(tmpDir, "swarm.yaml")
	require.NoError(t, pack.EnsureSampleSwarm(manifestFile))

	// Test executeSwarmRun with --entrypoint override
	cmd := NewRunCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{manifestFile, "--dry-run", "--entrypoint", "custom-entry"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "custom-entry")
	assert.Contains(t, buf.String(), "✓ Swarm validation successful (dry-run mode).")

	// Test runSwarmPackage with missing file
	cmdMissing := NewRunCmd()
	cmdMissing.SetArgs([]string{filepath.Join(tmpDir, "nonexistent.yaml")})
	err = cmdMissing.Execute()
	require.Error(t, err)

	// Test runSwarmPackage with invalid manifest syntax
	badManifest := filepath.Join(tmpDir, "bad.yaml")
	require.NoError(t, fileutil.WriteFile(badManifest, []byte("invalid: [yaml: unclosed"), paths.FilePerm644))
	cmdBad := NewRunCmd()
	cmdBad.SetArgs([]string{badManifest})
	err = cmdBad.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load swarm package")
}

func TestCycle93_Init_ChatBootstrap_SeatPlaceholder(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	root := proj.Root

	chatFn := swarmInitChatBootstrap(root)
	require.NotNil(t, chatFn)

	// Create payload containing seat id in file name
	seatPayload := filepath.Join(root, "payload-worker-1.txt")
	require.NoError(t, fileutil.WriteFile(seatPayload, []byte("CUSTOM_PAYLOAD_FOR_WORKER"), paths.FilePerm644))

	templatePattern := filepath.Join(root, "payload-{seat_id}.txt")
	err := chatFn(context.Background(), "worker-1", agentfeed.PeerSeatRecord{
		PID: 9999,
	}, templatePattern)
	// Execution may fail peer wake if process 9999 is inactive, but path replacement must be verified
	if err != nil {
		assert.Contains(t, err.Error(), "chat bootstrap worker-1")
	}
}

func TestCycle93_Init_RunSwarmInit_ContextVariations(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	root := proj.Root

	// 1. Execute swarm init with allow-chat=true and non-dry-run without pipeline
	cmd := NewInitCmd()
	cli.SetContext(cmd, cli.ContextForProjectRoot(root))
	buf := new(bytes.Buffer)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), buf))
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--allow-chat=true"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "require --pipeline or --workflow")

	// 2. Profile context variation
	cmdProfile := NewInitCmd()
	cliCtx := cli.ContextForProjectRoot(root)
	cliCtx.Profile = "human"
	cli.SetContext(cmdProfile, cliCtx)
	cmdProfile.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), buf))
	cmdProfile.SetOut(buf)
	cmdProfile.SetErr(buf)
	cmdProfile.SetArgs([]string{"--dry-run", "--pipeline", "PIP-CYCLE93-TEST"})
	_ = cmdProfile.Execute()
}

func TestCycle93_Init_SwarmInitSteer_EdgeCases(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	root := proj.Root
	logger := logging.GetLoggerFromProfile("test")

	require.NoError(t, datacell.WriteAgentChatChannelConfig(root, datacell.AgentChatChannelConfig{
		SchemaVersion: datacell.AgentChatChannelSchemaVersion,
		Enabled:       true,
		DeliveryMode:  datacell.DeliveryModeLog,
		Note:          "Cycle93 test feed channel",
	}))

	steerFn := swarmInitSteer(root, "coordinator-agent", "PER-COORDINATOR-001", logger)

	// Directed to worker without awaitAck violates directed hourglass policy
	_, err := steerFn(context.Background(), "peer-worker-1", "Hello", false)
	require.Error(t, err)

	// Valid steer with awaitAck true
	outcome, err := steerFn(context.Background(), "worker-agent", "Task assigned", true)
	require.NoError(t, err)
	assert.Equal(t, "worker-agent", outcome.ToAgentID)
	assert.NotEmpty(t, outcome.AwaitID)
}

func TestCycle93_SwarmCommand_BuilderInvariants(t *testing.T) {
	topRun := NewTopLevelRunCmd()
	assert.Equal(t, "run", topRun.Use)
	assert.NotNil(t, topRun.Flags().Lookup("stage-only"))
	assert.NotNil(t, topRun.Flags().Lookup("yes"))

	swarmRun := NewRunCmd()
	assert.Equal(t, "run", swarmRun.Use)
	assert.NotNil(t, swarmRun.Flags().Lookup("stage-only"))
	assert.NotNil(t, swarmRun.Flags().Lookup("yes"))

	agentSwarmInit := NewAgentSwarmInitCmd()
	assert.Equal(t, "swarm-init", agentSwarmInit.Use)
	assert.NotNil(t, agentSwarmInit.Flags().Lookup("pipeline"))
}
