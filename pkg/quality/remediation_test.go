package quality

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/swarm/metabolism"
	"github.com/zqk-os/zqk/pkg/workflow"
)

func TestFindingToRemediationBLI_Success(t *testing.T) {
	finding := metabolism.Finding{
		ID:          "DEFECT-CONCURRENCY-001",
		Lens:        "L-CONCURRENCY",
		Severity:    "E0",
		Title:       "Data Race in Session Cache Lookup",
		Description: "Concurrent map read and write detected by race detector during burst requests.",
		Evidence:    "WARNING: DATA RACE Write at 0x00c00010a088 by goroutine 7",
		Remediation: "Guard session cache read/writes with sync.RWMutex or migrate to sync.Map.",
		Files:       []string{"pkg/session/cache.go"},
	}

	bundle, err := FindingToRemediationBLI(finding, "MLS-DEFECT-REMEDIATION-WAVE1")
	if err != nil {
		t.Fatalf("FindingToRemediationBLI failed: %v", err)
	}

	// 1. Verify Backlog Item fields
	bli := bundle.BacklogItem
	if bli[objects.FieldKeyPriority] != "p0" {
		t.Errorf("expected p0 for E0 severity, got %v", bli[objects.FieldKeyPriority])
	}
	if bli[objects.FieldKeyStatus] != objects.ObjectStatusPlanned {
		t.Errorf("expected planned status, got %v", bli[objects.FieldKeyStatus])
	}
	mlsRefs, _ := bli[objects.FieldKeyMilestoneRefs].([]string)
	if len(mlsRefs) == 0 || mlsRefs[0] != "MLS-DEFECT-REMEDIATION-WAVE1" {
		t.Errorf("unexpected milestone refs: %v", mlsRefs)
	}

	// 2. Verify Three-Fold Criteria & Quality Gate
	req := bundle.Requirement
	criteria := bundle.Criteria
	if len(criteria) != 3 {
		t.Fatalf("expected 3 criteria, got %d", len(criteria))
	}

	if err := workflow.CheckShovelReadyQuality(req, criteria); err != nil {
		t.Fatalf("synthesized remediation failed quality gate: %v", err)
	}

	// 3. Verify Test Case
	tst := bundle.TestCase
	tstCritRefs, _ := tst[objects.FieldKeyCriteriaRefs].([]string)
	if len(tstCritRefs) != 3 {
		t.Errorf("expected test case to link all 3 criteria, got %d", len(tstCritRefs))
	}
}

func TestFindingToRemediationBLI_InvalidFindingFails(t *testing.T) {
	invalid := metabolism.Finding{}
	_, err := FindingToRemediationBLI(invalid, "MLS-1")
	if err == nil {
		t.Fatal("expected error on empty finding, got nil")
	}
}
