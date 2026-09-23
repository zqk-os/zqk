package swarm

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/cmd/zqk/state"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/swarm/pack"
	"github.com/zqk-os/zqk/pkg/swarm/remote"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestRemoteSwarmAndSeismographDashboard verifies CRIT-STARTER-COMMUNITY-030-01, 02, and 03.
func TestRemoteSwarmAndSeismographDashboard(t *testing.T) {
	// 1. Verify Remote Swarm Resolution & Execution
	cacheDir := t.TempDir()
	slug := "expert-team/autonomous-refactor"
	destDir := filepath.Join(cacheDir, filepath.FromSlash(slug))
	if err := fileutil.MkdirAll(destDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create cache destDir: %v", err)
	}

	manifestContent := `schema_version: 1.0.0
name: expert-refactor-swarm
version: 1.2.0
description: Autonomous multi-agent refactoring swarm
license: Apache-2.0
entrypoint: pipeline
agents:
  - name: planner
    role: architecture_planner
    skills:
      - go-architect
      - ast-rewriter
  - name: builder
    role: code_craftsman
membranes:
  - path: .zqk/process/
    mode: disallow
tasks:
  - id: plan
    title: Create Refactoring Plan
  - id: execute
    title: Run Parallel Builders
    depends_on:
      - plan
`
	manifestPath := filepath.Join(destDir, "swarm.yaml")
	if err := fileutil.WriteFile(manifestPath, []byte(manifestContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write swarm.yaml: %v", err)
	}

	// Resolve from cache
	ctx := context.Background()
	opts := remote.ResolveOptions{
		CacheDir: cacheDir,
		Timeout:  5 * time.Second,
	}

	resolved, err := remote.Resolve(ctx, "github.com/expert-team/autonomous-refactor", opts)
	if err != nil {
		t.Fatalf("remote.Resolve failed: %v", err)
	}
	if resolved != manifestPath {
		t.Errorf("expected resolved %s, got %s", manifestPath, resolved)
	}

	// Load & validate manifest
	pkg, err := pack.LoadManifestFile(resolved)
	if err != nil {
		t.Fatalf("failed to load resolved manifest: %v", err)
	}
	if pkg.Name != "expert-refactor-swarm" || len(pkg.Agents) != 2 || len(pkg.Tasks) != 2 {
		t.Errorf("unexpected swarm package content: %+v", pkg)
	}

	// Run swarm via CLI runner
	runCmd := NewTopLevelRunCmd()
	buf := new(bytes.Buffer)
	runCmd.SetOut(buf)
	runCmd.SetErr(buf)
	runCmd.SetArgs([]string{manifestPath, "--dry-run"})

	if err := runCmd.Execute(); err != nil {
		t.Fatalf("runCmd.Execute failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "expert-refactor-swarm v1.2.0") {
		t.Errorf("expected swarm name in output, got:\n%s", output)
	}
	if !strings.Contains(output, "architecture_planner") {
		t.Errorf("expected agent role in output, got:\n%s", output)
	}
	if !strings.Contains(output, "disallow") {
		t.Errorf("expected membrane mode in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Swarm validation successful") {
		t.Errorf("expected dry-run success message, got:\n%s", output)
	}

	// 2. Verify Seismograph Dashboard Telemetry Rendering
	stateDir := t.TempDir()
	streamDir := filepath.Join(stateDir, paths.ProjectDataDir, "streams")
	if err := fileutil.MkdirAll(streamDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create streams dir: %v", err)
	}

	m1 := state.JournalMutation{
		ID:          "MUT-100",
		ChangeType:  "create",
		ObjectRef:   "backlog_item:BLI-030",
		DiffSummary: "Implement remote git swarm distribution",
		CreatedAt:   time.Now().Unix(),
		CreatedBy:   "ACC-AGENT-1",
	}
	m2 := state.JournalMutation{
		ID:          "MUT-101",
		ChangeType:  "update",
		ObjectRef:   "criteria:CRIT-030",
		DiffSummary: "Validate terminal dashboard telemetry",
		CreatedAt:   time.Now().Unix() + 1,
		CreatedBy:   "ACC-AGENT-2",
	}

	dashOutput := state.BuildDashboardView(stateDir, []state.JournalMutation{m1, m2})
	if !strings.Contains(dashOutput, "ZQK STATE SEISMOGRAPH & TELEMETRY DASHBOARD") {
		t.Errorf("expected dashboard title banner in output, got:\n%s", dashOutput)
	}
	if !strings.Contains(dashOutput, "CPCP-MEMBRANE-001 (FAIL-CLOSED)") {
		t.Errorf("expected CPCP membrane indicator in dashboard, got:\n%s", dashOutput)
	}
	if !strings.Contains(dashOutput, "BLI-030") || !strings.Contains(dashOutput, "CRIT-030") {
		t.Errorf("expected object refs in dashboard table, got:\n%s", dashOutput)
	}
	if !strings.Contains(dashOutput, "[CPCP: PASS]") {
		t.Errorf("expected CPCP: PASS badges in dashboard table, got:\n%s", dashOutput)
	}

	// 3. Verify static dashboard stream execution
	dashStreamCmd := &cobra.Command{}
	outBuf := new(bytes.Buffer)
	dashStreamCmd.SetOut(outBuf)
	err = state.StreamJournalMutations(dashStreamCmd, stateDir, false, 10, "", true)
	if err != nil {
		t.Fatalf("StreamJournalMutations with dashboard failed: %v", err)
	}
}
