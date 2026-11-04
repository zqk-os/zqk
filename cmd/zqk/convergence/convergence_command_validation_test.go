package convergence_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/cmd/zqk/convergence"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/convergerollup"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestConvergenceCommand_HierarchyAndSubcommands(t *testing.T) {
	cmd := convergence.NewConvergenceCmd()
	if cmd == nil {
		t.Fatalf("expected non-nil convergence command")
	}

	if cmd.Use != "convergence" {
		t.Fatalf("expected command Use 'convergence', got: %q", cmd.Use)
	}
	if len(cmd.Short) == 0 {
		t.Fatalf("expected non-empty Short description")
	}
	if len(cmd.Long) == 0 {
		t.Fatalf("expected non-empty Long description")
	}

	expectedSubcommands := []string{
		"nest-spawn",
		"nest-link",
		"nest-status",
		"record-cvs-orchestrate-run",
		"measure",
		"overseer",
		"promotion-readiness",
		"record-overseer-run",
	}

	for _, subName := range expectedSubcommands {
		subCmd, subArgs, findErr := cmd.Find([]string{subName})
		if findErr != nil {
			t.Fatalf("failed finding subcommand %q: %v", subName, findErr)
		}
		if subCmd == nil {
			t.Fatalf("expected subcommand %q to be non-nil", subName)
		}
		if len(subArgs) > 0 {
			t.Fatalf("unexpected trailing args when finding %q: %v", subName, subArgs)
		}
		if len(subCmd.Short) == 0 {
			t.Fatalf("expected non-empty Short description for subcommand %q", subName)
		}
	}

	if len(cmd.Commands()) < len(expectedSubcommands) {
		t.Fatalf("expected at least %d subcommands, found: %d", len(expectedSubcommands), len(cmd.Commands()))
	}
}

func TestConvergenceCommand_FlagDefinitionsAndDescriptions(t *testing.T) {
	cmd := convergence.NewConvergenceCmd()
	if cmd == nil {
		t.Fatalf("expected non-nil convergence command")
	}

	// 1. nest-spawn flags
	nestSpawnCmd, spawnArgs, findErr := cmd.Find([]string{"nest-spawn"})
	if findErr != nil {
		t.Fatalf("failed finding nest-spawn: %v", findErr)
	}
	if len(spawnArgs) > 0 {
		t.Fatalf("unexpected trailing args finding nest-spawn: %v", spawnArgs)
	}

	nestSpawnExpectedFlags := []struct {
		name     string
		flagType string
	}{
		{"parent-session-id", "string"},
		{"title", "string"},
		{"hypothesis", "string"},
		{"desired-end-state", "string"},
		{"next-action", "string"},
		{"current-phase", "string"},
		{"max-depth", "int"},
	}

	for _, ef := range nestSpawnExpectedFlags {
		flag := nestSpawnCmd.Flags().Lookup(ef.name)
		if flag == nil {
			t.Fatalf("nest-spawn missing expected flag: --%s", ef.name)
		}
		if flag.Value.Type() != ef.flagType {
			t.Fatalf("nest-spawn flag --%s expected type %s, got: %s", ef.name, ef.flagType, flag.Value.Type())
		}
		if len(flag.Usage) == 0 {
			t.Fatalf("nest-spawn flag --%s has empty usage description", ef.name)
		}
	}

	// 2. nest-link flags
	nestLinkCmd, linkArgs, findErr := cmd.Find([]string{"nest-link"})
	if findErr != nil {
		t.Fatalf("failed finding nest-link: %v", findErr)
	}
	if len(linkArgs) > 0 {
		t.Fatalf("unexpected trailing args finding nest-link: %v", linkArgs)
	}

	nestLinkExpectedFlags := []struct {
		name     string
		flagType string
	}{
		{"parent-session-id", "string"},
		{"child-session-id", "string"},
		{"coordinator-session-id", "string"},
		{"max-depth", "int"},
	}

	for _, ef := range nestLinkExpectedFlags {
		flag := nestLinkCmd.Flags().Lookup(ef.name)
		if flag == nil {
			t.Fatalf("nest-link missing expected flag: --%s", ef.name)
		}
		if flag.Value.Type() != ef.flagType {
			t.Fatalf("nest-link flag --%s expected type %s, got: %s", ef.name, ef.flagType, flag.Value.Type())
		}
		if len(flag.Usage) == 0 {
			t.Fatalf("nest-link flag --%s has empty usage description", ef.name)
		}
	}

	// 3. nest-status flags
	nestStatusCmd, statusArgs, findErr := cmd.Find([]string{"nest-status"})
	if findErr != nil {
		t.Fatalf("failed finding nest-status: %v", findErr)
	}
	if len(statusArgs) > 0 {
		t.Fatalf("unexpected trailing args finding nest-status: %v", statusArgs)
	}

	nestStatusExpectedFlags := []struct {
		name     string
		flagType string
	}{
		{"parent-session-id", "string"},
		{"max-depth", "int"},
	}

	for _, ef := range nestStatusExpectedFlags {
		flag := nestStatusCmd.Flags().Lookup(ef.name)
		if flag == nil {
			t.Fatalf("nest-status missing expected flag: --%s", ef.name)
		}
		if flag.Value.Type() != ef.flagType {
			t.Fatalf("nest-status flag --%s expected type %s, got: %s", ef.name, ef.flagType, flag.Value.Type())
		}
		if len(flag.Usage) == 0 {
			t.Fatalf("nest-status flag --%s has empty usage description", ef.name)
		}
	}
}

func TestConvergenceCommand_FlagParsingAndValidation(t *testing.T) {
	cmd := convergence.NewConvergenceCmd()
	if cmd == nil {
		t.Fatalf("expected non-nil convergence command")
	}

	// 1. Test nest-spawn flag parsing
	nestSpawnCmd, spawnFindArgs, findErr := cmd.Find([]string{"nest-spawn"})
	if findErr != nil {
		t.Fatalf("failed finding nest-spawn: %v", findErr)
	}
	if len(spawnFindArgs) > 0 {
		t.Fatalf("unexpected trailing args finding nest-spawn: %v", spawnFindArgs)
	}

	spawnArgs := []string{
		"--parent-session-id", "CVS-PAR-001",
		"--title", "Elevate Subsystem Tests",
		"--hypothesis", "Unit testing increases stability",
		"--desired-end-state", "100% test coverage achieved",
		"--next-action", "Run all tests",
		"--current-phase", "c1_scope",
		"--max-depth", "4",
	}

	parseErr := nestSpawnCmd.ParseFlags(spawnArgs)
	if parseErr != nil {
		t.Fatalf("nest-spawn failed to parse flags: %v", parseErr)
	}

	parentID, err := nestSpawnCmd.Flags().GetString("parent-session-id")
	if err != nil {
		t.Fatalf("failed reading parent-session-id flag: %v", err)
	}
	if parentID != "CVS-PAR-001" {
		t.Fatalf("expected parent-session-id 'CVS-PAR-001', got: %q", parentID)
	}

	title, err := nestSpawnCmd.Flags().GetString("title")
	if err != nil {
		t.Fatalf("failed reading title flag: %v", err)
	}
	if title != "Elevate Subsystem Tests" {
		t.Fatalf("expected title 'Elevate Subsystem Tests', got: %q", title)
	}

	hypothesis, err := nestSpawnCmd.Flags().GetString("hypothesis")
	if err != nil {
		t.Fatalf("failed reading hypothesis flag: %v", err)
	}
	if hypothesis != "Unit testing increases stability" {
		t.Fatalf("expected hypothesis 'Unit testing increases stability', got: %q", hypothesis)
	}

	desired, err := nestSpawnCmd.Flags().GetString("desired-end-state")
	if err != nil {
		t.Fatalf("failed reading desired-end-state flag: %v", err)
	}
	if desired != "100% test coverage achieved" {
		t.Fatalf("expected desired-end-state '100%% test coverage achieved', got: %q", desired)
	}

	nextAction, err := nestSpawnCmd.Flags().GetString("next-action")
	if err != nil {
		t.Fatalf("failed reading next-action flag: %v", err)
	}
	if nextAction != "Run all tests" {
		t.Fatalf("expected next-action 'Run all tests', got: %q", nextAction)
	}

	phase, err := nestSpawnCmd.Flags().GetString("current-phase")
	if err != nil {
		t.Fatalf("failed reading current-phase flag: %v", err)
	}
	if phase != "c1_scope" {
		t.Fatalf("expected current-phase 'c1_scope', got: %q", phase)
	}

	maxDepth, err := nestSpawnCmd.Flags().GetInt("max-depth")
	if err != nil {
		t.Fatalf("failed reading max-depth flag: %v", err)
	}
	if maxDepth != 4 {
		t.Fatalf("expected max-depth 4, got: %d", maxDepth)
	}

	// 2. Test nest-link flag parsing
	nestLinkCmd, linkFindArgs, findErr := cmd.Find([]string{"nest-link"})
	if findErr != nil {
		t.Fatalf("failed finding nest-link: %v", findErr)
	}
	if len(linkFindArgs) > 0 {
		t.Fatalf("unexpected trailing args finding nest-link: %v", linkFindArgs)
	}

	linkArgs := []string{
		"--parent-session-id", "CVS-PAR-002",
		"--child-session-id", "CVS-CHD-002",
		"--coordinator-session-id", "CVS-COORD-001",
		"--max-depth", "5",
	}

	parseErr = nestLinkCmd.ParseFlags(linkArgs)
	if parseErr != nil {
		t.Fatalf("nest-link failed to parse flags: %v", parseErr)
	}

	linkParentID, err := nestLinkCmd.Flags().GetString("parent-session-id")
	if err != nil {
		t.Fatalf("failed reading parent-session-id flag: %v", err)
	}
	if linkParentID != "CVS-PAR-002" {
		t.Fatalf("expected parent-session-id 'CVS-PAR-002', got: %q", linkParentID)
	}

	childID, err := nestLinkCmd.Flags().GetString("child-session-id")
	if err != nil {
		t.Fatalf("failed reading child-session-id flag: %v", err)
	}
	if childID != "CVS-CHD-002" {
		t.Fatalf("expected child-session-id 'CVS-CHD-002', got: %q", childID)
	}

	coordID, err := nestLinkCmd.Flags().GetString("coordinator-session-id")
	if err != nil {
		t.Fatalf("failed reading coordinator-session-id flag: %v", err)
	}
	if coordID != "CVS-COORD-001" {
		t.Fatalf("expected coordinator-session-id 'CVS-COORD-001', got: %q", coordID)
	}

	linkMaxDepth, err := nestLinkCmd.Flags().GetInt("max-depth")
	if err != nil {
		t.Fatalf("failed reading max-depth flag: %v", err)
	}
	if linkMaxDepth != 5 {
		t.Fatalf("expected max-depth 5, got: %d", linkMaxDepth)
	}

	// 3. Test nest-status flag parsing
	nestStatusCmd, statusFindArgs, findErr := cmd.Find([]string{"nest-status"})
	if findErr != nil {
		t.Fatalf("failed finding nest-status: %v", findErr)
	}
	if len(statusFindArgs) > 0 {
		t.Fatalf("unexpected trailing args finding nest-status: %v", statusFindArgs)
	}

	statusArgs := []string{
		"--parent-session-id", "CVS-PAR-003",
		"--max-depth", "2",
	}

	parseErr = nestStatusCmd.ParseFlags(statusArgs)
	if parseErr != nil {
		t.Fatalf("nest-status failed to parse flags: %v", parseErr)
	}

	statusParentID, err := nestStatusCmd.Flags().GetString("parent-session-id")
	if err != nil {
		t.Fatalf("failed reading parent-session-id flag: %v", err)
	}
	if statusParentID != "CVS-PAR-003" {
		t.Fatalf("expected parent-session-id 'CVS-PAR-003', got: %q", statusParentID)
	}

	statusMaxDepth, err := nestStatusCmd.Flags().GetInt("max-depth")
	if err != nil {
		t.Fatalf("failed reading max-depth flag: %v", err)
	}
	if statusMaxDepth != 2 {
		t.Fatalf("expected max-depth 2, got: %d", statusMaxDepth)
	}
}

func TestConvergenceBFSTraversal_ValidationAndHierarchy(t *testing.T) {
	// 1. Negative inputs
	negNodes1, err := convergerollup.CollectCVSTreeBFS("", 3, func(id string) ([]string, string, string, error) {
		return nil, "", "", nil
	})
	if err == nil {
		t.Fatalf("expected error when rootID is empty")
	}
	if negNodes1 != nil {
		t.Fatalf("expected nil nodes when rootID is empty")
	}

	negNodes2, err := convergerollup.CollectCVSTreeBFS("CVS-ROOT", 3, nil)
	if err == nil {
		t.Fatalf("expected error when nodeFor is nil")
	}
	if negNodes2 != nil {
		t.Fatalf("expected nil nodes when nodeFor is nil")
	}

	negNodes3, err := convergerollup.CollectCVSTreeBFS("CVS-ROOT", -1, func(id string) ([]string, string, string, error) {
		return nil, "", "", nil
	})
	if err == nil {
		t.Fatalf("expected error when maxDepth is negative")
	}
	if negNodes3 != nil {
		t.Fatalf("expected nil nodes when maxDepth is negative")
	}

	// 2. Multi-level hierarchy verification
	hierarchy := map[string]struct {
		refs   []string
		status string
		phase  string
	}{
		"CVS-ROOT": {
			refs:   []string{"CVS-CHILD-A", "CVS-CHILD-B", "REQ-IGNORED-001"},
			status: "active",
			phase:  "c1_scope",
		},
		"CVS-CHILD-A": {
			refs:   []string{"CVS-CHILD-A1", "CVS-CHILD-A2"},
			status: "active",
			phase:  "c2_execute",
		},
		"CVS-CHILD-B": {
			refs:   nil,
			status: "active",
			phase:  "c3_verify",
		},
		"CVS-CHILD-A1": {
			refs:   nil,
			status: "completed",
			phase:  "c4_measure",
		},
		"CVS-CHILD-A2": {
			refs:   nil,
			status: "failed",
			phase:  "c4_measure",
		},
	}

	mockLoader := func(id string) ([]string, string, string, error) {
		node, ok := hierarchy[id]
		if !ok {
			return nil, "", "", fmt.Errorf("node not found: %s", id)
		}
		return node.refs, node.status, node.phase, nil
	}

	nodes, err := convergerollup.CollectCVSTreeBFS("CVS-ROOT", 3, mockLoader)
	if err != nil {
		t.Fatalf("CollectCVSTreeBFS failed: %v", err)
	}

	if len(nodes) != 5 {
		t.Fatalf("expected 5 nodes in hierarchy, got: %d", len(nodes))
	}

	// Verify BFS order: root (depth 0), child A and B (depth 1), child A1 and A2 (depth 2)
	expectedOrder := []struct {
		id    string
		depth int
		phase string
	}{
		{"CVS-ROOT", 0, "c1_scope"},
		{"CVS-CHILD-A", 1, "c2_execute"},
		{"CVS-CHILD-B", 1, "c3_verify"},
		{"CVS-CHILD-A1", 2, "c4_measure"},
		{"CVS-CHILD-A2", 2, "c4_measure"},
	}

	for i, eo := range expectedOrder {
		if nodes[i].ID != eo.id {
			t.Fatalf("node[%d] expected ID %s, got: %s", i, eo.id, nodes[i].ID)
		}
		if nodes[i].Depth != eo.depth {
			t.Fatalf("node[%d] (%s) expected Depth %d, got: %d", i, eo.id, eo.depth, nodes[i].Depth)
		}
		if nodes[i].CurrentPhase != eo.phase {
			t.Fatalf("node[%d] (%s) expected Phase %s, got: %s", i, eo.id, eo.phase, nodes[i].CurrentPhase)
		}
	}

	// 3. Depth-limited BFS verification
	depthLimitedNodes, err := convergerollup.CollectCVSTreeBFS("CVS-ROOT", 1, mockLoader)
	if err != nil {
		t.Fatalf("CollectCVSTreeBFS with maxDepth=1 failed: %v", err)
	}
	if len(depthLimitedNodes) != 3 {
		t.Fatalf("expected 3 nodes at depth <= 1, got: %d", len(depthLimitedNodes))
	}
	for _, n := range depthLimitedNodes {
		if n.Depth > 1 {
			t.Fatalf("node %s has depth %d > 1", n.ID, n.Depth)
		}
	}

	// 4. Node loader error propagation
	errorLoader := func(id string) ([]string, string, string, error) {
		if id == "CVS-FAIL" {
			return nil, "", "", fmt.Errorf("simulated loader failure")
		}
		return []string{"CVS-FAIL"}, "active", "c1_scope", nil
	}
	errNodes, err := convergerollup.CollectCVSTreeBFS("CVS-ROOT", 3, errorLoader)
	if err == nil {
		t.Fatalf("expected error when nodeLoader fails for child")
	}
	if errNodes != nil {
		t.Fatalf("expected nil nodes on loader failure")
	}
	if !strings.Contains(err.Error(), "simulated loader failure") {
		t.Fatalf("expected loader failure message, got: %v", err)
	}
}

func TestConvergenceNegativeBoundaries_Validation(t *testing.T) {
	treeData := map[string][]string{
		"CVS-ROOT":  {"CVS-LVL1"},
		"CVS-LVL1":  {"CVS-LVL2"},
		"CVS-LVL2":  {"CVS-LVL3"},
		"CVS-LVL3":  nil,
		"CVS-OTHER": nil,
	}

	loader := func(id string) ([]string, string, string, error) {
		refs, ok := treeData[id]
		if !ok {
			return nil, "", "", fmt.Errorf("unknown session: %s", id)
		}
		return refs, "active", "c1_scope", nil
	}

	// 1. Missing parent ID
	err := convergerollup.ValidateNestLink("CVS-ROOT", "", "CVS-CHILD", 3, loader)
	if err == nil {
		t.Fatalf("expected error for empty parent ID")
	}
	if !strings.Contains(err.Error(), "requires parent and child") {
		t.Fatalf("unexpected error message: %v", err)
	}

	// 2. Missing child ID
	err = convergerollup.ValidateNestLink("CVS-ROOT", "CVS-PARENT", "", 3, loader)
	if err == nil {
		t.Fatalf("expected error for empty child ID")
	}
	if !strings.Contains(err.Error(), "requires parent and child") {
		t.Fatalf("unexpected error message: %v", err)
	}

	// 3. Non-CVS parent prefix
	err = convergerollup.ValidateNestLink("CVS-ROOT", "INVALID-PARENT", "CVS-CHILD", 3, loader)
	if err == nil {
		t.Fatalf("expected error for non-CVS parent prefix")
	}
	if !strings.Contains(err.Error(), "requires CVS-* ids") {
		t.Fatalf("unexpected error message: %v", err)
	}

	// 4. Non-CVS child prefix
	err = convergerollup.ValidateNestLink("CVS-ROOT", "CVS-PARENT", "INVALID-CHILD", 3, loader)
	if err == nil {
		t.Fatalf("expected error for non-CVS child prefix")
	}
	if !strings.Contains(err.Error(), "requires CVS-* ids") {
		t.Fatalf("unexpected error message: %v", err)
	}

	// 5. Self nesting (parent == child)
	err = convergerollup.ValidateNestLink("CVS-ROOT", "CVS-SAME", "CVS-SAME", 3, loader)
	if err == nil {
		t.Fatalf("expected error for self-nesting")
	}
	if !strings.Contains(err.Error(), "cannot nest CVS under itself") {
		t.Fatalf("unexpected error message: %v", err)
	}

	// 6. Parent not reachable under coordinator
	err = convergerollup.ValidateNestLink("CVS-ROOT", "CVS-OTHER", "CVS-NEW", 3, loader)
	if err == nil {
		t.Fatalf("expected error when parent is not found under coordinator")
	}
	if !strings.Contains(err.Error(), "not found under coordinator") {
		t.Fatalf("unexpected error message: %v", err)
	}

	// 7. Max depth exceeded
	// CVS-ROOT (depth 0) -> CVS-LVL1 (depth 1) -> CVS-LVL2 (depth 2) -> CVS-LVL3 (depth 3)
	// Nesting under CVS-LVL3 with maxDepth=3 requires parentDepth+1 (3+1=4) <= 3, which fails.
	err = convergerollup.ValidateNestLink("CVS-ROOT", "CVS-LVL3", "CVS-LVL4", 3, loader)
	if err == nil {
		t.Fatalf("expected error when nesting exceeds maxDepth")
	}
	if !strings.Contains(err.Error(), "would exceed max depth") {
		t.Fatalf("unexpected error message: %v", err)
	}

	// 8. Cyclic nesting: linking an ancestor under a descendant
	cycleTree := map[string][]string{
		"CVS-A": {"CVS-B"},
		"CVS-B": {"CVS-C"},
		"CVS-C": nil,
	}
	cycleLoader := func(id string) ([]string, string, string, error) {
		refs, ok := cycleTree[id]
		if !ok {
			return nil, "", "", fmt.Errorf("session not found: %s", id)
		}
		return refs, "active", "c1_scope", nil
	}

	// Attempting to link CVS-A (ancestor) under CVS-C (descendant)
	err = convergerollup.ValidateNestLink("CVS-A", "CVS-C", "CVS-A", 5, cycleLoader)
	if err == nil {
		t.Fatalf("expected error for cyclic nesting")
	}
	if !strings.Contains(err.Error(), "would create a cycle") {
		t.Fatalf("unexpected error message: %v", err)
	}

	// 9. NestStatus negative boundaries
	emptyParentStatus, err := convergerollup.NestStatus("", 3, loader)
	if err == nil {
		t.Fatalf("expected error for empty parentID in NestStatus")
	}
	if emptyParentStatus != nil {
		t.Fatalf("expected nil result for empty parentID in NestStatus")
	}
	if !strings.Contains(err.Error(), "requires parent CVS id") {
		t.Fatalf("unexpected error message: %v", err)
	}

	// 10. AppendRelatedObjectRef helper tests
	initialRefs := []string{"CVS-001", "CVS-002"}
	// Empty string does not modify
	afterEmpty := convergerollup.AppendRelatedObjectRef(initialRefs, "")
	if len(afterEmpty) != 2 {
		t.Fatalf("expected 2 refs after appending empty, got: %d", len(afterEmpty))
	}
	// Duplicate ID is not appended
	afterDup := convergerollup.AppendRelatedObjectRef(initialRefs, "CVS-001")
	if len(afterDup) != 2 {
		t.Fatalf("expected 2 refs after appending duplicate, got: %d", len(afterDup))
	}
	// New ID is appended
	afterNew := convergerollup.AppendRelatedObjectRef(initialRefs, "CVS-003")
	if len(afterNew) != 3 {
		t.Fatalf("expected 3 refs after appending new id, got: %d", len(afterNew))
	}

	// 11. RelatedObjectRefsFromMap helper tests
	nilMapRefs := convergerollup.RelatedObjectRefsFromMap(nil)
	if nilMapRefs != nil {
		t.Fatalf("expected nil refs for nil map")
	}

	emptyMapRefs := convergerollup.RelatedObjectRefsFromMap(map[string]any{})
	if emptyMapRefs != nil {
		t.Fatalf("expected nil refs for map without related_object_refs")
	}

	sliceStringMap := map[string]any{
		objects.FieldKeyRelatedObjectRefs: []string{"CVS-1", "CVS-2"},
	}
	sliceStringRefs := convergerollup.RelatedObjectRefsFromMap(sliceStringMap)
	if len(sliceStringRefs) != 2 || sliceStringRefs[0] != "CVS-1" || sliceStringRefs[1] != "CVS-2" {
		t.Fatalf("unexpected extracted string refs: %v", sliceStringRefs)
	}

	sliceAnyMap := map[string]any{
		objects.FieldKeyRelatedObjectRefs: []any{"CVS-3", "CVS-4", 123},
	}
	sliceAnyRefs := convergerollup.RelatedObjectRefsFromMap(sliceAnyMap)
	if len(sliceAnyRefs) != 2 || sliceAnyRefs[0] != "CVS-3" || sliceAnyRefs[1] != "CVS-4" {
		t.Fatalf("unexpected extracted any refs: %v", sliceAnyRefs)
	}

	// 12. ChildSessionFields helper tests
	req := convergerollup.NestSpawnRequest{
		ParentID:        "CVS-PARENT-X",
		Title:           "Child Session Title",
		Hypothesis:      "Child Hypothesis",
		DesiredEndState: "Child Desired State",
		NextAction:      "Child Next Action",
		CurrentPhase:    "",
		Predictions: map[string]any{
			"coverage": 100,
		},
	}
	fields := convergerollup.ChildSessionFields(req)
	if fields[objects.FieldKeyKind] != objects.KindConvergenceSession {
		t.Fatalf("expected kind %s, got: %v", objects.KindConvergenceSession, fields[objects.FieldKeyKind])
	}
	if fields[objects.FieldKeyCurrentPhase] != "c1_scope" {
		t.Fatalf("expected default phase 'c1_scope', got: %v", fields[objects.FieldKeyCurrentPhase])
	}
	if fields[objects.FieldKeyNextAction] != "Child Next Action" {
		t.Fatalf("expected next action 'Child Next Action', got: %v", fields[objects.FieldKeyNextAction])
	}
	if fields[objects.FieldKeyHypothesis] != "Child Hypothesis" {
		t.Fatalf("expected hypothesis 'Child Hypothesis', got: %v", fields[objects.FieldKeyHypothesis])
	}
}

func TestConvergenceNestVeneer_ExecutionWorkflows(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// Create CLI context for commands
	cliCtx := cli.ContextForProjectRoot(proj.Root)

	// 1. nest-spawn validation: missing required flags fails closed
	topCmd := convergence.NewConvergenceCmd()
	nestSpawnCmd, spawnFindArgs, findErr := topCmd.Find([]string{"nest-spawn"})
	if findErr != nil {
		t.Fatalf("failed finding nest-spawn: %v", findErr)
	}
	if len(spawnFindArgs) > 0 {
		t.Fatalf("unexpected trailing args finding nest-spawn: %v", spawnFindArgs)
	}
	cli.SetContext(nestSpawnCmd, cliCtx)

	err := nestSpawnCmd.Flags().Set("parent-session-id", "CVS-PARENT-001")
	if err != nil {
		t.Fatalf("failed setting parent-session-id: %v", err)
	}
	// Missing title, hypothesis, desired-end-state
	runErr := nestSpawnCmd.RunE(nestSpawnCmd, []string{})
	if runErr == nil {
		t.Fatalf("expected nest-spawn to fail when required flags are missing")
	}
	if !strings.Contains(runErr.Error(), "nest-spawn requires --title, --hypothesis, and --desired-end-state") {
		t.Fatalf("unexpected error message for missing flags: %v", runErr)
	}

	// 2. Prepare parent CVS in storage
	storageProvider := proj.FileStorage
	if storageProvider == nil {
		t.Fatalf("expected non-nil FileStorage on isolated temp project")
	}
	opCtx := context.Background()
	sec := pkgctx.NewTestSecurityContext()

	parentObj := map[string]any{
		objects.FieldKeyID:                "CVS-PARENT-001",
		objects.FieldKeyKind:              objects.KindConvergenceSession,
		objects.FieldKeyStatus:            "active",
		objects.FieldKeyCurrentPhase:      "c1_scope",
		objects.FieldKeyOutcomeCharacter:  "expected",
		objects.FieldKeyHypothesis:        "Parent hypothesis",
		objects.FieldKeyDesiredEndState:   "Parent desired end state",
		objects.FieldKeyTitle:             "Parent Convergence Session",
		objects.FieldKeyRelatedObjectRefs: []string{},
	}
	err = storageProvider.Create(opCtx, sec, parentObj)
	if err != nil {
		t.Fatalf("failed creating parent session in storage: %v", err)
	}

	// 3. Execute nest-spawn successfully
	err = nestSpawnCmd.Flags().Set("title", "Spawned Child Session")
	if err != nil {
		t.Fatalf("failed setting title flag: %v", err)
	}
	err = nestSpawnCmd.Flags().Set("hypothesis", "Isolated execution validates veneer")
	if err != nil {
		t.Fatalf("failed setting hypothesis flag: %v", err)
	}
	err = nestSpawnCmd.Flags().Set("desired-end-state", "Child is spawned and linked to parent")
	if err != nil {
		t.Fatalf("failed setting desired-end-state flag: %v", err)
	}
	err = nestSpawnCmd.Flags().Set("current-phase", "c1_scope")
	if err != nil {
		t.Fatalf("failed setting current-phase flag: %v", err)
	}
	err = nestSpawnCmd.Flags().Set("max-depth", "3")
	if err != nil {
		t.Fatalf("failed setting max-depth flag: %v", err)
	}

	spawnBuf := new(bytes.Buffer)
	nestSpawnCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), spawnBuf))
	nestSpawnCmd.SetOut(spawnBuf)

	runErr = nestSpawnCmd.RunE(nestSpawnCmd, []string{})
	if runErr != nil {
		t.Fatalf("nest-spawn RunE failed: %v", runErr)
	}

	// Verify parent session updated in storage
	updatedParent, err := storageProvider.Read(opCtx, sec, "CVS-PARENT-001")
	if err != nil {
		t.Fatalf("failed reading updated parent session: %v", err)
	}
	refs := convergerollup.RelatedObjectRefsFromMap(updatedParent)
	if len(refs) == 0 {
		t.Fatalf("expected parent to contain linked child in related_object_refs")
	}
	spawnedChildID := refs[0]
	if !strings.HasPrefix(spawnedChildID, "CVS-") {
		t.Fatalf("expected child ID with CVS- prefix, got: %s", spawnedChildID)
	}

	// Verify child session in storage
	childObj, err := storageProvider.Read(opCtx, sec, spawnedChildID)
	if err != nil {
		t.Fatalf("failed reading spawned child session: %v", err)
	}
	if childObj[objects.FieldKeyTitle] != "Spawned Child Session" {
		t.Fatalf("expected child title 'Spawned Child Session', got: %v", childObj[objects.FieldKeyTitle])
	}

	// 4. Prepare pre-existing child and execute nest-link
	existingChildObj := map[string]any{
		objects.FieldKeyID:                "CVS-EXISTING-CHILD",
		objects.FieldKeyKind:              objects.KindConvergenceSession,
		objects.FieldKeyStatus:            "active",
		objects.FieldKeyCurrentPhase:      "c2_execute",
		objects.FieldKeyOutcomeCharacter:  "expected",
		objects.FieldKeyHypothesis:        "Child hypothesis",
		objects.FieldKeyDesiredEndState:   "Child desired end state",
		objects.FieldKeyTitle:             "Pre-existing Child Session",
		objects.FieldKeyRelatedObjectRefs: []string{},
	}
	err = storageProvider.Create(opCtx, sec, existingChildObj)
	if err != nil {
		t.Fatalf("failed creating existing child session in storage: %v", err)
	}

	nestLinkCmd, linkFindArgs, findErr := topCmd.Find([]string{"nest-link"})
	if findErr != nil {
		t.Fatalf("failed finding nest-link: %v", findErr)
	}
	if len(linkFindArgs) > 0 {
		t.Fatalf("unexpected trailing args finding nest-link: %v", linkFindArgs)
	}
	cli.SetContext(nestLinkCmd, cliCtx)
	linkOutBuf := new(bytes.Buffer)
	nestLinkCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), linkOutBuf))
	nestLinkCmd.SetOut(linkOutBuf)

	err = nestLinkCmd.Flags().Set("parent-session-id", "CVS-PARENT-001")
	if err != nil {
		t.Fatalf("failed setting parent-session-id for nest-link: %v", err)
	}
	err = nestLinkCmd.Flags().Set("child-session-id", "CVS-EXISTING-CHILD")
	if err != nil {
		t.Fatalf("failed setting child-session-id for nest-link: %v", err)
	}
	err = nestLinkCmd.Flags().Set("max-depth", "3")
	if err != nil {
		t.Fatalf("failed setting max-depth for nest-link: %v", err)
	}

	runErr = nestLinkCmd.RunE(nestLinkCmd, []string{})
	if runErr != nil {
		t.Fatalf("nest-link RunE failed: %v", runErr)
	}

	// Verify parent session now has both children
	parentAfterLink, err := storageProvider.Read(opCtx, sec, "CVS-PARENT-001")
	if err != nil {
		t.Fatalf("failed reading parent session after link: %v", err)
	}
	refsAfterLink := convergerollup.RelatedObjectRefsFromMap(parentAfterLink)
	if len(refsAfterLink) < 2 {
		t.Fatalf("expected at least 2 children in parent related_object_refs, got: %d", len(refsAfterLink))
	}

	// 5. Execute nest-status
	nestStatusCmd, statusFindArgs, findErr := topCmd.Find([]string{"nest-status"})
	if findErr != nil {
		t.Fatalf("failed finding nest-status: %v", findErr)
	}
	if len(statusFindArgs) > 0 {
		t.Fatalf("unexpected trailing args finding nest-status: %v", statusFindArgs)
	}
	cli.SetContext(nestStatusCmd, cliCtx)
	statusOutBuf := new(bytes.Buffer)
	nestStatusCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), statusOutBuf))
	nestStatusCmd.SetOut(statusOutBuf)

	err = nestStatusCmd.Flags().Set("parent-session-id", "CVS-PARENT-001")
	if err != nil {
		t.Fatalf("failed setting parent-session-id for nest-status: %v", err)
	}
	err = nestStatusCmd.Flags().Set("max-depth", "3")
	if err != nil {
		t.Fatalf("failed setting max-depth for nest-status: %v", err)
	}

	runErr = nestStatusCmd.RunE(nestStatusCmd, []string{})
	if runErr != nil {
		t.Fatalf("nest-status RunE failed: %v", runErr)
	}
	if statusOutBuf.Len() == 0 {
		t.Fatalf("expected non-empty output from nest-status")
	}

	// 6. Test negative case: nest-status on nonexistent parent session
	nonexistentStatusCmd, nonexistFindArgs, findErr := topCmd.Find([]string{"nest-status"})
	if findErr != nil {
		t.Fatalf("failed finding nest-status for negative test: %v", findErr)
	}
	if len(nonexistFindArgs) > 0 {
		t.Fatalf("unexpected trailing args finding nest-status: %v", nonexistFindArgs)
	}
	cli.SetContext(nonexistentStatusCmd, cliCtx)
	err = nonexistentStatusCmd.Flags().Set("parent-session-id", "CVS-NONEXISTENT")
	if err != nil {
		t.Fatalf("failed setting parent-session-id: %v", err)
	}
	runErr = nonexistentStatusCmd.RunE(nonexistentStatusCmd, []string{})
	if runErr == nil {
		t.Fatalf("expected nest-status to fail on nonexistent parent")
	}
}

func TestConvergenceRecordCvsOrchestrateRun_Contract(t *testing.T) {
	cmd := convergence.NewConvergenceCmd()
	if cmd == nil {
		t.Fatalf("expected non-nil convergence command")
	}

	recordCmd, recordFindArgs, findErr := cmd.Find([]string{"record-cvs-orchestrate-run"})
	if findErr != nil {
		t.Fatalf("failed finding record-cvs-orchestrate-run: %v", findErr)
	}
	if len(recordFindArgs) > 0 {
		t.Fatalf("unexpected trailing args finding record-cvs-orchestrate-run: %v", recordFindArgs)
	}
	if recordCmd == nil {
		t.Fatalf("expected record-cvs-orchestrate-run command to be non-nil")
	}
	if recordCmd.RunE == nil {
		t.Fatalf("expected record-cvs-orchestrate-run RunE to be wired")
	}
}
