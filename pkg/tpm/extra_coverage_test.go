package tpm

import (
	"context"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestDetectDomain_AllCases(t *testing.T) {
	tests := []struct {
		id       string
		title    string
		expected string
	}{
		{"REQ-STORAGE-001", "CAS Git lock", "storage"},
		{"REQ-SCHED-001", "daemon cron retention", "scheduler"},
		{"REQ-MESH-001", "mcp agent feed", "mesh_agent"},
		{"REQ-TPM-001", "cap orchestrate whats-next plan", "tpm_orchestration"},
		{"REQ-GOV-001", "policy audit governance doc", "governance"},
		{"REQ-PERF-001", "metric timeseries cache", "performance"},
		{"REQ-QA-001", "test trace attest criteria", "qa_traceability"},
		{"REQ-OTHER-001", "unclassified entity", "core_kernel"},
	}

	for _, tt := range tests {
		got := DetectDomain(tt.id, tt.title)
		if got != tt.expected {
			t.Errorf("DetectDomain(%q, %q) = %q; want %q", tt.id, tt.title, got, tt.expected)
		}
	}
}

func TestApplyEventAndLifecycle(t *testing.T) {
	tempDir := t.TempDir()
	view := NewStrategicReadinessView(tempDir)

	// Add requirement and link criterion
	critID := "CRIT-TEST-001"
	reqID := "REQ-TEST-001"
	planID := "PRI-TEST-001"

	view.critToReqs[critID] = []string{reqID}
	view.requirements[reqID] = &RequirementReadinessNode{
		ID:              reqID,
		Status:          objects.ObjectStatusOpen,
		PriorityPlanRef: planID,
		CriteriaRefs:    []string{critID},
	}
	view.planStatuses[planID] = "in_progress"

	// Nil event
	if !view.ApplyEvent(nil) {
		t.Fatal("expected true for nil event")
	}

	// Criterion satisfied event
	critEv := &lifecycle.LifecycleEvent{
		EventType:   lifecycle.EventTypeCriterionSatisfied,
		CriterionID: critID,
		Ts:          time.Now(),
	}
	view.ApplyEvent(critEv)

	// Requirement transition
	reqEv := &lifecycle.LifecycleEvent{
		EventType:  lifecycle.EventTypeStatusTransition,
		Kind:       objects.KindRequirement,
		ID:         reqID,
		FromStatus: "open",
		ToStatus:   "active",
	}
	view.ApplyEvent(reqEv)

	// Priority plan transition
	planEv := &lifecycle.LifecycleEvent{
		EventType:  lifecycle.EventTypeStatusTransition,
		Kind:       objects.KindPriorityPlan,
		ID:         planID,
		FromStatus: "in_progress",
		ToStatus:   "complete",
	}
	view.ApplyEvent(planEv)

	// Verify events were appended
	view.mu.RLock()
	defer view.mu.RUnlock()
	if len(view.recentEvents) < 3 {
		t.Fatalf("expected at least 3 recent events, got %d", len(view.recentEvents))
	}
}

func TestSaveToLiteFile_NilEngine(t *testing.T) {
	tempDir := t.TempDir()
	view := &StrategicReadinessView{
		projectRoot:         tempDir,
		requirements:        make(map[string]*RequirementReadinessNode),
		planStatuses:        make(map[string]string),
		domainClusters:      make(map[string]*DomainCluster),
		synthesisCandidates: make([]*SynthesisCandidate, 0),
		recentEvents:        make([]StrategicLifecycleEventSummary, 0),
	}

	view.requirements["REQ-001"] = &RequirementReadinessNode{
		ID:             "REQ-001",
		Status:         objects.ObjectStatusActive,
		IsFullyAligned: true,
	}

	if err := view.SaveToLiteFile(); err != nil {
		t.Fatalf("SaveToLiteFile with nil engine failed: %v", err)
	}

	// Verify file exists
	loaded, err := view.LoadFromLiteFile()
	if err != nil {
		t.Fatalf("LoadFromLiteFile failed: %v", err)
	}
	if !loaded {
		t.Fatal("expected loaded to be true")
	}
}

func TestSubscribeWAL_Cancel(t *testing.T) {
	tempDir := t.TempDir()
	view := NewStrategicReadinessView(tempDir)

	ctx, cancel := context.WithCancel(context.Background())
	updateCh := make(chan struct{}, 1)

	// Launch background subscriber
	view.StartBackgroundWALSubscriber(ctx, updateCh)

	// Immediate cancel to test graceful exit
	cancel()
	time.Sleep(50 * time.Millisecond)
}
