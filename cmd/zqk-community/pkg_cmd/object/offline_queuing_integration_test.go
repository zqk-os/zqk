package object

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/scheduler"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestOfflineQueuingForLifecycleEvents(t *testing.T) {
	// Not t.Parallel() because it sets process-global ZQK_TEST_ROOT
	testEnv := SetupTestEnvironment(t)
	tmpDir := testEnv.GetTestRoot()
	cliBinary := testEnv.CLIBinary

	// 1. Create a goal first so that the backlog item's goal_ref validation passes
	goalYAML := `id: GOAL-REDACTED
kind: goal
title: Test Goal
description: A test goal description that is long enough to pass validation checks
`
	goalFile := filepath.Join(tmpDir, "goal.yaml")
	if err := os.WriteFile(goalFile, []byte(goalYAML), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write goal YAML: %v", err)
	}

	cmdCreateGoal := exec.Command(cliBinary, "object", "create", "goal",
		"--file", goalFile,
		"--relaxed",
	)
	zqkenv.WireExecForIsolatedProject(cmdCreateGoal, tmpDir)
	cmdCreateGoal.Env = EnvWithTestRoot(tmpDir)
	if out, err := cmdCreateGoal.CombinedOutput(); err != nil {
		t.Fatalf("Failed to create goal: %v\nOutput: %s", err, string(out))
	}

	// 2. Create the backlog item in exploring status
	bliYAML := `id: ITEM-REDACTED-750cf486
kind: backlog_item
title: Test Backlog Item
status: exploring
goal_refs:
  - GOAL-REDACTED
`
	bliFile := filepath.Join(tmpDir, "backlog_item.yaml")
	if err := os.WriteFile(bliFile, []byte(bliYAML), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write backlog item YAML: %v", err)
	}

	cmdCreateBLI := exec.Command(cliBinary, "object", "create", "backlog_item",
		"--file", bliFile,
		"--relaxed",
	)
	zqkenv.WireExecForIsolatedProject(cmdCreateBLI, tmpDir)
	cmdCreateBLI.Env = EnvWithTestRoot(tmpDir)
	if out, err := cmdCreateBLI.CombinedOutput(); err != nil {
		t.Fatalf("Failed to create backlog item: %v\nOutput: %s", err, string(out))
	}

	// 3. Perform bulk update to change status from exploring -> validated.
	// This command must succeed even when the scheduler daemon is stopped.
	cmdBulkUpdate := exec.Command(cliBinary, "object", "bulk", "update", "backlog_item",
		"--filter", "id=ITEM-REDACTED-750cf486",
		"--set", "status=validated",
		"--relaxed",
	)
	zqkenv.WireExecForIsolatedProject(cmdBulkUpdate, tmpDir)
	cmdBulkUpdate.Env = EnvWithTestRoot(tmpDir)
	out, err := cmdBulkUpdate.CombinedOutput()
	if err != nil {
		var logInfo string
		// Read backlog item files
		backlogDir := filepath.Join(tmpDir, "docs", "process", "backlog")
		if entries, readErr := os.ReadDir(backlogDir); readErr == nil {
			for _, entry := range entries {
				if !entry.IsDir() && filepath.Ext(entry.Name()) == ".yaml" {
					content, _ := os.ReadFile(filepath.Join(backlogDir, entry.Name()))
					logInfo += fmt.Sprintf("=== Backlog item file: %s ===\n%s\n", entry.Name(), string(content))
				}
			}
		}
		t.Fatalf("Failed to execute offline bulk update: %v\nOutput: %s\nBacklog items:\n%s", err, string(out), logInfo)
	}
	// Print backlog item files and logs to diagnose
	{
		var info strings.Builder
		backlogDir := filepath.Join(tmpDir, "docs", "process", "backlog")
		if entries, readErr := os.ReadDir(backlogDir); readErr == nil {
			for _, entry := range entries {
				if !entry.IsDir() && filepath.Ext(entry.Name()) == ".yaml" {
					content, _ := os.ReadFile(filepath.Join(backlogDir, entry.Name()))
					info.WriteString(fmt.Sprintf("=== Backlog item file: %s ===\n%s\n", entry.Name(), string(content)))
				}
			}
		}
		logsDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.LogsDir)
		if entries, readErr := os.ReadDir(logsDir); readErr == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					content, _ := os.ReadFile(filepath.Join(logsDir, entry.Name()))
					info.WriteString(fmt.Sprintf("=== Log file: %s ===\n%s\n", entry.Name(), string(content)))
				}
			}
		}
		t.Logf("State after bulk update:\nCommand Output:\n%s\n%s", string(out), info.String())
	}
	// 4. Verify that a lifecycle event is enqueued in .zqk/scheduler/triggers/queue.json
	queuePath := filepath.Join(tmpDir, paths.ProjectDataDir, paths.SchedulerDir, "triggers", "queue.json")
	data, err := os.ReadFile(queuePath)
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
			if bliID != "ITEM-REDACTED-750cf486" {
				t.Errorf("LifecycleData ID mismatch: got %s, want ITEM-REDACTED-750cf486", bliID)
			}
			break
		}
	}

	if !found {
		t.Errorf("Expected lifecycle trigger request (exploring -> validated) not found in queue: %+v", requests)
	}

	// 5. Start the scheduler daemon and verify it drains and processes the queued trigger
	startCmd := exec.Command(cliBinary, "scheduler", "start", "--test-id="+t.Name())
	zqkenv.WireExecForIsolatedProject(startCmd, tmpDir)
	startCmd.Env = EnvWithTestRoot(tmpDir)
	if out, err := startCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to start scheduler daemon: %v\nOutput: %s", err, string(out))
	}

	defer func() {
		stopCmd := exec.Command(cliBinary, "scheduler", "stop", "--test-id="+t.Name(), "--force")
		zqkenv.WireExecForIsolatedProject(stopCmd, tmpDir)
		stopCmd.Env = EnvWithTestRoot(tmpDir)
		if stopOut, stopErr := stopCmd.CombinedOutput(); stopErr != nil {
			t.Logf("Scheduler cleanup stop failed: %v\nOutput: %s", stopErr, string(stopOut))
		}
	}()

	// Poll until the queue is drained (i.e. contains no lifecycle requests or becomes empty/deleted)
	success := false
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)

		data, err := os.ReadFile(queuePath)
		if err != nil {
			if os.IsNotExist(err) {
				// Queue file deleted, which means it was successfully drained
				success = true
				break
			}
			t.Fatalf("Failed to poll queue file: %v", err)
		}

		var currentRequests []scheduler.JobTriggerRequest
		if err := json.Unmarshal(data, &currentRequests); err != nil {
			// File might be in the middle of being written, ignore and retry
			continue
		}

		// Check if the lifecycle request has been removed
		hasLifecycle := false
		for _, req := range currentRequests {
			if req.IsLifecycleTrigger && req.LifecycleKind == "backlog_item" {
				hasLifecycle = true
				break
			}
		}

		if !hasLifecycle {
			success = true
			break
		}
	}

	if !success {
		t.Error("Trigger queue was not successfully drained by the scheduler daemon within timeout")
	}
}
