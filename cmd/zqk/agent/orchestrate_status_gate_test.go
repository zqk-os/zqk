package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type mockStatusGateStorage struct {
	storage.ObjectStorageProvider
	objs map[string]map[string]any
}

func (m *mockStatusGateStorage) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if obj, ok := m.objs[id]; ok {
		return obj, nil
	}
	return nil, errfmt.Errorf("not found: %s", id)
}

func TestTriggerPlanOrchestration_RefusesGroomingAndComplete(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := fileutil.MkdirAll(binDir, paths.DirPerm750); err != nil {
		t.Fatal(err)
	}
	zqkPath := filepath.Join(binDir, "zqk")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$0.args\"\n"
	if err := fileutil.WriteFile(zqkPath, []byte(script), paths.DirPerm700); err != nil {
		t.Fatal(err)
	}

	mockStore := &mockStatusGateStorage{
		objs: map[string]map[string]any{
			"PRI-GROOMING-001": {
				objects.FieldKeyKind:   objects.KindPriorityPlan,
				objects.FieldKeyStatus: objects.ObjectStatusGrooming,
				objects.FieldKeyTitle:  "Grooming Plan",
			},
			"PRI-COMPLETE-001": {
				objects.FieldKeyKind:   objects.KindPriorityPlan,
				objects.FieldKeyStatus: objects.ObjectStatusComplete,
				objects.FieldKeyTitle:  "Complete Plan",
			},
			"PRI-ACTIVE-001": {
				objects.FieldKeyKind:   objects.KindPriorityPlan,
				objects.FieldKeyStatus: objects.ObjectStatusActive,
				objects.FieldKeyTitle:  "Active Plan",
			},
			"PRI-IN-PROG-001": {
				objects.FieldKeyKind:   objects.KindPriorityPlan,
				objects.FieldKeyStatus: objects.ObjectStatusInProgress,
				objects.FieldKeyTitle:  "In Progress Plan",
			},
		},
	}

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. Grooming plan must refuse and return error without submitting
	_, errGrooming := triggerPlanOrchestration(ctx, mockStore, secCtx, root, "PRI-GROOMING-001")
	if errGrooming == nil {
		t.Fatal("expected triggerPlanOrchestration to fail for grooming plan, got nil")
	}
	if !strings.Contains(errGrooming.Error(), "orchestrator can only operate on active or in_progress priority plans (got 'grooming')") {
		t.Fatalf("unexpected error message for grooming: %v", errGrooming)
	}
	if _, err := fileutil.Stat(zqkPath + ".args"); err == nil {
		t.Fatal("zqk args file should not exist after refused grooming plan")
	}

	// 2. Complete plan must refuse and return error without submitting
	_, errComplete := triggerPlanOrchestration(ctx, mockStore, secCtx, root, "PRI-COMPLETE-001")
	if errComplete == nil {
		t.Fatal("expected triggerPlanOrchestration to fail for complete plan, got nil")
	}
	if !strings.Contains(errComplete.Error(), "orchestrator can only operate on active or in_progress priority plans (got 'complete')") {
		t.Fatalf("unexpected error message for complete: %v", errComplete)
	}
	if _, err := fileutil.Stat(zqkPath + ".args"); err == nil {
		t.Fatal("zqk args file should not exist after refused complete plan")
	}

	// 3. Active plan must succeed and submit
	outActive, errActive := triggerPlanOrchestration(ctx, mockStore, secCtx, root, "PRI-ACTIVE-001")
	if errActive != nil {
		t.Fatalf("expected triggerPlanOrchestration to succeed for active plan: %v", errActive)
	}
	if outActive == "" {
		t.Log("outActive is empty, but err is nil")
	}
	gotArgs, readErr := fileutil.ReadFile(zqkPath + ".args")
	if readErr != nil {
		t.Fatalf("failed to read zqk args for active plan: %v", readErr)
	}
	if !strings.Contains(string(gotArgs), "agent orchestrate PRI-ACTIVE-001") {
		t.Fatalf("expected args to contain 'agent orchestrate PRI-ACTIVE-001', got: %s", string(gotArgs))
	}
}

func TestHandleNonCommsWithAgentX_AcksRefusedStatusCleanly(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := datacell.WriteAgentChatChannelConfig(root, datacell.AgentChatChannelConfig{
		SchemaVersion: datacell.AgentChatChannelSchemaVersion,
		Enabled:       true,
		DeliveryMode:  datacell.DeliveryModeLog,
		FeedID:        "AGF-test-status-gate",
	}); err != nil {
		t.Fatal(err)
	}
	mockStore := &mockStatusGateStorage{
		objs: map[string]map[string]any{
			"PRI-GROOMING-002": {
				objects.FieldKeyKind:   objects.KindPriorityPlan,
				objects.FieldKeyStatus: objects.ObjectStatusGrooming,
				objects.FieldKeyTitle:  "Grooming Plan 2",
			},
		},
	}

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	steer, err := agentfeed.AppendEvent(agentfeed.AppendEventInput{
		ProjectRoot: root,
		Message:     "ORCHESTRATE_PLAN PRI-GROOMING-002",
		AgentID:     "coordinator-1",
		ToAgentID:   "peer-agent-1",
		Sender:      agentfeed.FeedSenderHumanSteer,
		EventType:   agentfeed.FeedEventTypeSteering,
		SelfACK:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	item := agentfeed.CorrespondenceItem{
		EventID: steer.EventID,
	}

	err = handleNonCommsWithAgentX(
		ctx,
		nil,
		mockStore,
		secCtx,
		root,
		"peer-agent-1",
		"PER-ORCH-ALPHA",
		"session-1",
		item,
		"ORCHESTRATE_PLAN PRI-GROOMING-002",
	)
	if err != nil {
		t.Fatalf("handleNonCommsWithAgentX must ack skip cleanly without returning error, got: %v", err)
	}
}
