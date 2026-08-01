package scheduler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/primaryorch"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
)

func TestCapOrchestratorHandler_Execute(t *testing.T) {
	tempDir := t.TempDir()

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
	if err := os.WriteFile(mockZqk, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write mock zqk: %v", err)
	}

	// Override PATH so resolveCLIExecutable finds our mock zqk
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", tempDir+string(os.PathListSeparator)+oldPath)
	defer os.Setenv("PATH", oldPath)

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
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		t.Fatalf("failed to create scripts dir: %v", err)
	}

	wakeLog := filepath.Join(tempDir, "wake.log")
	wakeScript := filepath.Join(scriptsDir, "wake-agy.sh")
	scriptContent := fmt.Sprintf("#!/bin/bash\necho \"$@\" >> %s\n", wakeLog)
	if err := os.WriteFile(wakeScript, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("failed to write mock wake-agy.sh: %v", err)
	}
	if err := primaryorch.WriteBinding(tempDir, primaryorch.Binding{
		SchemaVersion: primaryorch.SchemaVersion,
		AgentID:       "antigravity",
		Adapter:       primaryorch.AdapterScript,
		Script:        "scripts/wake-agy.sh",
	}); err != nil {
		t.Fatalf("write primary binding: %v", err)
	}

	storage := &mockStorage{data: map[string]map[string]any{
		"PLAN-TEST": {
			objects.FieldKeyID:     "PLAN-TEST",
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyStatus: objects.ObjectStatusActive,
		},
		"ITEM-P0": {
			objects.FieldKeyID:              "ITEM-P0",
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
			objects.FieldKeyPriorityPlanRef: "PLAN-TEST",
			objects.FieldKeyPriorityTier:    "P0",
			objects.FieldKeyTitle:           "Ship gate",
		},
		"ITEM-P1": {
			objects.FieldKeyID:              "ITEM-P1",
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyStatus:          "in_progress",
			objects.FieldKeyPriorityPlanRef: "PLAN-TEST",
			objects.FieldKeyPriorityTier:    "P1",
			objects.FieldKeyTitle:           "Wake payload",
		},
	}}
	h := NewCapOrchestratorHandler(storage, tempDir, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*CapOrchestratorHandler)

	h.wakeAgentAndScheduleHourglass("PLAN-TEST", "persona:tpm")

	if _, err := os.Stat(wakeLog); os.IsNotExist(err) {
		t.Fatalf("expected wake script to be executed and create wake.log")
	}

	logData, _ := os.ReadFile(wakeLog)
	got := string(logData)
	if !strings.Contains(got, "PLAN-TEST") {
		t.Errorf("expected wake log to contain task/plan ID, got: %s", got)
	}
	if !strings.Contains(got, "PRI=PLAN-TEST") {
		t.Errorf("expected PRI= line in wake message, got: %s", got)
	}
	if !strings.Contains(got, "ITEM-P0") || !strings.Contains(got, "Ship gate") {
		t.Errorf("expected top BLI in wake message, got: %s", got)
	}
}

func TestCapOrchestratorHandler_executeDispatchStage(t *testing.T) {
	tempDir := t.TempDir()
	mockExe := filepath.Join(tempDir, "mock-zqk.sh")

	// Create a mock ZQK binary that records its invocations and mocks the persona list.
	// executeDispatchStage calls: object show <planID> --format json (not "object show priority_plan").
	script := `#!/bin/bash
echo "$@" >> ` + tempDir + `/calls.log

` + fmt.Sprintf(`if [[ "$1" == "object" && "$2" == "show" ]]; then
	cat <<EOF
{"%s":"PLN-TEST"}
EOF
	exit 0
fi
if [[ "$1" == "object" && "$2" == "list" && "$3" == "persona" ]]; then
	cat <<EOF
{"objects":[
  {"%s":"PER-TPM", "%s":"technical-program-manager"},
  {"%s":"PER-CODER", "%s":"coder-agent"}
]}
EOF
	exit 0
fi`, objects.FieldKeyID, objects.FieldKeyID, objects.FieldKeyRole, objects.FieldKeyID, objects.FieldKeyRole) + `

	// Simulate a delay for agent orchestrate to ensure concurrency works without blocking
if [[ "$1" == "agent" && "$2" == "orchestrate" ]]; then
    sleep 0.1
fi

exit 0
`
	if err := os.WriteFile(mockExe, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write mock exe: %v", err)
	}

	mStorage := &mockCapStorage{created: make([]map[string]any, 0)}
	h := NewCapOrchestratorHandler(mStorage, tempDir, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))

	err := h.(*CapOrchestratorHandler).executeDispatchStage(context.Background(), mockExe, "PLN-TEST", "cap_stage_dispatch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	callsBytes, err := os.ReadFile(filepath.Join(tempDir, "calls.log"))
	if err != nil {
		t.Fatalf("failed to read calls log: %v", err)
	}

	calls := string(callsBytes)

	// Verify it fetched personas
	if !strings.Contains(calls, "object list persona") {
		t.Errorf("Expected mock to be called with 'object list persona', got:\n%s", calls)
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
	if err := os.WriteFile(mockExe, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write mock exe: %v", err)
	}

	mStorage := &mockCapStorage{created: make([]map[string]any, 0)}
	h := NewCapOrchestratorHandler(mStorage, tempDir, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))

	err := h.(*CapOrchestratorHandler).executeDispatchStage(context.Background(), mockExe, "PLN-TEST-ROUTED", "cap_stage_dispatch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	createdObjsRouted := mStorage.getCreated()
	if len(createdObjsRouted) != 2 {
		t.Fatalf("expected 2 agent_tasks created, got %d", len(createdObjsRouted))
	}

	foundTSK1 := false
	foundTSK2 := false
	for _, obj := range createdObjsRouted {
		if st, _ := obj[objects.FieldKeyStatus].(string); st != objects.ObjectStatusProposed {
			t.Errorf("expected agent_task status %q, got %q for task %v", objects.ObjectStatusProposed, st, obj["task_id"])
		}
		if obj[objects.FieldKeyKind] == objects.KindAgentTask && obj["task_id"] == "TSK-1" && obj["persona_id"] == "persona:coder" {
			foundTSK1 = true
		}
		if obj[objects.FieldKeyKind] == objects.KindAgentTask && obj["task_id"] == "TSK-2" && obj["persona_id"] == "persona:qa" {
			foundTSK2 = true
		}
	}

	if !foundTSK1 || !foundTSK2 {
		t.Errorf("did not find expected agent_task objects. got: %v", createdObjsRouted)
	}
}

type mockCapStorage struct {
	storagepkg.ObjectStorageProvider
	mu      sync.Mutex
	created []map[string]any
	updated []map[string]any
	listRes []map[string]any
	listed  map[string][]map[string]any
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

func TestCapOrchestratorHandler_hasOpenTaskForPlan(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

	mStorage := &mockCapStorage{
		listRes: []map[string]any{
			{
				objects.FieldKeyKind:   objects.KindAgentTask,
				objects.FieldKeyStatus: objects.ObjectStatusProposed,
				"plan_id":              "PLN-GROOM",
				"task_id":              "ITEM-101",
			},
		},
	}

	h := NewCapOrchestratorHandler(mStorage, tempDir, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*CapOrchestratorHandler)

	if !h.hasOpenTaskForPlan(context.Background(), "PLN-GROOM", "ITEM-101") {
		t.Errorf("expected open task to be detected for PLN-GROOM / ITEM-101")
	}

	if h.hasOpenTaskForPlan(context.Background(), "PLN-GROOM", "ITEM-999") {
		t.Errorf("did not expect open task for ITEM-999")
	}
}
