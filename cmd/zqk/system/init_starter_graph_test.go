package system

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lanceman/zqk/cmd/zqk/workflow"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage/filecas"
	"github.com/lanceman/zqk/pkg/testkit"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/workflow/whatsnext"
)

func TestInit_Greenfield_StarterKernelGraph(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		SkipSetupTestEnvironment: true,
		SkipFileStorage:          true,
	})
	tmpDir := proj.Root

	originalDir, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer fileutil.Chdir(originalDir)

	if err := fileutil.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	entries, err := fileutil.ReadDir(tmpDir)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	t.Logf("before init, tmpDir entries: %v (err: %v)", names, err)

	cmd := NewInitCmd()
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Greenfield init failed: %v", err)
	}

	// Verify starter kernel objects exist in CAS
	kinds := []string{
		objects.KindOrganization,
		objects.KindMission,
		objects.KindVision,
		objects.KindGoal,
		objects.KindWorkstream,
		objects.KindPriorityPlan,
		objects.KindRequirement,
		objects.KindCriteria,
		objects.KindBacklogItem,
	}

	for _, k := range kinds {
		dir := filepath.Join(tmpDir, paths.ProcessDir, objects.GetDirectoryFromKind(k))
		cas := filecas.NewContentAddressableStorage(dir, k)
		ids, err := cas.ListIDs()
		t.Logf("k=%s dir=%s ids=%v err=%v", k, dir, ids, err)
		if err != nil || len(ids) == 0 {
			t.Errorf("expected starter graph object of kind %s in %s, found 0 (err: %v)", k, dir, err)
			continue
		}

		// Verify created_by is SystemAccountID, not ACC-TEST-HARNESS
		for _, id := range ids {
			data, err := cas.Read(id)
			if err != nil {
				t.Errorf("failed to read object %s: %v", id, err)
				continue
			}
			content := string(data)
			if !strings.Contains(content, "created_by: "+pkgctx.SystemAccountID) &&
				!strings.Contains(content, "created_by: \""+pkgctx.SystemAccountID+"\"") {
				t.Errorf("object %s (%s) created_by must be %s", id, k, pkgctx.SystemAccountID)
			}
		}
	}

	// CRIT-INIT-STARTER-GRAPH-002: Verify workflow whats-next succeeds immediately
	payload, err := whatsnext.GetOrRecoverPayload(context.Background(), nil, tmpDir, 2*time.Minute)
	if err != nil {
		t.Fatalf("whats-next payload recovery failed: %v", err)
	}
	if payload.LeadPlan == nil || payload.LeadPlan.ID != "PRI-STARTER-COMMUNITY-001" {
		t.Fatalf("expected lead plan PRI-STARTER-COMMUNITY-001, got %v", payload.LeadPlan)
	}

	wnCmd := workflow.NewWhatsNextCmd()
	wnCmd.SetArgs([]string{"--format", "json"})
	if err := wnCmd.Execute(); err != nil {
		t.Fatalf("whats-next failed following greenfield init: %v", err)
	}
}

