// Copyright (c) 2026 ZQK Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package community_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/community"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestCAPGovernor_StaticFloorInvariant (CRIT-1794446126954083000-6a3b0a6a)
// Verifies marker creation, validation, activation, deactivation, and filesystem security bounds.
func TestCAPGovernor_StaticFloorInvariant(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "cap_governor_test",
	})
	root := proj.Root

	gov, err := community.NewCAPGovernor(community.CAPGovernorOptions{
		ProjectRoot: root,
		AgentID:     "test-agent-seat-1",
		Vendor:      "antigravity",
	})
	require.NoError(t, err)
	require.NotNil(t, gov)

	expectedMarker := filepath.Join(root, paths.ProjectDataDir, "state", community.CAPLoopMarkerFileName)
	assert.Equal(t, expectedMarker, gov.MarkerPath())

	// Initially inactive
	assert.False(t, gov.IsActive())

	// Activate marker
	require.NoError(t, gov.Activate())
	assert.True(t, gov.IsActive())

	// Verify marker file contents
	content, err := fileutil.ReadFile(gov.MarkerPath())
	require.NoError(t, err)
	assert.Contains(t, string(content), "active:pid=")
	assert.Contains(t, string(content), "agent=test-agent-seat-1")

	// Deactivate marker
	require.NoError(t, gov.Deactivate())
	assert.False(t, gov.IsActive())
}

// TestCAPGovernor_OperationalDynamicProof (CRIT-1794446126954083000-6a3b0a6a)
// Verifies dynamic inspection with active priority plans, auto-healing of missing markers,
// and decision propagation.
func TestCAPGovernor_OperationalDynamicProof(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind:            "cap_governor_test",
		SeedSchemaPlane: true,
	})
	root, fs := proj.Root, proj.FileStorage
	secCtx := pkgctx.NewSystemSecurityContext()

	gov, err := community.NewCAPGovernor(community.CAPGovernorOptions{
		ProjectRoot: root,
		AgentID:     "swarm-coordinator-1",
		Vendor:      "antigravity",
	})
	require.NoError(t, err)

	// Case 1: Inactive system (no marker, no active plan)
	status, err := gov.Inspect(context.Background(), secCtx, fs)
	require.NoError(t, err)
	assert.False(t, status.ShouldContinue)
	assert.False(t, status.MarkerPresent)
	assert.Empty(t, status.ActivePlanID)

	// Case 2: Seed active priority plan in kernel CAS
	pri := map[string]any{
		objects.FieldKeyID:            "PRI-GOV-TEST-107",
		objects.FieldKeyKind:          objects.KindPriorityPlan,
		objects.FieldKeyTitle:         "Cycle 107 Test Plan",
		objects.FieldKeyDescription:   "Advance autonomous progression",
		objects.FieldKeyStatus:        objects.ObjectStatusInProgress,
		objects.FieldKeySchemaVersion: "2.0.0",
	}
	require.NoError(t, fs.Create(context.Background(), secCtx, pri))

	// Dynamic Proof: Inspect detects active plan and auto-heals missing marker
	status, err = gov.Inspect(context.Background(), secCtx, fs)
	require.NoError(t, err)
	assert.True(t, status.ShouldContinue)
	assert.True(t, status.MarkerPresent)
	assert.Equal(t, "PRI-GOV-TEST-107", status.ActivePlanID)
	assert.Contains(t, status.DecisionReason, "Active priority plan in progress")
	assert.True(t, gov.IsActive(), "Marker must be auto-healed when active plan is in progress")
}

// TestCAPGovernor_NegativeBoundaryAndErrorRecovery (CRIT-1794446126954084000-d831b372)
// Verifies rejection of invalid project roots, quarantine of stale markers, and idempotent deactivations.
func TestCAPGovernor_NegativeBoundaryAndErrorRecovery(t *testing.T) {
	// Boundary 1: Empty project root must fail
	if _, badErr := community.NewCAPGovernor(community.CAPGovernorOptions{ProjectRoot: ""}); badErr == nil {
		t.Fatal("expected error on empty project root")
	}

	testDir := t.TempDir()
	gov, initErr := community.NewCAPGovernor(community.CAPGovernorOptions{
		ProjectRoot: testDir,
		AgentID:     "adversarial-edge-seat",
	})
	if initErr != nil {
		t.Fatalf("failed init: %v", initErr)
	}

	// Boundary 2: Idempotent deactivation on nonexistent marker
	if deactErr := gov.Deactivate(); deactErr != nil {
		t.Fatalf("unexpected deact error: %v", deactErr)
	}

	// Boundary 3: Stale marker detection and quarantine
	if actErr := gov.Activate(); actErr != nil {
		t.Fatalf("unexpected act error: %v", actErr)
	}
	if !gov.IsActive() {
		t.Fatal("expected active marker")
	}

	// Backdate the marker file modtime by 48 hours
	staleTime := time.Now().Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(gov.MarkerPath(), staleTime, staleTime))

	// Inspect without active plans should detect stale marker, quarantine it, and allow stop
	secCtx := pkgctx.NewSystemSecurityContext()
	status, inspectErr := gov.Inspect(context.Background(), secCtx, nil)
	if inspectErr != nil {
		t.Fatalf("unexpected inspect error: %v", inspectErr)
	}
	if status.ShouldContinue {
		t.Error("expected shouldContinue to be false for stale marker")
	}
	if status.MarkerPresent {
		t.Error("expected marker to be removed")
	}
	if !status.StaleMarkerClean {
		t.Error("expected staleMarkerClean to be true")
	}
	assert.Contains(t, status.DecisionReason, "Stale CAP loop marker quarantined")
	if gov.IsActive() {
		t.Error("stale marker must be removed from disk")
	}
}
