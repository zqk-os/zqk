package object

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/execwrap"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/scheduler"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestOfflineQueuingForLifecycleEvents(t *testing.T) {
	// Not t.Parallel() because it sets process-global ZQK_TEST_ROOT
	testEnv := SetupTestEnvironment(t)
	tmpDir := testEnv.GetTestRoot()
	cliBinary := testEnv.CLIBinary

	// 1. Create a goal in CAS visible state so that the backlog item's goal_ref validation passes
	fs, err := storage.NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	cliCtx := storage.WithCLIOperation(ctx)
	storage.CreateCASVisible(t, fs, cliCtx, secCtx, map[string]any{
		objects.FieldKeyID:          "GOAL-REDACTED",
		objects.FieldKeyKind:        objects.KindGoal,
		objects.FieldKeyTitle:       "Test Goal",
		objects.FieldKeyDescription: "A test goal description that is long enough to pass validation checks",
		objects.FieldKeyStatus:      objects.ObjectStatusActive,
		objects.FieldKeyMetric:      "system-security-compliance",
		objects.FieldKeyTarget:      "100",
	}, objects.ObjectStatusActive)

	// 2. Create the backlog item in exploring status (draft plane; List filters will not see it).
	bliYAML := `id: BLI-REDACTED
kind: backlog_item
title: Test Backlog Item
description: A test backlog item description that is long enough to pass CAS boundary validation.
status: exploring
problem_statement: This is a valid problem statement of sufficient length for validation.
acceptance_considerations: Narrative only; gates are criteria_refs.
goal_refs:
  - GOAL-REDACTED
`
	bliFile := filepath.Join(tmpDir, "backlog_item.yaml")
	if err := fileutil.WriteFile(bliFile, []byte(bliYAML), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write backlog item YAML: %v", err)
	}

	cmdCreateBLI := execwrap.Command(cliBinary, "object", "create", "backlog_item",
		"--file", bliFile,
		"--relaxed",
	)
	wireExecForTest(cmdCreateBLI, tmpDir)
	if out, err := cmdCreateBLI.CombinedOutput(); err != nil {
		t.Fatalf("Failed to create backlog item: %v\nOutput: %s", err, string(out))
	}

	// 3. Promote exploring → validated by ID (draft-plane objects are not List-filterable).
	// This must succeed even when the scheduler daemon is stopped.
	cmdPromote := execwrap.Command(cliBinary, "object", "promote", "BLI-REDACTED",
		"--allow-degraded",
	)
	wireExecForTest(cmdPromote, tmpDir)
	out, err := cmdPromote.CombinedOutput()
	if err != nil {
		var logInfo string
		// Read backlog item files
		backlogDir := filepath.Join(tmpDir, paths.ProcessBacklogDir)
		if entries, readErr := fileutil.ReadDir(backlogDir); readErr == nil {
			for _, entry := range entries {
				if !entry.IsDir() && filepath.Ext(entry.Name()) == ".yaml" {
					content, _ := fileutil.ReadFile(filepath.Join(backlogDir, entry.Name()))
					logInfo += fmt.Sprintf("=== Backlog item file: %s ===\n%s\n", entry.Name(), string(content))
				}
			}
		}
		draftDir := filepath.Join(tmpDir, ".zqk", "object_drafts", "backlog_item")
		if entries, readErr := fileutil.ReadDir(draftDir); readErr == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				content, _ := fileutil.ReadFile(filepath.Join(draftDir, entry.Name()))
				logInfo += fmt.Sprintf("=== Draft backlog item file: %s ===\n%s\n", entry.Name(), string(content))
			}
		}
		t.Fatalf("Failed to execute offline promote: %v\nOutput: %s\nBacklog items:\n%s", err, string(out), logInfo)
	}
	// Print backlog item files and logs to diagnose
	{
		var info strings.Builder
		backlogDir := filepath.Join(tmpDir, paths.ProcessBacklogDir)
		if entries, readErr := fileutil.ReadDir(backlogDir); readErr == nil {
			for _, entry := range entries {
				if !entry.IsDir() && filepath.Ext(entry.Name()) == ".yaml" {
					content, _ := fileutil.ReadFile(filepath.Join(backlogDir, entry.Name()))
					info.WriteString(fmt.Sprintf("=== Backlog item file: %s ===\n%s\n", entry.Name(), string(content)))
				}
			}
		}
		logsDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.LogsDir)
		if entries, readErr := fileutil.ReadDir(logsDir); readErr == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					content, _ := fileutil.ReadFile(filepath.Join(logsDir, entry.Name()))
					info.WriteString(fmt.Sprintf("=== Log file: %s ===\n%s\n", entry.Name(), string(content)))
				}
			}
		}
		t.Logf("State after offline promote:\nCommand Output:\n%s\n%s", string(out), info.String())
	}
	// 4. Verify that a lifecycle event is enqueued in .zqk/scheduler/triggers/queue.json
	queuePath := filepath.Join(tmpDir, paths.ProjectDataDir, paths.SchedulerDir, "triggers", "queue.json")
	data, err := fileutil.ReadFile(queuePath)
	if err != nil {
		t.Fatalf("Failed to read trigger queue file: %v", err)
	}

	var requests []scheduler.JobTriggerRequest
	if err := json.Unmarshal(data, &requests); err != nil {
		t.Fatalf("Failed to unmarshal trigger queue: %v\nData: %s", err, string(data))
	}

	if len(requests) == 0 {
		t.Fatal("Trigger queue is empty, expected offline lifecycle trigger request to be enqueued")
	}

	found := false
	for _, req := range requests {
		if req.IsLifecycleTrigger &&
			req.LifecycleKind == "backlog_item" &&
			req.LifecycleFrom == "exploring" &&
			req.LifecycleTo == "validated" {
			found = true
			bliID, _ := req.LifecycleData[objects.FieldKeyID].(string)
			if bliID != "BLI-REDACTED" {
				t.Errorf("LifecycleData ID mismatch: got %s, want BLI-REDACTED", bliID)
			}
			break
		}
	}

	if !found {
		t.Errorf("Expected lifecycle trigger request (exploring -> validated) not found in queue: %+v", requests)
	}

	// 5. Start the scheduler daemon (hard lifetime bound + Cleanup stop --force) and verify drain.
	schedHandle := testkit.StartBoundCLIScheduler(t, testkit.BoundCLISchedulerOpts{
		CLIBinary:   cliBinary,
		ProjectRoot: tmpDir,
		Env:         EnvWithTestRoot(tmpDir),
		// Drain should finish well under the default 3m ceiling; keep a short poll window below.
		MaxLifetime: 30 * time.Second,
	})

	// Poll until the queue is drained (i.e. contains no lifecycle requests or becomes empty/deleted)
	success := false
	pollCap := time.NewTimer(10 * time.Second)
	defer pollCap.Stop()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for !success {
		select {
		case <-schedHandle.BoundCtx.Done():
			t.Error("scheduler daemon bound expired before trigger queue drained")
			return
		case <-pollCap.C:
			t.Error("Trigger queue was not successfully drained by the scheduler daemon within timeout")
			return
		case <-ticker.C:
			data, err := fileutil.ReadFile(queuePath)
			if err != nil {
				if fileutil.IsNotExist(err) {
					// Queue file deleted, which means it was successfully drained
					success = true
					continue
				}
				t.Fatalf("Failed to poll queue file: %v", err)
			}

			var currentRequests []scheduler.JobTriggerRequest
			if err := json.Unmarshal(data, &currentRequests); err != nil {
				// File might be in the middle of being written, ignore and retry
				continue
			}

			hasLifecycle := false
			for _, req := range currentRequests {
				if req.IsLifecycleTrigger && req.LifecycleKind == "backlog_item" {
					hasLifecycle = true
					break
				}
			}
			if !hasLifecycle {
				success = true
			}
		}
	}
}
