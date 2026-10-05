package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/zqk-os/zqk/pkg/audit"
	"github.com/zqk-os/zqk/pkg/brand"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/mutation"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testenvroot"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestBuildContextBundle(t *testing.T) {
	task := map[string]any{
		objects.FieldKeyID:          "task-123",
		objects.FieldKeyKind:        objects.KindAgentTask,
		objects.FieldKeyTitle:       "Test Task",
		objects.FieldKeyDescription: "Do something",
	}

	deps := []map[string]any{
		{
			objects.FieldKeyID:   "dep-1",
			objects.FieldKeyKind: objects.KindTechnicalSpec,
		},
		{
			objects.FieldKeyID:   "dep-2",
			objects.FieldKeyKind: objects.KindCodeFile,
		},
	}

	bundle := buildContextBundle(task, deps)

	if bundle.Task == nil {
		t.Error("Expected task in bundle")
	}

	if len(bundle.Dependencies) != 2 {
		t.Errorf("Expected 2 dependencies in bundle, got %v", bundle.Dependencies)
	}
}

func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	entries, err := fileutil.ReadDir(src)
	if err != nil {
		t.Fatalf("failed to read src dir %s: %v", src, err)
	}
	if err := fileutil.EnsureDir(dst); err != nil {
		t.Fatalf("failed to create dst dir %s: %v", dst, err)
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			copyDir(t, srcPath, dstPath)
		} else {
			data, err := fileutil.ReadFile(srcPath)
			if err != nil {
				t.Fatalf("failed to read file %s: %v", srcPath, err)
			}
			if err := fileutil.WriteSecureFile(dstPath, data); err != nil {
				t.Fatalf("failed to write file %s: %v", dstPath, err)
			}
		}
	}
}

func setupSyncLoopTestProject(t *testing.T) (string, storage.ObjectStorageProvider) {
	t.Helper()
	brand.SetExecutableName("zqk")
	storage.SetGraphConnectionProvider(nil)

	origMapper := objects.GetGlobalKindMapper()
	origProcessDir, origSpecsDir := origMapper.GetDirectories()
	t.Cleanup(func() {
		origMapper.SetDirectories(origProcessDir, origSpecsDir)
		_ = origMapper.Reload()
	})

	cwd, _ := fileutil.Getwd()
	repoRoot := filepath.Join(cwd, "..", "..", "..")
	project := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind:            "agent_sync_loop",
		SkipFileStorage: true,
		AppendStagesBeforeStorage: func(root string) []testkit.NamedTestStep {
			return []testkit.NamedTestStep{{
				Name: "seed_agent_sync_loop_project",
				Fn: func() error {
					if err := paths.EnsureProcessAndObjectSpecsLayout(root); err != nil {
						return err
					}
					for _, dir := range []string{"agent_tasks", "assessment_ratings"} {
						if err := fileutil.EnsureDir(filepath.Join(root, paths.ProcessDir, dir)); err != nil {
							return err
						}
					}

					srcSpecs := filepath.Join(repoRoot, paths.ProcessInternalObjectSpecsDir)
					dstSpecs := filepath.Join(root, paths.ProcessInternalObjectSpecsDir)
					if err := fileutil.RemoveAll(dstSpecs); err != nil {
						return err
					}
					copyDir(t, srcSpecs, dstSpecs)
					workSpecs := filepath.Join(repoRoot, "packs", "work", "specs")
					if entries, err := fileutil.ReadDir(workSpecs); err == nil && len(entries) > 0 {
						copyDir(t, workSpecs, dstSpecs)
					}
					agentSpecs := filepath.Join(repoRoot, "packs", "agent", "specs")
					if entries, err := fileutil.ReadDir(agentSpecs); err == nil && len(entries) > 0 {
						copyDir(t, agentSpecs, dstSpecs)
					}

					srcConfigs := filepath.Join(repoRoot, paths.ProcessInternalConfigsDir)
					dstConfigs := filepath.Join(root, paths.ProcessInternalConfigsDir)
					if err := fileutil.RemoveAll(dstConfigs); err != nil {
						return err
					}
					copyDir(t, srcConfigs, dstConfigs)

					if err := testenvroot.CopyLifecyclesFromProject(root, repoRoot); err != nil {
						return err
					}
					dstLifecycles := filepath.Join(root, paths.ProcessInternalLifecyclesDir)
					workLifecycles := filepath.Join(repoRoot, "packs", "work", "lifecycles")
					if entries, err := fileutil.ReadDir(workLifecycles); err == nil && len(entries) > 0 {
						copyDir(t, workLifecycles, dstLifecycles)
					}
					agentLifecycles := filepath.Join(repoRoot, "packs", "agent", "lifecycles")
					if entries, err := fileutil.ReadDir(agentLifecycles); err == nil && len(entries) > 0 {
						copyDir(t, agentLifecycles, dstLifecycles)
					}
					return nil
				},
			}}
		},
	})
	root := project.Root
	testProcessDir := filepath.Join(root, paths.ProcessDir)
	testSpecsDir := filepath.Join(root, paths.ProcessInternalObjectSpecsDir)
	origMapper.SetDirectories(testProcessDir, testSpecsDir)
	if err := origMapper.Reload(); err != nil {
		t.Fatalf("failed to reload kind mapper: %v", err)
	}
	store, err := storage.GetGlobalStorageProviderCache().GetOrCreate(context.Background(), root)
	if err != nil {
		t.Fatalf("GetOrCreate storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, root, store)

	// Create an agent_skill to satisfy CRIT-PERSONA-SKILL-BOUND
	skillCtx := storage.WithSyncCreateForKind(context.Background(), objects.KindAgentSkill)
	skillCtx = storage.WithSkipWriteBehind(skillCtx)
	skillID := "ASK-ORCH-ALPHA"
	skill := map[string]any{
		objects.FieldKeyID:            skillID,
		objects.FieldKeyKind:          objects.KindAgentSkill,
		objects.FieldKeyTitle:         "Orchestrator Skill",
		objects.FieldKeySchemaVersion: "2.0.0",
		objects.FieldKeyStatus:        objects.ObjectStatusImplemented,
	}
	storage.CreateCASVisible(t, store, skillCtx, pkgctx.NewSystemSecurityContext(), skill, objects.ObjectStatusImplemented)

	// Create the persona PER-ORCH-ALPHA to satisfy reference checks
	personaCtx := storage.WithSyncCreateForKind(context.Background(), "persona")
	personaCtx = storage.WithSkipWriteBehind(personaCtx)
	persona := map[string]any{
		objects.FieldKeyID:             objects.ConstPersonaOrchestratorAlpha,
		objects.FieldKeyKind:           objects.KindPersona,
		objects.FieldKeyTitle:          "Orchestrator Persona",
		objects.FieldKeyName:           "Orchestrator Persona",
		objects.FieldKeyRole:           "agent",
		objects.FieldKeySchemaVersion:  "2.0.0",
		objects.FieldKeyStatus:         objects.ObjectStatusImplemented,
		objects.FieldKeyAgentSkillRefs: []any{skillID},
	}
	storage.CreateCASVisible(t, store, personaCtx, pkgctx.NewSystemSecurityContext(), persona, objects.ObjectStatusImplemented)

	return root, store
}

func TestSyncLoop_MaxVerificationAttempts(t *testing.T) {
	root, store := setupSyncLoopTestProject(t)

	// Resolve the built CLI binary under repository module root and set it to ZQK_BIN
	if wd, err := fileutil.Getwd(); err == nil {
		if modRoot, err := paths.ModuleRootFromPath(wd); err == nil {
			candidate := filepath.Join(modRoot, "bin", "zqk")
			if info, err := fileutil.Stat(candidate); err == nil && !info.IsDir() {
				t.Setenv(zqkenv.Bin().Name(), candidate)
			}
		}
	}

	// Create a task that already has 3 verification attempts on its step
	ctx := storage.WithSyncCreateForKind(context.Background(), objects.KindAgentTask)
	ctx = storage.WithSkipWriteBehind(ctx)
	taskID := "ATK-loop-guard-test"
	task := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Test Loop Guard",
		objects.FieldKeyStatus:             objects.ObjectStatusPendingVerification,
		objects.FieldKeySchemaVersion:      "2.0.0",
		objects.FieldKeyAssigneePersonaRef: objects.ConstPersonaOrchestratorAlpha,
		objects.FieldKeyEstimatedEffort:    "1h",
		objects.FieldKeyTaskSteps: []any{
			map[string]any{
				objects.FieldKeyTitle:                "Step 1",
				objects.FieldKeyDescription:          "Must pass check",
				objects.FieldKeyStatus:               objects.ObjectStatusPendingVerification,
				objects.FieldKeyVerificationAttempts: 3, // Already at 3 attempts!
			},
		},
	}

	storage.CreateCASVisible(t, store, ctx, pkgctx.NewSystemSecurityContext(), task, objects.ObjectStatusPendingVerification)

	// Build the sync loop command
	cmd := NewSyncLoopCmd()
	cmd.SetArgs([]string{taskID})

	// Run the sync loop in a goroutine because it polls, but it should exit on first tick!
	errChan := make(chan error, 1)
	goroutinelabels.NewGoroutine("agent_sync_loop_test", "execute bounded sync loop").StartSimple(func() {
		runCtx := storage.WithSkipWriteBehind(context.Background())
		errChan <- cmd.ExecuteContext(runCtx)
	})

	select {
	case err := <-errChan:
		if err != nil {
			t.Fatalf("runSyncLoop returned error: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Sync loop hung and did not terminate after exceeding max attempts")
	}

	// Flush storage write queue
	writeQueue := caspkg.GetListingIndexWriteQueueForProjectRoot(root)
	if writeQueue != nil {
		_ = writeQueue.FlushAll(1 * time.Second)
	}

	// Verify that the task status in storage was updated to error
	bypassCtx := pkgctx.WithBypassCache(context.Background())
	updatedTask, err := store.Read(bypassCtx, pkgctx.NewSystemSecurityContext(), taskID)
	if err != nil {
		t.Fatalf("failed to read updated task: %v", err)
	}

	status, _ := updatedTask[objects.FieldKeyStatus].(string)
	if status != objects.ObjectStatusFailed {
		t.Errorf("Expected task status to be 'error', got '%s'", status)
	}

	steps, ok := updatedTask[objects.FieldKeyTaskSteps].([]any)
	if !ok || len(steps) == 0 {
		t.Fatalf("Expected task_steps to exist, got %v", updatedTask[objects.FieldKeyTaskSteps])
	}

	step1, ok := steps[0].(map[string]any)
	if !ok {
		t.Fatalf("Expected step 1 to be a map, got %T", steps[0])
	}

	stepStatus, _ := step1[objects.FieldKeyStatus].(string)
	if stepStatus != objects.ObjectStatusFailed {
		t.Errorf("Expected step status to be %q, got %q", objects.ObjectStatusFailed, stepStatus)
	}

	feedback, _ := step1[objects.FieldKeyVerificationFeedback].(string)
	if !strings.Contains(strings.ToLower(feedback), "max verification loop limit reached") {
		t.Errorf("Expected feedback to mention limit, got '%s'", feedback)
	}
}

func TestApplyStateMutation_ErrorPropagation(t *testing.T) {
	root, store := setupSyncLoopTestProject(t)

	ctx := storage.WithSkipWriteBehind(context.Background())
	secCtx := pkgctx.NewSystemSecurityContext()
	taskID := "ATK-err-propagation-test"
	task := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Error Propagation Test Task",
		objects.FieldKeySchemaVersion:      "2.0.0",
		objects.FieldKeyStatus:             objects.ObjectStatusInProgress,
		objects.FieldKeyAssigneePersonaRef: objects.ConstPersonaOrchestratorAlpha,
		objects.FieldKeyEstimatedEffort:    "1h",
	}
	storage.CreateCASVisible(t, store, ctx, secCtx, task, objects.ObjectStatusInProgress)

	validator := mutation.NewValidator(nil)
	auditStream := audit.NewAuditStream(root)

	// An update to a non-existent object or failed transaction must fail closed and propagate an explicit error
	err := applyStateMutation(ctx, secCtx, store, "ATK-nonexistent-missing-id", objects.KindAgentTask, validator, auditStream, objects.ObjectStatusInProgress)
	if err == nil {
		t.Fatal("expected error from applyStateMutation with non-existent object, got nil")
	}
}
