package scheduler

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	schedulerpkg "github.com/zqk-os/zqk/pkg/scheduler"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// registerSubmitFlags mirrors the flags submitJob reads from the generated
// command builder, so unit tests can drive submitJob without the full CLI tree.
func registerSubmitFlags(cmd *cobra.Command) {
	cmd.Flags().Int("max-runtime", 3600, "")
	cmd.Flags().String("workdir", "", "")
	// Match generated builder: StringArray. Using StringSlice here previously
	// hid the production bug where GetStringSlice dropped live --env values.
	cmd.Flags().StringArray("env", nil, "")
	cmd.Flags().Int("retry", 0, "")
	cmd.Flags().Int("retry-delay", 5, "")
	cmd.Flags().String("title", "", "")
	cmd.Flags().String("description", "", "")
	cmd.Flags().String("callback-completion", "", "")
	cmd.Flags().String("callback-failure", "", "")
}

// TestSubmitJobEnqueuesTriggerForNewJob guards the regression where submitJob
// created the job object for a fresh ID but never enqueued a trigger request.
// The daemon only runs immediate jobs it is told about, so the job silently sat
// unexecuted: no run history, no metrics, and no callback.
func TestSubmitJobEnqueuesTriggerForNewJob(t *testing.T) {
	testRoot, cliCtx, cmd := setupTestEnvironment(t)
	registerSubmitFlags(cmd)

	if err := submitJob(cliCtx, cmd, []string{"echo hello"}); err != nil {
		t.Fatalf("submitJob failed: %v", err)
	}

	requests, err := schedulerpkg.NewJobTriggerQueue(testRoot).PeekTriggerRequests()
	if err != nil {
		t.Fatalf("Failed to peek trigger queue: %v", err)
	}
	if len(requests) == 0 {
		t.Fatal("submitJob created the job but enqueued no trigger request; the daemon would never execute it")
	}
	if requests[0].TriggerOrigin != schedulerpkg.TriggerOriginCLISubmit {
		t.Errorf("TriggerOrigin = %q, want %q", requests[0].TriggerOrigin, schedulerpkg.TriggerOriginCLISubmit)
	}
}

// TestSubmitJobPersistsCallbacks guards the regression where --callback-completion
// and --callback-failure were accepted and documented but dropped, so run_wrapper
// had nothing to invoke when the job finished.
func TestSubmitJobPersistsCallbacks(t *testing.T) {
	testRoot, cliCtx, cmd := setupTestEnvironment(t)
	registerSubmitFlags(cmd)

	const (
		completionCallback = "tee -a completion.json"
		failureCallback    = "tee -a failure.json"
	)
	if err := cmd.Flags().Set("callback-completion", completionCallback); err != nil {
		t.Fatalf("Failed to set callback-completion: %v", err)
	}
	if err := cmd.Flags().Set("callback-failure", failureCallback); err != nil {
		t.Fatalf("Failed to set callback-failure: %v", err)
	}

	if err := submitJob(cliCtx, cmd, []string{"echo hello"}); err != nil {
		t.Fatalf("submitJob failed: %v", err)
	}

	storageFactory, err := storagepkg.NewStorageFactory(pkgctx.NewSystemContext(), testRoot)
	if err != nil {
		t.Fatalf("Failed to create storage factory: %v", err)
	}

	result, err := storageFactory.GetStorage().List(pkgctx.NewSystemContext(), pkgctx.NewSystemSecurityContext(), pkgctx.NewStorageContext(), storagepkg.ListFilter{
		Kind:  getSchedulerJobKind(),
		Limit: 50,
	})
	if err != nil {
		t.Fatalf("Failed to list scheduler jobs: %v", err)
	}

	var found bool
	for _, job := range result.Objects {
		if job[objects.FieldKeyCallbackOnCompletion] != completionCallback {
			continue
		}
		found = true
		if got := job[objects.FieldKeyCallbackOnError]; got != failureCallback {
			t.Errorf("callback_on_error = %v, want %q", got, failureCallback)
		}
	}
	if !found {
		t.Errorf("No submitted job carried callback_on_completion=%q; callbacks were dropped", completionCallback)
	}
}

func TestNewSubmitJobIDDistinguishesSameSecondSubmissions(t *testing.T) {
	first := time.Unix(1_786_864_856, 1).UTC()
	second := time.Unix(1_786_864_856, 2).UTC()

	if firstID, secondID := newSubmitJobID(first), newSubmitJobID(second); firstID == secondID {
		t.Fatalf("same-second submissions collided at %q", firstID)
	}
}

// TestSubmitJobPersistsEnvironmentVariables guards the StringArray/--env path:
// generated builders register StringArray, and GetStringSlice silently drops values.
func TestSubmitJobPersistsEnvironmentVariables(t *testing.T) {
	testRoot, cliCtx, cmd := setupTestEnvironment(t)
	registerSubmitFlags(cmd)

	if err := cmd.Flags().Set("env", "ZQK_LLM_PROVIDER=openai"); err != nil {
		t.Fatalf("Failed to set env provider: %v", err)
	}
	if err := cmd.Flags().Set("env", "ZQK_LLM_API_KEY=ollama"); err != nil {
		t.Fatalf("Failed to set env api key: %v", err)
	}

	if err := submitJob(cliCtx, cmd, []string{"echo hello"}); err != nil {
		t.Fatalf("submitJob failed: %v", err)
	}

	storageFactory, err := storagepkg.NewStorageFactory(pkgctx.NewSystemContext(), testRoot)
	if err != nil {
		t.Fatalf("Failed to create storage factory: %v", err)
	}

	result, err := storageFactory.GetStorage().List(pkgctx.NewSystemContext(), pkgctx.NewSystemSecurityContext(), pkgctx.NewStorageContext(), storagepkg.ListFilter{
		Kind:  getSchedulerJobKind(),
		Limit: 50,
	})
	if err != nil {
		t.Fatalf("Failed to list scheduler jobs: %v", err)
	}

	var found bool
	for _, job := range result.Objects {
		raw, ok := job[objects.FieldKeyEnvironmentVariables].(map[string]any)
		if !ok || raw["ZQK_LLM_PROVIDER"] != "openai" {
			continue
		}
		found = true
		if raw["ZQK_LLM_API_KEY"] != "ollama" {
			t.Errorf("ZQK_LLM_API_KEY = %v, want ollama", raw["ZQK_LLM_API_KEY"])
		}
	}
	if !found {
		t.Fatal("submitted job missing environment_variables; --env was dropped")
	}
}

func TestSubmitJobSetsHighPriority(t *testing.T) {
	testRoot, cliCtx, cmd := setupTestEnvironment(t)
	registerSubmitFlags(cmd)

	if err := submitJob(cliCtx, cmd, []string{"echo hello"}); err != nil {
		t.Fatalf("submitJob failed: %v", err)
	}

	storageFactory, err := storagepkg.NewStorageFactory(pkgctx.NewSystemContext(), testRoot)
	if err != nil {
		t.Fatalf("Failed to create storage factory: %v", err)
	}

	result, err := storageFactory.GetStorage().List(pkgctx.NewSystemContext(), pkgctx.NewSystemSecurityContext(), pkgctx.NewStorageContext(), storagepkg.ListFilter{
		Kind:  getSchedulerJobKind(),
		Limit: 50,
	})
	if err != nil {
		t.Fatalf("Failed to list scheduler jobs: %v", err)
	}

	var found bool
	for _, job := range result.Objects {
		id, _ := job[objects.FieldKeyID].(string)
		if id == emptyValue {
			continue
		}
		found = true
		if got := job[objects.FieldKeyPriority]; got != schedulerpkg.JobPriorityHigh {
			t.Errorf("priority = %v, want %q", got, schedulerpkg.JobPriorityHigh)
		}
		if got := job[objects.FieldKeyCategory]; got != schedulerpkg.CategoryManual {
			t.Errorf("category = %v, want %q", got, schedulerpkg.CategoryManual)
		}
	}
	if !found {
		t.Fatal("no submitted job found to assert priority=high")
	}
}

func TestEnqueueJobTriggerRequest_SetsCLISubmitOrigin(t *testing.T) {
	testRoot, cliCtx, cmd := setupTestEnvironment(t)
	t.Setenv(zqkenv.TestRoot().Name(), testRoot)
	// Write PID file for running scheduler daemon
	pidFile := paths.SchedulerPIDFilePath(testRoot)
	if err := os.MkdirAll(filepath.Dir(pidFile), 0750); err != nil {
		t.Fatalf("mkdir scheduler dir: %v", err)
	}
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0600); err != nil {
		t.Fatalf("write pid file: %v", err)
	}

	// 1. Default (preCommit = false) should carry TriggerOriginCLISubmit
	if err := enqueueJobTriggerRequest(cliCtx, cmd, "SCH-TEST-CLI-001", false); err != nil {
		t.Fatalf("enqueueJobTriggerRequest failed: %v", err)
	}

	queue := schedulerpkg.NewJobTriggerQueue(testRoot)
	requests, err := queue.PeekTriggerRequests()
	if err != nil {
		t.Fatalf("PeekTriggerRequests failed: %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(requests))
	}
	if requests[0].TriggerOrigin != schedulerpkg.TriggerOriginCLISubmit {
		t.Errorf("TriggerOrigin = %q, want %q", requests[0].TriggerOrigin, schedulerpkg.TriggerOriginCLISubmit)
	}

	// 2. PreCommit (preCommit = true) should carry TriggerOriginPreCommit
	if err := enqueueJobTriggerRequest(cliCtx, cmd, "SCH-TEST-CLI-002", true); err != nil {
		t.Fatalf("enqueueJobTriggerRequest with preCommit failed: %v", err)
	}

	requests, err = queue.PeekTriggerRequests()
	if err != nil {
		t.Fatalf("PeekTriggerRequests failed: %v", err)
	}
	if len(requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(requests))
	}
	if requests[1].TriggerOrigin != schedulerpkg.TriggerOriginPreCommit {
		t.Errorf("TriggerOrigin = %q, want %q", requests[1].TriggerOrigin, schedulerpkg.TriggerOriginPreCommit)
	}
}
