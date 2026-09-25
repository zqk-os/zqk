package metabolism

import (
	"bufio"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type mockStreamA struct {
	mu        sync.Mutex
	mutations []map[string]any
}

func (m *mockStreamA) RecordKernelMutation(kind string, id string, data map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := map[string]any{
		"kind": kind,
		"id":   id,
		"data": data,
	}
	m.mutations = append(m.mutations, entry)
	return nil
}

func TestDualStreamRouter_IsolationAndRouting(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "eval_output")
	streamA := &mockStreamA{}

	router, err := NewDualStreamRouter(outDir, streamA)
	if err != nil {
		t.Fatalf("NewDualStreamRouter failed: %v", err)
	}

	// 1. Route findings
	f1 := Finding{
		ID:          "FINDING-001",
		Lens:        "RDB",
		Severity:    "E1",
		Title:       "Missing Mutex in State Registry",
		Description: "Data race detected on concurrent map write.",
		Files:       []string{"pkg/core/registry.go"},
	}
	f2 := Finding{
		ID:          "FINDING-002",
		Lens:        "TST",
		Severity:    "E2",
		Title:       "Low Branch Coverage",
		Description: "Error handling paths lack assertions.",
	}

	if err := router.RouteFinding(f1); err != nil {
		t.Fatalf("RouteFinding f1 failed: %v", err)
	}
	if err := router.RouteFinding(f2); err != nil {
		t.Fatalf("RouteFinding f2 failed: %v", err)
	}

	// Verify findings.jsonl exists and contains 2 lines
	findingsPath := filepath.Join(outDir, "findings.jsonl")
	file, err := fileutil.OpenFile(findingsPath, fileutil.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("failed to open findings.jsonl: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if len(lines) != 2 {
		t.Fatalf("expected 2 findings in findings.jsonl, got %d", len(lines))
	}

	var parsedF1 Finding
	if err := json.Unmarshal([]byte(lines[0]), &parsedF1); err != nil {
		t.Fatalf("failed to parse first finding JSON: %v", err)
	}
	if parsedF1.ID != "FINDING-001" || parsedF1.Severity != "E1" {
		t.Errorf("parsed finding mismatch: %+v", parsedF1)
	}

	// 2. Route scorecard
	sc := Scorecard{
		PackURN:     "urn:zqk:pack:code-eval:1.0.0:abcdef",
		SessionID:   "session-100",
		EnvelopeMin: 4.6,
		Scores: map[string]float64{
			"RDB": 4.8,
			"MNT": 4.6,
		},
		Status: "converged",
	}

	if err := router.RouteScorecard(sc); err != nil {
		t.Fatalf("RouteScorecard failed: %v", err)
	}

	scorecardPath := filepath.Join(outDir, "scorecard.json")
	scData, err := fileutil.ReadFile(scorecardPath)
	if err != nil {
		t.Fatalf("failed to read scorecard.json: %v", err)
	}
	var parsedSC Scorecard
	if err := json.Unmarshal(scData, &parsedSC); err != nil {
		t.Fatalf("failed to parse scorecard: %v", err)
	}
	if parsedSC.EnvelopeMin != 4.6 || parsedSC.Status != "converged" {
		t.Errorf("parsed scorecard mismatch: %+v", parsedSC)
	}

	// 3. Route trace & emit summary
	if err := router.RouteTrace("seismograph.json", []byte(`{"telemetry": true}`)); err != nil {
		t.Fatalf("RouteTrace failed: %v", err)
	}
	tracePath := filepath.Join(outDir, "traces", "seismograph.json")
	if _, err := fileutil.Stat(tracePath); err != nil {
		t.Fatalf("trace file not found: %v", err)
	}

	if err := router.EmitSummary("# Evaluation Report\nAll passed."); err != nil {
		t.Fatalf("EmitSummary failed: %v", err)
	}
	summaryPath := filepath.Join(outDir, "summary.md")
	if _, err := fileutil.Stat(summaryPath); err != nil {
		t.Fatalf("summary file not found: %v", err)
	}

	// 4. Verify Stream A received notifications
	if len(streamA.mutations) < 3 { // 2 findings + 1 scorecard
		t.Errorf("expected Stream A to receive at least 3 mutations, got %d", len(streamA.mutations))
	}
}

func TestDualStreamRouter_InvalidFindingFailsClosed(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "eval_output")
	router, _ := NewDualStreamRouter(outDir, nil)

	invalid := Finding{
		ID: "F-1",
		// Missing Lens, Severity, Title
	}
	err := router.RouteFinding(invalid)
	if err == nil {
		t.Fatal("expected error for invalid finding, got nil")
	}
	if !errors.Is(err, ErrFindingInvalid) {
		t.Errorf("expected ErrFindingInvalid, got %v", err)
	}
}

type errorStreamA struct {
	err error
}

func (e *errorStreamA) RecordKernelMutation(kind string, id string, data map[string]any) error {
	return e.err
}

func TestDualStreamRouter_StreamAErrorsPropagated(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "eval_output")
	simulatedErr := errors.New("simulated kernel write error")
	streamA := &errorStreamA{err: simulatedErr}

	router, err := NewDualStreamRouter(outDir, streamA)
	if err != nil {
		t.Fatalf("NewDualStreamRouter failed: %v", err)
	}

	f := Finding{
		ID:          "FINDING-ERR-1",
		Lens:        "TST",
		Severity:    "E1",
		Title:       "Test Finding",
		Description: "Testing error propagation",
	}

	err = router.RouteFinding(f)
	if err == nil {
		t.Fatal("expected RouteFinding error when StreamA fails, got nil")
	}
	if !errors.Is(err, simulatedErr) {
		t.Errorf("expected wrapped simulatedErr, got %v", err)
	}

	sc := Scorecard{
		PackURN:     "urn:zqk:pack:test:1.0.0",
		SessionID:   "session-err-1",
		EnvelopeMin: 4.0,
		Status:      "passed",
	}

	err = router.RouteScorecard(sc)
	if err == nil {
		t.Fatal("expected RouteScorecard error when StreamA fails, got nil")
	}
	if !errors.Is(err, simulatedErr) {
		t.Errorf("expected wrapped simulatedErr, got %v", err)
	}
}
