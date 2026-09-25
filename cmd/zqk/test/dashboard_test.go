package test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/accumulator"
	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestDashboardState_LoadAndRender(t *testing.T) {
	state := NewDashboardState()

	tc1 := &TestCaseModel{
		ID:     "TST-DEMO-001",
		Title:  "First Verification Suite",
		Status: objects.ObjectStatusActive,
		Criteria: []*CriterionState{
			{ID: "CRIT-001", Status: objects.ObjectStatusComplete},
			{ID: "CRIT-002", Status: objects.ObjectStatusInProgress},
			{ID: "CRIT-003", Status: objects.ObjectStatusOriginated},
		},
		RemainingOpenCount: 2,
		TotalCriteria:      3,
		CompletedCriteria:  1,
	}

	tc2 := &TestCaseModel{
		ID:     "TST-DEMO-002",
		Title:  "Second Verification Suite",
		Status: objects.ObjectStatusComplete,
		Criteria: []*CriterionState{
			{ID: "CRIT-004", Status: objects.ObjectStatusComplete},
			{ID: "CRIT-005", Status: objects.ObjectStatusComplete},
		},
		RemainingOpenCount: 0,
		TotalCriteria:      2,
		CompletedCriteria:  2,
	}

	state.TestCases["TST-DEMO-001"] = tc1
	state.TestCases["TST-DEMO-002"] = tc2
	state.TestCaseOrder = []string{"TST-DEMO-001", "TST-DEMO-002"}
	state.CriteriaIndex["CRIT-001"] = []*TestCaseModel{tc1}
	state.CriteriaIndex["CRIT-002"] = []*TestCaseModel{tc1}
	state.CriteriaIndex["CRIT-003"] = []*TestCaseModel{tc1}
	state.CriteriaIndex["CRIT-004"] = []*TestCaseModel{tc2}
	state.CriteriaIndex["CRIT-005"] = []*TestCaseModel{tc2}

	var buf bytes.Buffer
	if err := state.Render(&buf, false, "all"); err != nil {
		t.Fatalf("render failed: %v", err)
	}

	output := buf.String()

	// Verify header and test case listings
	if !strings.Contains(output, "ZQK TEST MATRIX & CRITERIA RADAR") {
		t.Errorf("expected dashboard title in output")
	}
	if !strings.Contains(output, "TST-DEMO-001") || !strings.Contains(output, "TST-DEMO-002") {
		t.Errorf("expected test cases in output")
	}
	if !strings.Contains(output, "CRIT-001") || !strings.Contains(output, "CRIT-002") {
		t.Errorf("expected criteria pills in output")
	}
	if !strings.Contains(output, "3/5 satisfied") {
		t.Errorf("expected 3/5 satisfied criteria metrics in output, got:\n%s", output)
	}
}

func TestDashboardState_HandleLifecycleEvents(t *testing.T) {
	state := NewDashboardState()

	tc := &TestCaseModel{
		ID:     "TST-AUTORUN-001",
		Title:  "Live Verification",
		Status: objects.ObjectStatusActive,
		Criteria: []*CriterionState{
			{ID: "CRIT-AUTO-1", Status: objects.ObjectStatusInProgress},
			{ID: "CRIT-AUTO-2", Status: objects.ObjectStatusOriginated},
		},
		RemainingOpenCount: 2,
		TotalCriteria:      2,
		CompletedCriteria:  0,
	}

	state.TestCases["TST-AUTORUN-001"] = tc
	state.TestCaseOrder = []string{"TST-AUTORUN-001"}
	state.CriteriaIndex["CRIT-AUTO-1"] = []*TestCaseModel{tc}
	state.CriteriaIndex["CRIT-AUTO-2"] = []*TestCaseModel{tc}

	// 1. Initial render shows 0/2 satisfied
	var buf1 bytes.Buffer
	if err := state.Render(&buf1, false, "all"); err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if !strings.Contains(buf1.String(), "0/2 satisfied") {
		t.Errorf("expected 0/2 satisfied before event, got:\n%s", buf1.String())
	}

	// 2. Dispatch criteria satisfied event
	event := &lifecycle.LifecycleEvent{
		Seq:         1,
		Ts:          time.Now(),
		EventType:   lifecycle.EventTypeCriterionSatisfied,
		CriterionID: "CRIT-AUTO-1",
	}
	state.HandleLifecycleEvent(event)

	// Assert in-memory state mutated correctly
	if tc.Criteria[0].Status != objects.ObjectStatusComplete {
		t.Errorf("expected CRIT-AUTO-1 status complete, got %s", tc.Criteria[0].Status)
	}
	if tc.CompletedCriteria != 1 {
		t.Errorf("expected 1 completed criteria, got %d", tc.CompletedCriteria)
	}
	if tc.RemainingOpenCount != 1 {
		t.Errorf("expected remaining open count 1, got %d", tc.RemainingOpenCount)
	}
	if tc.Status != objects.ObjectStatusActive {
		t.Errorf("expected test case status active, got %s", tc.Status)
	}
	if len(state.RecentEvents) != 1 {
		t.Fatalf("expected 1 recent event recorded, got %d", len(state.RecentEvents))
	}
	if !strings.Contains(state.RecentEvents[0].Message, "CRIT-AUTO-1") {
		t.Errorf("expected event message to mention CRIT-AUTO-1, got %s", state.RecentEvents[0].Message)
	}

	// 2. Simulate second criterion satisfied event -> should cascade complete to test_case
	ev2 := &lifecycle.LifecycleEvent{
		Seq:         2,
		Ts:          time.Now(),
		EventType:   lifecycle.EventTypeCriterionSatisfied,
		CriterionID: "CRIT-AUTO-2",
		Scope:       map[string]string{"test_case_id": "TST-AUTORUN-001"},
	}
	state.HandleLifecycleEvent(ev2)

	if tc.CompletedCriteria != 2 {
		t.Errorf("expected 2 completed criteria, got %d", tc.CompletedCriteria)
	}
	if tc.RemainingOpenCount != 0 {
		t.Errorf("expected remaining open count 0, got %d", tc.RemainingOpenCount)
	}
	if tc.Status != objects.ObjectStatusComplete {
		t.Errorf("expected test case status complete, got %s", tc.Status)
	}
	if len(state.RecentEvents) != 3 {
		t.Fatalf("expected 3 recent events recorded, got %d", len(state.RecentEvents))
	}
	foundGraduated := false
	for _, rev := range state.RecentEvents {
		if strings.Contains(rev.Message, "GRADUATED") {
			foundGraduated = true
			break
		}
	}
	if !foundGraduated {
		t.Errorf("expected graduation event in recent events, got %+v", state.RecentEvents)
	}
}

func TestDashboardState_StatusTransitionEvents(t *testing.T) {
	state := NewDashboardState()

	tc := &TestCaseModel{
		ID:     "TST-TRANS-001",
		Title:  "Status Transition Suite",
		Status: objects.ObjectStatusActive,
		Criteria: []*CriterionState{
			{ID: "CRIT-TRANS-1", Status: objects.ObjectStatusOriginated},
		},
		RemainingOpenCount: 1,
		TotalCriteria:      1,
		CompletedCriteria:  0,
	}

	state.TestCases["TST-TRANS-001"] = tc
	state.TestCaseOrder = []string{"TST-TRANS-001"}
	state.CriteriaIndex["CRIT-TRANS-1"] = []*TestCaseModel{tc}

	ev := &lifecycle.LifecycleEvent{
		Seq:        1,
		Ts:         time.Now(),
		EventType:  lifecycle.EventTypeStatusTransition,
		Kind:       objects.KindCriteria,
		ID:         "CRIT-TRANS-1",
		FromStatus: objects.ObjectStatusOriginated,
		ToStatus:   objects.ObjectStatusComplete,
	}
	state.HandleLifecycleEvent(ev)

	if tc.Criteria[0].Status != "complete" {
		t.Errorf("expected criterion status complete, got %s", tc.Criteria[0].Status)
	}
	if tc.CompletedCriteria != 1 {
		t.Errorf("expected 1 completed criteria, got %d", tc.CompletedCriteria)
	}
	if tc.Status != objects.ObjectStatusComplete {
		t.Errorf("expected test case auto-completion, got %s", tc.Status)
	}
}

func TestDashboardState_FullLineageTraceability(t *testing.T) {
	state := NewDashboardState()

	tcIntact := &TestCaseModel{
		ID:       "TST-INTACT-001",
		Title:    "End-to-End Verified Suite",
		Status:   objects.ObjectStatusComplete,
		PathOrID: "pkg/api/test.go",
		Scope:    "integration",
		Lineage: &LineageChain{
			RootObject:   &LineageNode{ID: "GOAL-ROOT-001", Kind: objects.KindGoal, Status: objects.ObjectStatusComplete},
			Requirements: []*LineageNode{{ID: "REQ-001", Kind: objects.KindRequirement, Status: objects.ObjectStatusComplete}},
			BacklogItems: []*LineageNode{{ID: "BLI-001", Kind: objects.KindBacklogItem, Status: objects.ObjectStatusComplete}},
			IsIntact:     true,
		},
		Criteria: []*CriterionState{
			{ID: "CRIT-001", Status: objects.ObjectStatusComplete},
		},
		TotalCriteria:     1,
		CompletedCriteria: 1,
	}

	tcIncomplete := &TestCaseModel{
		ID:       "TST-INCOMPLETE-002",
		Title:    "Missing Root Suite",
		Status:   objects.ObjectStatusActive,
		PathOrID: "pkg/api/missing_root_test.go",
		Scope:    "unit",
		Lineage: &LineageChain{
			Requirements: []*LineageNode{{ID: "REQ-002", Kind: objects.KindRequirement, Status: objects.ObjectStatusActive}},
			IsIntact:     false,
			BrokenReason: "missing root object binding",
		},
		Criteria: []*CriterionState{
			{ID: "CRIT-002", Status: objects.ObjectStatusInProgress},
		},
		TotalCriteria:     1,
		CompletedCriteria: 0,
	}

	state.TestCases["TST-INTACT-001"] = tcIntact
	state.TestCases["TST-INCOMPLETE-002"] = tcIncomplete
	state.TestCaseOrder = []string{"TST-INTACT-001", "TST-INCOMPLETE-002"}
	state.CriteriaIndex["CRIT-001"] = []*TestCaseModel{tcIntact}
	state.CriteriaIndex["CRIT-002"] = []*TestCaseModel{tcIncomplete}

	var buf bytes.Buffer
	if err := state.Render(&buf, false, "all"); err != nil {
		t.Fatalf("render failed: %v", err)
	}

	out := buf.String()

	// Verify intact chain rendering
	if !strings.Contains(out, "GOAL-ROOT-001") || !strings.Contains(out, "REQ-001") || !strings.Contains(out, "BLI-001") {
		t.Errorf("expected full lineage path in output, got:\n%s", out)
	}
	if !strings.Contains(out, "Chain Intact") {
		t.Errorf("expected Chain Intact indicator in output")
	}

	// Verify incomplete chain rendering
	if !strings.Contains(out, "UNBOUND ROOT") || !strings.Contains(out, "Incomplete (missing root object binding)") {
		t.Errorf("expected incomplete chain warning in output, got:\n%s", out)
	}

	// Verify aggregate metrics lineage breakdown
	if !strings.Contains(out, "1/2 intact chains (full lineage to root object)") {
		t.Errorf("expected 1/2 intact chains in aggregate metrics, got:\n%s", out)
	}
}

func TestDashboardState_UnboundCriteria(t *testing.T) {
	state := NewDashboardState()

	unc := &UnboundCriterionModel{
		ID:               "CRIT-STANDALONE-TEST-001",
		Title:            "Orphan Acceptance Criterion",
		Status:           objects.ObjectStatusAwaitingVerification,
		Category:         "test",
		ValidationMethod: "automated_test",
		Lineage: &LineageChain{
			RootObject:   &LineageNode{ID: "GOAL-CORE-001", Kind: objects.KindGoal, Status: objects.ObjectStatusActive},
			Requirements: []*LineageNode{{ID: "REQ-999", Kind: objects.KindRequirement, Status: objects.ObjectStatusActive}},
			IsIntact:     true,
		},
	}
	state.UnboundTestCriteria = append(state.UnboundTestCriteria, unc)

	var buf bytes.Buffer
	if err := state.Render(&buf, false, "active"); err != nil {
		t.Fatalf("render failed: %v", err)
	}

	out := buf.String()

	if !strings.Contains(out, "UNBOUND & STANDALONE TEST CRITERIA") {
		t.Errorf("expected unbound test criteria section in output")
	}
	if !strings.Contains(out, "CRIT-STANDALONE-TEST-001") {
		t.Errorf("expected standalone criterion ID in output")
	}
	if !strings.Contains(out, "GOAL-CORE-001") || !strings.Contains(out, "REQ-999") {
		t.Errorf("expected parent lineage in unbound criteria, got:\n%s", out)
	}
	if !strings.Contains(out, "Test criterion has no test_case object bound!") {
		t.Errorf("expected unbound warning notice in output")
	}
}

func TestResolveLineageChain(t *testing.T) {
	mockStorage := map[string]map[string]any{
		"REQ-100": {
			objects.FieldKeyID:       "REQ-100",
			objects.FieldKeyStatus:   objects.ObjectStatusActive,
			objects.FieldKeyGoalRefs: []string{"GOAL-ROOT-100"},
			objects.FieldKeyTitle:    "Requirement 100",
		},
		"GOAL-ROOT-100": {
			objects.FieldKeyID:     "GOAL-ROOT-100",
			objects.FieldKeyStatus: objects.ObjectStatusComplete,
			objects.FieldKeyTitle:  "Master Goal",
		},
		"BLI-200": {
			objects.FieldKeyID:              "BLI-200",
			objects.FieldKeyStatus:          objects.ObjectStatusComplete,
			objects.FieldKeyPriorityPlanRef: "PRI-CORE-001",
			objects.FieldKeyTitle:           "Backlog Item 200",
		},
		"PRI-CORE-001": {
			objects.FieldKeyID:     "PRI-CORE-001",
			objects.FieldKeyStatus: objects.ObjectStatusActive,
			objects.FieldKeyTitle:  "Priority Plan Core",
		},
	}

	readMock := func(id string) map[string]any {
		return mockStorage[id]
	}

	// Case 1: Intact chain via Requirement -> Goal
	chain1 := resolveLineageChain(readMock, []string{"REQ-100"}, nil, nil)
	if !chain1.IsIntact {
		t.Errorf("expected chain1 to be intact, got: %s", chain1.BrokenReason)
	}
	if chain1.RootObject == nil || chain1.RootObject.ID != "GOAL-ROOT-100" {
		t.Errorf("expected root object GOAL-ROOT-100, got %+v", chain1.RootObject)
	}

	// Case 2: Intact chain via BacklogItem -> PriorityPlan
	chain2 := resolveLineageChain(readMock, nil, []string{"BLI-200"}, nil)
	if !chain2.IsIntact {
		t.Errorf("expected chain2 to be intact, got: %s", chain2.BrokenReason)
	}
	if chain2.RootObject == nil || chain2.RootObject.ID != "PRI-CORE-001" {
		t.Errorf("expected root object PRI-CORE-001, got %+v", chain2.RootObject)
	}

	// Case 3: Incomplete chain with missing root
	chain3 := resolveLineageChain(readMock, []string{"REQ-MISSING"}, nil, nil)
	if chain3.IsIntact {
		t.Errorf("expected chain3 to be incomplete")
	}

	// Case 4: Incomplete chain with no refs
	chain4 := resolveLineageChain(readMock, nil, nil, nil)
	if chain4.IsIntact {
		t.Errorf("expected chain4 to be incomplete")
	}
}

func TestDashboardState_GraduationStateMachine(t *testing.T) {
	state := NewDashboardState()

	tc := &TestCaseModel{
		ID:     "TST-GRAD-001",
		Title:  "Graduating Verification Suite",
		Status: objects.ObjectStatusActive,
		Lineage: &LineageChain{
			RootObject:   &LineageNode{ID: "GOAL-ROOT-001", Kind: objects.KindGoal, Status: objects.ObjectStatusComplete},
			Requirements: []*LineageNode{{ID: "REQ-001", Kind: objects.KindRequirement, Status: objects.ObjectStatusComplete}},
			IsIntact:     true,
		},
		Criteria: []*CriterionState{
			{ID: "CRIT-G-1", Status: objects.ObjectStatusComplete},
			{ID: "CRIT-G-2", Status: objects.ObjectStatusInProgress},
		},
		RemainingOpenCount: 1,
		TotalCriteria:      2,
		CompletedCriteria:  1,
	}

	state.TestCases["TST-GRAD-001"] = tc
	state.TestCaseOrder = []string{"TST-GRAD-001"}
	state.CriteriaIndex["CRIT-G-1"] = []*TestCaseModel{tc}
	state.CriteriaIndex["CRIT-G-2"] = []*TestCaseModel{tc}

	// 1. In active view, test case is present in active working set
	var bufActive1 bytes.Buffer
	if err := state.Render(&bufActive1, false, "active"); err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if !strings.Contains(bufActive1.String(), "ACTIVE WORKING SET (1 IN-FLIGHT") {
		t.Errorf("expected 1 in-flight test case in active working set, got:\n%s", bufActive1.String())
	}
	if !strings.Contains(bufActive1.String(), "TST-GRAD-001") {
		t.Errorf("expected TST-GRAD-001 in active view before graduation")
	}

	// 2. Fire criteria satisfied event for the final open criterion
	ev := &lifecycle.LifecycleEvent{
		Seq:         10,
		Ts:          time.Now(),
		EventType:   lifecycle.EventTypeCriterionSatisfied,
		CriterionID: "CRIT-G-2",
	}
	state.HandleLifecycleEvent(ev)

	// Assert completion state machine fired
	if tc.Status != objects.ObjectStatusComplete {
		t.Fatalf("expected test case status complete, got %s", tc.Status)
	}
	if tc.RemainingOpenCount != 0 {
		t.Fatalf("expected 0 open criteria, got %d", tc.RemainingOpenCount)
	}

	// Verify graduation event was recorded
	foundGradEvent := false
	for _, rev := range state.RecentEvents {
		if strings.Contains(rev.Message, "TRACEABILITY CHAIN GRADUATED: TST-GRAD-001") && strings.Contains(rev.Message, "Moved to Regression Suite") {
			foundGradEvent = true
			break
		}
	}
	if !foundGradEvent {
		t.Errorf("expected graduation event in recent events, got: %+v", state.RecentEvents)
	}

	// 3. Render active view again: test case is now pruned from active working set!
	var bufActive2 bytes.Buffer
	if err := state.Render(&bufActive2, false, "active"); err != nil {
		t.Fatalf("render failed: %v", err)
	}
	outActive2 := bufActive2.String()
	if !strings.Contains(outActive2, "All test case chains verified! Working set is clean.") {
		t.Errorf("expected working set to be clean, got:\n%s", outActive2)
	}
	if !strings.Contains(outActive2, "REGRESSION TESTING POOL:") || !strings.Contains(outActive2, "1 verified chains green & passing") {
		t.Errorf("expected 1 verified chain in regression testing pool, got:\n%s", outActive2)
	}

	// 4. Render regression view: test case appears in the regression suite
	var bufReg bytes.Buffer
	if err := state.Render(&bufReg, false, "regression"); err != nil {
		t.Fatalf("render failed: %v", err)
	}
	outReg := bufReg.String()
	if !strings.Contains(outReg, "REGRESSION TESTING SUITE (1 VERIFIED & INTACT CHAINS)") {
		t.Errorf("expected 1 chain in regression suite, got:\n%s", outReg)
	}
	if !strings.Contains(outReg, "TST-GRAD-001") {
		t.Errorf("expected TST-GRAD-001 in regression suite view")
	}
}

func TestDashboardState_LiteFileRoundTrip(t *testing.T) {
	tempDir := t.TempDir()

	state := NewDashboardState()
	tc := &TestCaseModel{
		ID:     "TST-LITE-001",
		Title:  "Lite File Test Suite",
		Status: objects.ObjectStatusActive,
		Criteria: []*CriterionState{
			{ID: "CRIT-L-1", Status: objects.ObjectStatusComplete},
		},
		Lineage: &LineageChain{
			RootObject:   &LineageNode{ID: "GOAL-LITE-ROOT", Kind: objects.KindGoal, Status: objects.ObjectStatusActive},
			Requirements: []*LineageNode{{ID: "REQ-LITE-1", Kind: objects.KindRequirement, Status: objects.ObjectStatusActive}},
			IsIntact:     true,
		},
		TotalCriteria:      1,
		CompletedCriteria:  1,
		RemainingOpenCount: 0,
	}
	state.TestCases["TST-LITE-001"] = tc
	state.TestCaseOrder = []string{"TST-LITE-001"}
	state.CriteriaIndex["CRIT-L-1"] = []*TestCaseModel{tc}

	// 1. Save to Lite-File
	if err := state.SaveToLiteFile(tempDir); err != nil {
		t.Fatalf("SaveToLiteFile failed: %v", err)
	}

	// Verify file exists on disk
	litePath := LiteFilePath(tempDir)
	if _, err := os.Stat(litePath); err != nil {
		t.Fatalf("expected lite-file to exist at %s, got: %v", litePath, err)
	}

	// 2. Load into fresh state
	newState := NewDashboardState()
	loaded, err := newState.LoadFromLiteFile(tempDir)
	if err != nil {
		t.Fatalf("LoadFromLiteFile failed: %v", err)
	}
	if !loaded {
		t.Fatalf("expected loaded=true")
	}

	// 3. Verify parity
	if len(newState.TestCaseOrder) != 1 || newState.TestCaseOrder[0] != "TST-LITE-001" {
		t.Errorf("expected TST-LITE-001 in test case order, got %+v", newState.TestCaseOrder)
	}
	loadedTC := newState.TestCases["TST-LITE-001"]
	if loadedTC == nil || loadedTC.Title != "Lite File Test Suite" {
		t.Errorf("expected loaded test case with matching title, got %+v", loadedTC)
	}
	if loadedTC.Lineage == nil || !loadedTC.Lineage.IsIntact {
		t.Errorf("expected loaded lineage to be intact")
	}
	if len(newState.CriteriaIndex["CRIT-L-1"]) != 1 {
		t.Errorf("expected criteria index to be rebuilt with 1 entry")
	}
}

func TestDashboardState_ValidateTPMDefinitionOfDone(t *testing.T) {
	state := NewDashboardState()

	// 1. Initially with broken chain
	tcBroken := &TestCaseModel{
		ID:     "TST-BROKEN-001",
		Title:  "Unbound Test Case",
		Status: objects.ObjectStatusActive,
		Lineage: &LineageChain{
			IsIntact:     false,
			BrokenReason: "missing root object binding",
		},
	}
	state.TestCases["TST-BROKEN-001"] = tcBroken
	state.TestCaseOrder = []string{"TST-BROKEN-001"}

	var outBuf, errBuf bytes.Buffer
	err := state.ValidateTPMDefinitionOfDone(&outBuf, &errBuf)
	if err == nil {
		t.Errorf("expected DoD validation to fail when broken chain exists")
	}
	if !strings.Contains(errBuf.String(), "TPM Definition of Done FAILED") {
		t.Errorf("expected failure message in stderr, got:\n%s", errBuf.String())
	}

	// 2. Fix the chain to intact
	tcBroken.Lineage.IsIntact = true
	tcBroken.Lineage.RootObject = &LineageNode{ID: "GOAL-ROOT-001", Kind: objects.KindGoal, Status: objects.ObjectStatusActive}

	outBuf.Reset()
	errBuf.Reset()
	err = state.ValidateTPMDefinitionOfDone(&outBuf, &errBuf)
	if err != nil {
		t.Errorf("expected DoD validation to pass with intact chain, got: %v", err)
	}
	if !strings.Contains(outBuf.String(), "TPM Definition of Done SATISFIED") {
		t.Errorf("expected success message in stdout, got:\n%s", outBuf.String())
	}

	// 3. Add an unbound test criterion; DoD must fail closed
	state.UnboundTestCriteria = []*UnboundCriterionModel{
		{
			ID:               "CRIT-UNBOUND-001",
			Title:            "Orphan Criteria",
			Status:           objects.ObjectStatusActive,
			Category:         "test",
			ValidationMethod: "automated_test",
		},
	}
	outBuf.Reset()
	errBuf.Reset()
	err = state.ValidateTPMDefinitionOfDone(&outBuf, &errBuf)
	if err == nil {
		t.Errorf("expected DoD validation to fail when unbound test criterion exists")
	}
	if !strings.Contains(errBuf.String(), "unbound test criteria detected") {
		t.Errorf("expected unbound criteria warning in stderr, got:\n%s", errBuf.String())
	}
}

func TestDashboard_ColorAndPagerFlags(t *testing.T) {
	cmd := NewDashboardCmd()

	colorFlag := cmd.Flags().Lookup("color")
	if colorFlag == nil {
		t.Fatalf("expected --color flag to be registered")
	}

	pagerFlag := cmd.Flags().Lookup("pager")
	if pagerFlag == nil {
		t.Fatalf("expected --pager flag to be registered")
	}
	if pagerFlag.Shorthand != "p" {
		t.Errorf("expected -p shorthand for --pager, got %q", pagerFlag.Shorthand)
	}

	// Verify runWithPager fallback executes render function, disconnects timeout monitor,
	// and invokes registered exit callbacks upon exit.
	t.Setenv("PAGER", "cat")
	executed := false
	var disconnectedDuringRender bool
	exitCallbackCalled := false
	cli.RegisterPagerExitCallback(func() {
		exitCallbackCalled = true
	})

	err := runWithPager(t.Context(), func(w ioWriter) error {
		executed = true
		disconnectedDuringRender = cli.IsTimeoutMonitorDisconnected()
		_, werr := w.Write([]byte("dashboard test output"))
		return werr
	})
	if err != nil {
		t.Fatalf("runWithPager failed: %v", err)
	}
	if !executed {
		t.Errorf("expected render function to be executed by runWithPager")
	}
	if !disconnectedDuringRender {
		t.Errorf("expected timeout monitor to be disconnected during pager execution")
	}
	if cli.IsTimeoutMonitorDisconnected() {
		t.Errorf("expected timeout monitor to be reconnected after pager exits")
	}
	if !exitCallbackCalled {
		t.Errorf("expected pager exit callback to be invoked")
	}
}

// TestDashboardState_AccumulatorConformance validates BLI-1789553224732980000-5f25ee03:
// DashboardState satisfies accumulator.Accumulator[*DashboardLitePayload] and guarantees sub-5ms read SLA.
func TestDashboardState_AccumulatorConformance(t *testing.T) {
	tempDir := t.TempDir()
	state := NewDashboardState()

	// Static interface compliance check
	var _ accumulator.Accumulator[*DashboardLitePayload] = state

	if state.Name() != "test_dashboard" {
		t.Errorf("expected name 'test_dashboard', got %s", state.Name())
	}

	eng := state.EnsureEngine(tempDir)
	if eng == nil {
		t.Fatal("expected non-nil engine from EnsureEngine")
	}

	// Persist initial payload
	tc := &TestCaseModel{
		ID:     "TST-ACC-001",
		Title:  "Accumulator Verification Test",
		Status: objects.ObjectStatusActive,
		Criteria: []*CriterionState{
			{ID: "CRIT-ACC-001", Status: objects.ObjectStatusComplete},
		},
		TotalCriteria:     1,
		CompletedCriteria: 1,
	}
	state.TestCases[tc.ID] = tc
	state.TestCaseOrder = []string{tc.ID}

	if err := state.SaveToLiteFile(tempDir); err != nil {
		t.Fatalf("save lite file failed: %v", err)
	}

	// Hot path SLA check
	newState := NewDashboardState()
	// Warmup read to load disk page into OS cache
	_, _ = newState.LoadFromLiteFile(tempDir)

	start := time.Now()
	loaded, err := newState.LoadFromLiteFile(tempDir)
	elapsed := time.Since(start)

	if err != nil || !loaded {
		t.Fatalf("load lite file failed: loaded=%v err=%v", loaded, err)
	}
	if elapsed > 5*time.Millisecond {
		t.Errorf("hot path read exceeded 5ms SLA: %v", elapsed)
	}
	if len(newState.TestCases) != 1 || newState.TestCases["TST-ACC-001"] == nil {
		t.Errorf("expected loaded test cases to contain TST-ACC-001, got %+v", newState.TestCases)
	}
}

func TestDashboardState_StartBackgroundWALSubscriber(t *testing.T) {
	tempDir := t.TempDir()
	state := NewDashboardStateWithProjectRoot(tempDir)
	if state.projectRoot != tempDir {
		t.Fatalf("expected projectRoot %s, got %s", tempDir, state.projectRoot)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	updateCh := make(chan struct{}, 10)
	state.StartBackgroundWALSubscriber(ctx, updateCh)

	wal, err := lifecycle.GetOrCreateLifecycleWAL(tempDir)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	defer wal.Close()

	event := &lifecycle.LifecycleEvent{
		Ts:        time.Now().UTC(),
		EventType: lifecycle.EventTypeStatusTransition,
		ID:        "TEST-001",
		Kind:      objects.KindTestCase,
		ToStatus:  objects.ObjectStatusActive,
	}
	if err := wal.Append(event); err != nil {
		t.Fatalf("append wal event: %v", err)
	}
	if err := wal.Sync(); err != nil {
		t.Fatalf("sync wal event: %v", err)
	}

	select {
	case <-updateCh:
		// Subscriber deterministically processed WAL event
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for WAL subscriber to process event")
	}
}

func TestDashboardState_HandleReferenceLinkedEvent(t *testing.T) {
	state := NewDashboardState()
	tc := &TestCaseModel{
		ID:       "TST-101",
		Status:   objects.ObjectStatusActive,
		Criteria: make([]*CriterionState, 0),
	}
	state.TestCases[tc.ID] = tc

	// Dispatch reference_linked event: TST-101 linked to CRIT-101
	ev := &lifecycle.LifecycleEvent{
		Seq:        1,
		Ts:         time.Now(),
		EventType:  lifecycle.EventTypeReferenceLinked,
		Kind:       objects.KindTestCase,
		ID:         "TST-101",
		TargetKind: objects.KindCriteria,
		TargetID:   "CRIT-101",
		FieldName:  "criteria_refs",
	}
	state.HandleLifecycleEvent(ev)

	if len(tc.Criteria) != 1 || tc.Criteria[0].ID != "CRIT-101" {
		t.Fatalf("expected criterion CRIT-101 linked to TST-101 in-memory, got %v", tc.Criteria)
	}
	if len(state.CriteriaIndex["CRIT-101"]) != 1 {
		t.Errorf("expected CriteriaIndex for CRIT-101 to contain TST-101")
	}
	if len(state.RecentEvents) != 1 || !strings.Contains(state.RecentEvents[0].Message, "LINKAGE SHOCKWAVE") {
		t.Errorf("expected LINKAGE SHOCKWAVE in recent events, got %v", state.RecentEvents)
	}

	// Verify that TST-101 was added to TestCaseOrder
	found := false
	for _, id := range state.TestCaseOrder {
		if id == "TST-101" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected TST-101 in state.TestCaseOrder")
	}
}

func TestDashboardState_HandleReferenceLinked_BacklogItemAndRequirement(t *testing.T) {
	state := NewDashboardState()
	tc := &TestCaseModel{
		ID:       "TST-201",
		Status:   objects.ObjectStatusActive,
		Criteria: make([]*CriterionState, 0),
	}
	state.TestCases[tc.ID] = tc

	// 1. Link Requirement
	evReq := &lifecycle.LifecycleEvent{
		Seq:        1,
		Ts:         time.Now(),
		EventType:  lifecycle.EventTypeReferenceLinked,
		Kind:       objects.KindTestCase,
		ID:         "TST-201",
		TargetKind: objects.KindRequirement,
		TargetID:   "REQ-201",
		FieldName:  "requirement_refs",
	}
	state.HandleLifecycleEvent(evReq)

	if len(tc.RequirementRefs) != 1 || tc.RequirementRefs[0] != "REQ-201" {
		t.Fatalf("expected requirement REQ-201 linked to TST-201, got %v", tc.RequirementRefs)
	}

	// 2. Link BacklogItem
	evBli := &lifecycle.LifecycleEvent{
		Seq:        2,
		Ts:         time.Now(),
		EventType:  lifecycle.EventTypeReferenceLinked,
		Kind:       objects.KindTestCase,
		ID:         "TST-201",
		TargetKind: objects.KindBacklogItem,
		TargetID:   "BLI-201",
		FieldName:  "backlog_item_refs",
	}
	state.HandleLifecycleEvent(evBli)

	if len(tc.BacklogItemRefs) != 1 || tc.BacklogItemRefs[0] != "BLI-201" {
		t.Fatalf("expected backlog item BLI-201 linked to TST-201, got %v", tc.BacklogItemRefs)
	}
}

func TestDashboardState_BuildPayload_EnsuresAllTestCasesInOrder(t *testing.T) {
	state := NewDashboardState()
	state.TestCases["TST-ORPHAN-01"] = &TestCaseModel{
		ID:     "TST-ORPHAN-01",
		Title:  "Orphan",
		Status: objects.ObjectStatusActive,
	}
	// Note: state.TestCaseOrder intentionally left empty

	payload := state.BuildPayload()
	if payload.TotalTestCases != 1 {
		t.Fatalf("expected TotalTestCases = 1, got %d", payload.TotalTestCases)
	}
	if len(payload.TestCaseOrder) != 1 || payload.TestCaseOrder[0] != "TST-ORPHAN-01" {
		t.Fatalf("expected TestCaseOrder to include TST-ORPHAN-01, got %v", payload.TestCaseOrder)
	}
}

