package scheduler

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/primaryorch"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestHasOpenAgentInstruction_MatchesPersonaPlanStage(t *testing.T) {
	t.Parallel()
	mStorage := &mockCapStorage{
		listed: map[string][]map[string]any{
			objects.KindAgentInstruction: {
				{
					objects.FieldKeyID:          "AGI-1",
					objects.FieldKeyStatus:      objects.ObjectStatusProposed,
					"persona_id":                "PER-coder-1",
					"plan_id":                   "PRI-PLAN-1",
					objects.FieldKeyInstruction: "cap_stage_design",
				},
			},
		},
	}
	h := NewCapOrchestratorHandler(mStorage, t.TempDir(), logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*CapOrchestratorHandler)

	if !h.hasOpenAgentInstruction(context.Background(), "PRI-PLAN-1", "PER-coder-1", "cap_stage_design") {
		t.Fatal("expected open AGI match")
	}
	if h.hasOpenAgentInstruction(context.Background(), "PRI-PLAN-1", "PER-coder-1", "cap_stage_planning") {
		t.Fatal("different stage should not match")
	}
	if h.hasOpenAgentInstruction(context.Background(), "PRI-OTHER", "PER-coder-1", "cap_stage_design") {
		t.Fatal("different plan should not match")
	}
	mStorage.listed[objects.KindAgentInstruction] = append(mStorage.listed[objects.KindAgentInstruction], map[string]any{
		objects.FieldKeyID:          "AGI-groom",
		objects.FieldKeyStatus:      objects.ObjectStatusProposed,
		"persona_id":                "tpm",
		"plan_id":                   "PRI-PLAN-1",
		objects.FieldKeyInstruction: "cap_stage_grooming GROOMING/STRATEGY stay ahead",
	})
	if !h.hasOpenTPMGroomingInstruction(context.Background(), "PRI-PLAN-1") {
		t.Fatal("expected TPM grooming reuse helper to match")
	}
}

func TestExecuteDispatchStage_ReusesOpenAgentInstruction(t *testing.T) {
	tempDir := t.TempDir()
	mockExe := filepath.Join(tempDir, "mock-zqk.sh")
	script := `#!/bin/bash
if [[ "$1" == "object" && "$2" == "show" && "$3" == "PRI-REUSE" ]]; then
	cat <<EOF
{"id":"PRI-REUSE","persona_refs":["PER-reuse-1"]}
EOF
	exit 0
fi
exit 1
`
	if err := fileutil.WriteFile(mockExe, []byte(script), paths.DirPerm755); err != nil {
		t.Fatalf("write mock: %v", err)
	}

	mStorage := &mockCapStorage{
		created: make([]map[string]any, 0),
		listed: map[string][]map[string]any{
			objects.KindAgentInstruction: {
				{
					objects.FieldKeyID:          "AGI-open",
					objects.FieldKeyStatus:      objects.ObjectStatusProposed,
					"persona_id":                "PER-reuse-1",
					"plan_id":                   "PRI-REUSE",
					objects.FieldKeyInstruction: "cap_stage_design",
				},
			},
		},
	}
	h := NewCapOrchestratorHandler(mStorage, tempDir, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))

	err := h.(*CapOrchestratorHandler).executeDispatchStage(context.Background(), mockExe, "PRI-REUSE", "cap_stage_design", nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n := len(mStorage.getCreated()); n != 0 {
		t.Fatalf("expected 0 creates when open AGI exists, got %d: %v", n, mStorage.getCreated())
	}
}

func TestCapOrchestratorHandler_Execute(t *testing.T) {
	tempDir := t.TempDir()
	t.Cleanup(func() {
		whatsnext.WaitForReconcile(2 * time.Second)
	})

	// Create a mock zqk binary
	mockZqk := filepath.Join(tempDir, "zqk")
	script := `#!/bin/bash
if [[ "$1" == "workflow" && "$2" == "whats-next" ]]; then
	cat <<EOF
{"agent_instruction":"wait"}
EOF
	exit 0
fi
exit 1
`
	if err := fileutil.WriteFile(mockZqk, []byte(script), paths.DirPerm755); err != nil {
		t.Fatalf("failed to write mock zqk: %v", err)
	}

	// Override PATH so resolveCLIExecutable finds our mock zqk
	oldPath := zqkenv.OSPath().Get()
	t.Setenv(zqkenv.OSPath().Name(), tempDir+string(fileutil.PathListSeparator)+oldPath)

	storage := &mockStorage{data: make(map[string]map[string]any)}
	h := NewCapOrchestratorHandler(storage, tempDir, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))

	job := &ScheduledJob{
		ID:      "SCH-123",
		JobType: "cap_orchestrator",
	}

	err := h.Execute(context.Background(), job)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
}

func TestWakeAgentAndScheduleHourglass_TriggersWakeAgyForTPM(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

	// Bind primary orchestrator to a script adapter (agy-style vendor wake).
	scriptsDir := filepath.Join(tempDir, "scripts")
	if err := fileutil.MkdirAll(scriptsDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create scripts dir: %v", err)
	}

	wakeLog := filepath.Join(tempDir, "wake.log")
	wakeScript := filepath.Join(scriptsDir, "wake-agy.sh")
	scriptContent := fmt.Sprintf("#!/bin/bash\necho \"$@\" >> %s\n", wakeLog)
	if err := fileutil.WriteFile(wakeScript, []byte(scriptContent), paths.DirPerm755); err != nil {
		t.Fatalf("failed to write mock wake-agy.sh: %v", err)
	}
	if err := primaryorch.WriteBinding(tempDir, primaryorch.Binding{
		SchemaVersion: primaryorch.SchemaVersion,
		AgentID:       "peer-agent-01",
		Adapter:       primaryorch.AdapterScript,
		Script:        "scripts/wake-agy.sh",
	}); err != nil {
		t.Fatalf("write primary binding: %v", err)
	}

	storage := &mockStorage{data: map[string]map[string]any{
		"PRI-TEST": {
			objects.FieldKeyID:     "PRI-TEST",
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyStatus: objects.ObjectStatusActive,
		},
		"BLI-P0": {
			objects.FieldKeyID:              "BLI-P0",
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
			objects.FieldKeyPriorityPlanRef: "PRI-TEST",
			objects.FieldKeyPriorityTier:    "P0",
			objects.FieldKeyTitle:           "Ship gate",
		},
		"BLI-P1": {
			objects.FieldKeyID:              "BLI-P1",
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyStatus:          objects.ObjectStatusInProgress,
			objects.FieldKeyPriorityPlanRef: "PRI-TEST",
			objects.FieldKeyPriorityTier:    "P1",
			objects.FieldKeyTitle:           "Wake payload",
		},
	}}
	h := NewCapOrchestratorHandler(storage, tempDir, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*CapOrchestratorHandler)

	h.wakeAgentAndScheduleHourglass("PRI-TEST", "persona:tpm")

	if _, err := fileutil.Stat(wakeLog); fileutil.IsNotExist(err) {
		t.Fatalf("expected wake script to be executed and create wake.log")
	}

	logData, _ := fileutil.ReadFile(wakeLog)
	got := string(logData)
	if !strings.Contains(got, "PRI-TEST") {
		t.Errorf("expected wake log to contain task/plan ID, got: %s", got)
	}
	if !strings.Contains(got, "PRI=PRI-TEST") {
		t.Errorf("expected PRI= line in wake message, got: %s", got)
	}
	if !strings.Contains(got, "BLI-P0") || !strings.Contains(got, "Ship gate") {
		t.Errorf("expected top BLI in wake message, got: %s", got)
	}
}

func TestCapOrchestratorHandler_executeDispatchStage(t *testing.T) {
	tempDir := t.TempDir()
	mockExe := filepath.Join(tempDir, "mock-zqk.sh")

	// Plan must carry persona_refs (lifecycle / CAP dispatch identity) — no all-persona fallback.
	//
	script := `#!/bin/bash
echo "$@" >> ` + tempDir + `/calls.log

` + fmt.Sprintf(`if [[ "$1" == "object" && "$2" == "show" ]]; then
	cat <<EOF
{"%s":"PLN-TEST","%s":["PER-TPM","PER-CODER"]}
EOF
	exit 0
fi
`, objects.FieldKeyID, objects.FieldKeyPersonaRefs) + `

	# Simulate a delay for agent orchestrate to ensure concurrency works without blocking
if [[ "$1" == "agent" && "$2" == "orchestrate" ]]; then
    sleep 0.1
fi

exit 0
`
	if err := fileutil.WriteFile(mockExe, []byte(script), paths.DirPerm755); err != nil {
		t.Fatalf("failed to write mock exe: %v", err)
	}

	mStorage := &mockCapStorage{
		created: make([]map[string]any, 0),
		listRes: []map[string]any{
			{
				objects.FieldKeyID:                "PER-TPM",
				objects.FieldKeyAgentSkillRefs:    []any{"ASK-TPM"},
				objects.FieldKeyRelatedObjectRefs: []any{},
			},
			{
				objects.FieldKeyID:                "PER-CODER",
				objects.FieldKeyAgentSkillRefs:    []any{"ASK-CODER"},
				objects.FieldKeyRelatedObjectRefs: []any{},
			},
			{
				objects.FieldKeyID:     "ASK-TPM",
				objects.FieldKeyStatus: objects.ObjectStatusApproved,
			},
			{
				objects.FieldKeyID:     "ASK-CODER",
				objects.FieldKeyStatus: objects.ObjectStatusApproved,
			},
		},
	}
	h := NewCapOrchestratorHandler(mStorage, tempDir, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))

	err := h.(*CapOrchestratorHandler).executeDispatchStage(context.Background(), mockExe, "PLN-TEST", "cap_stage_dispatch", nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	callsBytes, err := fileutil.ReadFile(filepath.Join(tempDir, "calls.log"))
	if err != nil {
		t.Fatalf("failed to read calls log: %v", err)
	}

	calls := string(callsBytes)

	if strings.Contains(calls, "object list persona") {
		t.Errorf("must not fall back to listing all personas, got:\n%s", calls)
	}

	// Verify it dispatched TPM to storage
	foundTPM := false
	createdObjs := mStorage.getCreated()
	for _, obj := range createdObjs {
		if obj["persona_id"] == "PER-TPM" && obj["plan_id"] == "PLN-TEST" {
			foundTPM = true
		}
	}
	if !foundTPM {
		t.Errorf("Expected TPM dispatch via storage")
	}

	// Verify it dispatched CODER to storage
	foundCoder := false
	for _, obj := range createdObjs {
		if obj["persona_id"] == "PER-CODER" && obj["plan_id"] == "PLN-TEST" {
			foundCoder = true
		}
	}
	if !foundCoder {
		t.Errorf("Expected CODER dispatch via storage")
	}
}

func TestCapOrchestratorHandler_executeDispatchStage_SkipsWithoutDispatchIdentity(t *testing.T) {
	tempDir := t.TempDir()
	mockExe := filepath.Join(tempDir, "mock-zqk.sh")
	script := `#!/bin/bash
if [[ "$1" == "object" && "$2" == "show" ]]; then
	cat <<EOF
{"id":"PLN-BARE"}
EOF
	exit 0
fi
exit 1
`
	if err := fileutil.WriteFile(mockExe, []byte(script), paths.DirPerm755); err != nil {
		t.Fatalf("write mock: %v", err)
	}
	mStorage := &mockCapStorage{created: make([]map[string]any, 0)}
	h := NewCapOrchestratorHandler(mStorage, tempDir, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))
	if err := h.(*CapOrchestratorHandler).executeDispatchStage(context.Background(), mockExe, "PLN-BARE", "cap_stage_design", nil, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n := len(mStorage.getCreated()); n != 0 {
		t.Fatalf("expected 0 AGI creates without team/persona_refs, got %d", n)
	}
}

func TestCapOrchestratorHandler_executeDispatchStage_Routed(t *testing.T) {
	tempDir := t.TempDir()
	mockExe := filepath.Join(tempDir, "mock-zqk.sh")

	// Create a mock ZQK binary that returns a priority plan with workstreams.
	// Handler invokes: object show <planID> --format json
	script := `#!/bin/bash
if [[ "$1" == "object" && "$2" == "show" && "$3" == "PLN-TEST-ROUTED" ]]; then
	cat <<EOF
{
  "id": "PLN-TEST-ROUTED",
  "workstreams": [
    {
      "id": "WS-1",
      "tasks": [
        {"id": "TSK-1", "tags": ["vocabulary_scheme:coder"]},
        {"id": "TSK-2", "tags": ["vocabulary_scheme:qa"]}
      ]
    }
  ]
}
EOF
	exit 0
fi
exit 1
`
	if err := fileutil.WriteFile(mockExe, []byte(script), paths.DirPerm755); err != nil {
		t.Fatalf("failed to write mock exe: %v", err)
	}

	mStorage := &mockCapStorage{created: make([]map[string]any, 0)}
	h := NewCapOrchestratorHandler(mStorage, tempDir, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))

	err := h.(*CapOrchestratorHandler).executeDispatchStage(context.Background(), mockExe, "PLN-TEST-ROUTED", "cap_stage_dispatch", nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	createdObjsRouted := mStorage.getCreated()
	for _, obj := range createdObjsRouted {
		if obj[objects.FieldKeyKind] == objects.KindAgentTask {
			t.Fatalf("CAP must not mint agent_task; got %v", obj)
		}
	}
	if len(createdObjsRouted) != 0 {
		t.Fatalf("expected 0 creates on routed dispatch (wake-only), got %d: %v", len(createdObjsRouted), createdObjsRouted)
	}
}

type mockCapStorage struct {
	storagepkg.ObjectStorageProvider
	mu      sync.Mutex
	created []map[string]any
	updated []map[string]any
	listRes []map[string]any
	listed  map[string][]map[string]any
	byID    map[string]map[string]any
}

func (m *mockCapStorage) Create(ctx context.Context, secCtx *storagepkg.SecurityContext, obj map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.created = append(m.created, obj)
	return nil
}

func (m *mockCapStorage) Update(ctx context.Context, secCtx *storagepkg.SecurityContext, id string, obj map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updated = append(m.updated, obj)
	return nil
}

func (m *mockCapStorage) Read(ctx context.Context, secCtx *storagepkg.SecurityContext, id string) (map[string]any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.byID != nil {
		if o, ok := m.byID[id]; ok {
			return o, nil
		}
	}
	for _, objs := range m.listed {
		for _, o := range objs {
			if oid, _ := o[objects.FieldKeyID].(string); oid == id {
				return o, nil
			}
		}
	}
	for _, o := range m.listRes {
		if oid, _ := o[objects.FieldKeyID].(string); oid == id {
			return o, nil
		}
	}
	for _, o := range m.created {
		if oid, _ := o[objects.FieldKeyID].(string); oid == id {
			return o, nil
		}
	}
	return nil, errfmt.Errorf("mockCapStorage: not found %s", id)
}

func (m *mockCapStorage) List(ctx context.Context, secCtx *storagepkg.SecurityContext, storageCtx *storagepkg.StorageContext, filter storagepkg.ListFilter) (*storagepkg.QueryResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.listed != nil {
		if objs, ok := m.listed[filter.Kind]; ok {
			return &storagepkg.QueryResult{Objects: objs}, nil
		}
	}
	return &storagepkg.QueryResult{
		Objects: m.listRes,
	}, nil
}

func (m *mockCapStorage) getCreated() []map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := make([]map[string]any, len(m.created))
	copy(res, m.created)
	return res
}

func TestCapOrchestratorHandler_autoRecoverPlanTasks(t *testing.T) {
	tempDir := t.TempDir()

	// Create mock storage with an errored task
	mStorage := &mockCapStorage{
		created: make([]map[string]any, 0),
		updated: make([]map[string]any, 0),
		listRes: []map[string]any{
			{
				objects.FieldKeyID:     "ATK-1",
				objects.FieldKeyTitle:  "Errored Task 1",
				objects.FieldKeyStatus: objects.ObjectStatusError,
				objects.FieldKeyTaskSteps: []any{
					map[string]any{
						objects.FieldKeyStatus:               objects.ObjectStatusError,
						objects.FieldKeyVerificationFeedback: "Failed test suite",
					},
					map[string]any{
						objects.FieldKeyStatus:               objects.ObjectStatusRejected,
						objects.FieldKeyVerificationFeedback: "Check rejected",
					},
					map[string]any{
						objects.FieldKeyStatus: objects.ObjectStatusComplete,
					},
				},
			},
		},
	}

	h := NewCapOrchestratorHandler(mStorage, tempDir, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))

	err := h.(*CapOrchestratorHandler).autoRecoverPlanTasks(context.Background(), "PLN-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mStorage.mu.Lock()
	updatedLen := len(mStorage.updated)
	mStorage.mu.Unlock()

	if updatedLen != 1 {
		t.Fatalf("expected 1 task updated/recovered, got %d", updatedLen)
	}

	updatedObj := mStorage.updated[0]
	if status, _ := updatedObj[objects.FieldKeyStatus].(string); status != objects.ObjectStatusApproved {
		t.Errorf("expected recovered status to be approved, got %s", status)
	}

	steps, ok := updatedObj[objects.FieldKeyTaskSteps].([]any)
	if !ok {
		t.Fatalf("expected task_steps to be present")
	}

	step1 := steps[0].(map[string]any)
	if status, _ := step1[objects.FieldKeyStatus].(string); status != "pending_verification" {
		t.Errorf("expected step 1 status to be pending_verification, got %s", status)
	}
	if feedback, _ := step1[objects.FieldKeyVerificationFeedback].(string); feedback != "" {
		t.Errorf("expected step 1 feedback to be cleared, got %s", feedback)
	}

	step2 := steps[1].(map[string]any)
	if status, _ := step2[objects.FieldKeyStatus].(string); status != "pending_verification" {
		t.Errorf("expected step 2 status to be pending_verification, got %s", status)
	}
}

func TestResumePendingVerificationTasksRequiresCommitEvidence(t *testing.T) {
	t.Parallel()

	mStorage := &mockCapStorage{listed: map[string][]map[string]any{
		objects.KindAgentTask: {
			{
				objects.FieldKeyID:              "ATK-pending",
				objects.FieldKeyStatus:          objects.ObjectStatusPendingVerification,
				objects.FieldKeyPriorityPlanRef: "PRI-CAP",
			},
		},
	}}
	h := NewCapOrchestratorHandler(mStorage, "", logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*CapOrchestratorHandler)

	err := h.resumePendingVerificationTasks(context.Background(), "", "PRI-CAP")
	if err == nil || !strings.Contains(err.Error(), "no durable commit evidence") {
		t.Fatalf("expected missing commit evidence error, got %v", err)
	}
}

func TestMintAgentTaskID(t *testing.T) {
	t.Parallel()

	id := mintAgentTaskID()
	if !strings.HasPrefix(id, "ATK-") || len(id) < len("ATK-1") {
		t.Fatalf("mintAgentTaskID() = %q, want ATK-prefixed id", id)
	}
}

func TestCapOrchestratorHandler_hasOpenTaskForPlan(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

	mStorage := &mockCapStorage{
		listRes: []map[string]any{
			{
				objects.FieldKeyKind:   objects.KindAgentTask,
				objects.FieldKeyStatus: objects.ObjectStatusProposed,
				"plan_id":              "PLN-GROOM",
				"task_id":              "BLI-101",
			},
		},
	}

	h := NewCapOrchestratorHandler(mStorage, tempDir, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*CapOrchestratorHandler)

	if !h.hasOpenTaskForPlan(context.Background(), "PLN-GROOM", "BLI-101") {
		t.Errorf("expected open task to be detected for PLN-GROOM / BLI-101")
	}

	if h.hasOpenTaskForPlan(context.Background(), "PLN-GROOM", "BLI-999") {
		t.Errorf("did not expect open task for BLI-999")
	}
}

// CAP mints agent_task at the preliminary `proposed` origin, so its own tasks live on
// the draft plane, which List omits. A CAS-only dedupe re-minted every task each tick.
func TestCapOrchestratorHandler_hasOpenTaskForPlan_DraftPlane(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

	const draftID = "ATK-DRAFT-0001"
	draftPath := storagepkg.ObjectDraftPlanePath(tempDir, objects.KindAgentTask, draftID)
	if err := fileutil.MkdirAll(filepath.Dir(draftPath), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir draft plane: %v", err)
	}
	if err := fileutil.WriteFile(draftPath, []byte("id: "+draftID+"\n"), paths.FilePerm644); err != nil {
		t.Fatalf("write draft: %v", err)
	}

	// CAS list is empty: the only record of this task is on the draft plane.
	mStorage := &mockCapStorage{
		byID: map[string]map[string]any{
			draftID: {
				objects.FieldKeyID:     draftID,
				objects.FieldKeyKind:   objects.KindAgentTask,
				objects.FieldKeyStatus: objects.ObjectStatusProposed,
				capFieldPlanID:         "PLN-GROOM",
				capFieldTaskID:         "BLI-101",
			},
		},
	}

	h := NewCapOrchestratorHandler(mStorage, tempDir, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*CapOrchestratorHandler)

	if !h.hasOpenTaskForPlan(context.Background(), "PLN-GROOM", "BLI-101") {
		t.Errorf("expected draft-plane task to be detected for PLN-GROOM / BLI-101")
	}
	if !h.hasOpenTaskForPlan(context.Background(), "", "BLI-101") {
		t.Errorf("expected empty planID to match the task under any plan")
	}
	if h.hasOpenTaskForPlan(context.Background(), "PLN-GROOM", "BLI-999") {
		t.Errorf("did not expect open task for BLI-999")
	}
}

func TestCapOrchestratorHandler_executeMetricsStage(t *testing.T) {
	tempDir := t.TempDir()
	mockExe := filepath.Join(tempDir, "mock-zqk.sh")

	script := `#!/bin/bash
if [[ "$1" == "workflow" && "$2" == "whats-next" ]]; then
	cat <<EOF
{"priority_plan": "PRI-TEST-ALIGN"}
EOF
	exit 0
fi

if [[ "$1" == "system" && "$2" == "metrics" && "$3" == "--summary" ]]; then
	cat <<EOF
{"improvement_suggestions": ["fix the tests", "do something else"]}
EOF
	exit 0
fi

if [[ "$1" == "feed" && "$2" == "steer" ]]; then
    found_await=0
    for arg in "$@"; do
        if [[ "$arg" == "--await-peer-ack" ]]; then
            found_await=1
        fi
    done
    if [[ $found_await -eq 0 ]]; then
        echo "Missing --await-peer-ack" >&2
        exit 1
    fi
    echo '{"id":"AFE-MOCK-123","status":"delivered"}'
    exit 0
fi

# Ignore other commands like object count, system metrics --filter, object list
echo "{}"
exit 0
`
	if err := fileutil.WriteFile(mockExe, []byte(script), paths.DirPerm755); err != nil {
		t.Fatalf("failed to write mock exe: %v", err)
	}

	mStorage := &mockCapStorage{created: make([]map[string]any, 0)}
	h := NewCapOrchestratorHandler(mStorage, tempDir, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))

	err := h.(*CapOrchestratorHandler).executeMetricsStage(context.Background(), mockExe)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	createdObjs := mStorage.getCreated()
	foundReport := false
	for _, obj := range createdObjs {
		if obj[objects.FieldKeyKind] == "metrics_report" {
			foundReport = true
			metrics, ok := obj[objects.FieldKeyMetrics].(map[string]any)
			if !ok {
				t.Fatalf("expected metrics map")
			}

			if metrics["align_pointer"] != "PRI-TEST-ALIGN" {
				t.Errorf("expected align_pointer=PRI-TEST-ALIGN, got %v", metrics["align_pointer"])
			}
			if metrics["next_admin_action"] != "fix the tests" {
				t.Errorf("expected next_admin_action='fix the tests', got %v", metrics["next_admin_action"])
			}
		}
	}
	if !foundReport {
		t.Errorf("Expected metrics_report creation")
	}
}

func TestDecodeCAPPlanOutputIgnoresTrailingLogs(t *testing.T) {
	t.Parallel()
	output := []byte("{\"id\":\"PRI-1\",\"status\":\"active\"}\nDEBUG: audit side effect")
	var got map[string]any
	if err := decodeCAPPlanOutput(output, &got); err != nil {
		t.Fatalf("decode CAP plan output: %v", err)
	}
	if got[objects.FieldKeyID] != "PRI-1" || got[objects.FieldKeyStatus] != "active" {
		t.Fatalf("unexpected decoded plan: %#v", got)
	}
}

func TestDecodeCAPPlanOutputRejectsMalformedModelOutput(t *testing.T) {
	t.Parallel()

	var got map[string]any
	if err := decodeCAPPlanOutput([]byte("not-json"), &got); err == nil {
		t.Fatal("malformed model output must fail closed")
	}
}

func TestCAPDispatchPlanIDsPrimaryFirst(t *testing.T) {
	t.Parallel()
	plans := []whatsnext.WhatsNextPriorityPlan{{ID: "PRI-STALE"}, {ID: "PRI-PRIMARY"}, {ID: "PRI-OTHER"}}
	got := capDispatchPlanIDs("PRI-PRIMARY", plans)
	want := []string{"PRI-PRIMARY", "PRI-STALE", "PRI-OTHER"}
	if len(got) != len(want) {
		t.Fatalf("unexpected plan count: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected dispatch order: got %v want %v", got, want)
		}
	}
}

func TestNestPlanWorkstreamsIncludesUnassignedBacklog(t *testing.T) {
	t.Parallel()
	const (
		planID = "PRI-PRIMARY"
		bliID  = "BLI-UNASSIGNED"
	)
	storage := &mockCapStorage{listed: map[string][]map[string]any{
		objects.KindWorkstream: {},
		objects.KindBacklogItem: {
			{
				objects.FieldKeyID:              bliID,
				objects.FieldKeyPriorityPlanRef: planID,
			},
		},
	}}
	handler := NewCapOrchestratorHandler(
		storage,
		t.TempDir(),
		logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	).(*CapOrchestratorHandler)

	plan := map[string]any{objects.FieldKeyID: planID}
	backlog := handler.nestPlanWorkstreams(context.Background(), planID, plan)
	if _, ok := backlog[bliID]; !ok {
		t.Fatalf("expected unassigned backlog item %s in plan map: %#v", bliID, backlog)
	}
	workstreams, ok := plan["workstreams"].([]any)
	if !ok || len(workstreams) != 1 {
		t.Fatalf("expected synthetic unassigned workstream, got %#v", plan["workstreams"])
	}
	unassigned, _ := workstreams[0].(map[string]any)
	if unassigned[objects.FieldKeyID] != capUnassignedWorkstreamID {
		t.Fatalf("unexpected synthetic workstream: %#v", unassigned)
	}
	tasks, ok := unassigned["tasks"].([]any)
	if !ok || len(tasks) != 1 {
		t.Fatalf("expected unassigned task in synthetic workstream, got %#v", unassigned["tasks"])
	}
}

func TestExecuteDispatchStage_LeadEmptyColumnIgnoresCompleteShovelReady(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	mockExe := filepath.Join(tempDir, "mock-zqk.sh")
	script := `#!/bin/bash
if [[ "$1" == "object" && "$2" == "show" ]]; then
	cat <<'EOF'
{"id":"PRI-EMPTY","persona_refs":["PER-TPM"]}
EOF
	exit 0
fi
exit 0
`
	if err := fileutil.WriteFile(mockExe, []byte(script), paths.DirPerm755); err != nil {
		t.Fatalf("write mock: %v", err)
	}
	mStorage := &mockCapStorage{
		listed: map[string][]map[string]any{
			objects.KindWorkstream: {},
			objects.KindBacklogItem: {
				{
					objects.FieldKeyID:              "BLI-DONE",
					objects.FieldKeyStatus:          objects.ObjectStatusComplete,
					objects.FieldKeyPriorityPlanRef: "PRI-EMPTY",
					objects.FieldKeyRequirementRefs: []any{"REQ-1"},
					objects.FieldKeyCriteriaRefs:    []any{"CRIT-1"},
					objects.FieldKeyEstimatedEffort: "1d",
					objects.FieldKeyPersonaRefs:     []any{"PER-CODER"},
				},
			},
		},
	}
	h := NewCapOrchestratorHandler(mStorage, tempDir, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*CapOrchestratorHandler)
	err := h.executeDispatchStage(context.Background(), mockExe, "PRI-EMPTY", "cap_stage_dispatch", nil, true)
	if err == nil || !strings.Contains(err.Error(), "ATTN empty-column") {
		t.Fatalf("expected ATTN empty-column when only completes remain, got %v", err)
	}
}

func TestExecuteDispatchStage_LeadDorGapWhenOpenNotShovelReady(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	mockExe := filepath.Join(tempDir, "mock-zqk.sh")
	script := `#!/bin/bash
if [[ "$1" == "object" && "$2" == "show" ]]; then
	cat <<'EOF'
{"id":"PRI-DOR","persona_refs":["PER-TPM"]}
EOF
	exit 0
fi
exit 0
`
	if err := fileutil.WriteFile(mockExe, []byte(script), paths.DirPerm755); err != nil {
		t.Fatalf("write mock: %v", err)
	}
	mStorage := &mockCapStorage{
		listed: map[string][]map[string]any{
			objects.KindWorkstream: {},
			objects.KindBacklogItem: {
				{
					objects.FieldKeyID:              "BLI-OPEN-GAP",
					objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
					objects.FieldKeyPriorityPlanRef: "PRI-DOR",
					// Missing requirement_refs / criteria / persona → not CRI-SHOVEL-READY.
				},
			},
		},
	}
	h := NewCapOrchestratorHandler(mStorage, tempDir, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*CapOrchestratorHandler)
	err := h.executeDispatchStage(context.Background(), mockExe, "PRI-DOR", "cap_stage_dispatch", nil, true)
	if err == nil || !strings.Contains(err.Error(), "ATTN dor-gap") {
		t.Fatalf("expected ATTN dor-gap when open BLIs fail DoR, got %v", err)
	}
}

func TestExecuteDispatchStage_LeadTerminalStatusReturnsNil(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	mockExe := filepath.Join(tempDir, "mock-zqk.sh")
	script := fmt.Sprintf(`#!/bin/bash
if [[ "$1" == "object" && "$2" == "show" ]]; then
	cat <<'EOF'
{"%s":"PRI-DONE-PLAN","%s":"complete","%s":["PER-TPM"]}
EOF
	exit 0
fi
exit 0
`, objects.FieldKeyID, objects.FieldKeyStatus, objects.FieldKeyPersonaRefs)
	if err := fileutil.WriteFile(mockExe, []byte(script), paths.DirPerm755); err != nil {
		t.Fatalf("write mock: %v", err)
	}
	mStorage := &mockCapStorage{
		listed: map[string][]map[string]any{
			objects.KindWorkstream: {},
			objects.KindBacklogItem: {
				{
					objects.FieldKeyID:              "BLI-1",
					objects.FieldKeyStatus:          objects.ObjectStatusComplete,
					objects.FieldKeyPriorityPlanRef: "PRI-DONE-PLAN",
				},
			},
		},
	}
	h := NewCapOrchestratorHandler(mStorage, tempDir, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*CapOrchestratorHandler)
	err := h.executeDispatchStage(context.Background(), mockExe, "PRI-DONE-PLAN", "cap_stage_dispatch", nil, true)
	if err != nil {
		t.Fatalf("expected nil error for complete priority plan, got: %v", err)
	}
}

func TestCapOrchestratorHandler_IdleGroomingWakesTPM(t *testing.T) {
	tempDir := t.TempDir()
	t.Cleanup(func() {
		whatsnext.WaitForReconcile(2 * time.Second)
	})

	mockZqk := filepath.Join(tempDir, "zqk")
	script := `#!/bin/bash
if [[ "$1" == "workflow" && "$2" == "whats-next" ]]; then
	cat <<EOF
{"agent_instruction":"shutdown","active_plans":[{"id":"PRI-GROOM-1","status":"grooming","title":"Grooming Cycle"}]}
EOF
	exit 0
fi
exit 1
`
	if err := fileutil.WriteFile(mockZqk, []byte(script), paths.DirPerm755); err != nil {
		t.Fatalf("failed to write mock zqk: %v", err)
	}

	oldPath := zqkenv.OSPath().Get()
	t.Setenv(zqkenv.OSPath().Name(), tempDir+string(fileutil.PathListSeparator)+oldPath)

	storage := &mockStorage{data: map[string]map[string]any{
		"PRI-GROOM-1": {
			objects.FieldKeyID:     "PRI-GROOM-1",
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyStatus: objects.ObjectStatusGrooming,
			objects.FieldKeyTitle:  "Grooming Cycle",
		},
	}}
	h := NewCapOrchestratorHandler(storage, tempDir, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))

	job := &ScheduledJob{
		ID:      "SCH-CAP-GROOM",
		JobType: "cap_orchestrator",
	}

	err := h.Execute(context.Background(), job)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	eventPath := datacell.AgentChatChannelEventsJSONLPath(tempDir)
	raw, err := fileutil.ReadFile(eventPath)
	if err != nil {
		t.Fatalf("expected agent_chat_channel events file to exist: %v", err)
	}
	content := string(raw)
	if !strings.Contains(content, "PRI-GROOM-1") || !strings.Contains(content, "tpm") {
		t.Fatalf("expected wake event for PRI-GROOM-1 and tpm in %s, got: %s", eventPath, content)
	}
}

