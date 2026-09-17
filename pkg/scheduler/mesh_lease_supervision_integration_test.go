package scheduler

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
	__exec "os/exec"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/federation"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	_ "github.com/lanceman/zqk/pkg/specbuilder/bldr_v2"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testenvroot"
	"github.com/lanceman/zqk/pkg/testkit"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// writeMeshLeaseDaemonStub installs a long-lived stub at projectRoot/bin/zqk so
// ResolveSchedulerDaemonBinary can Start without requiring a full CLI build.
// TRACK: BLI-REDACTED — mesh lease supervision integration spawn.
func writeMeshLeaseDaemonStub(t *testing.T, projectRoot string) {
	t.Helper()
	binDir := filepath.Join(projectRoot, binDirName)
	if err := fileutil.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	stub := filepath.Join(binDir, zqkBinaryName)
	script := "#!/bin/sh\n# mesh lease supervision test stub\nsleep 120\n"
	if err := fileutil.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatalf("write daemon stub: %v", err)
	}
}

// cleanupMeshLeaseSubprocesses kills any stub daemons so TempDir RemoveAll can succeed.
func cleanupMeshLeaseSubprocesses(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		leaseSupervisionMutex.Lock()
		defer leaseSupervisionMutex.Unlock()
		for id, cmd := range activeLeaseSubprocesses {
			if cmd != nil && cmd.Process != nil {
				_ = cmd.Process.Kill()
				_, _ = cmd.Process.Wait()
			}
			delete(activeLeaseSubprocesses, id)
		}
	})
}

func ensureMeshSpecsGenerated(t *testing.T, root string) {
	t.Helper()
	specsDir := filepath.Join(root, paths.ProcessInternalObjectSpecsDir)
	if err := fileutil.MkdirAll(specsDir, 0755); err != nil {
		t.Fatalf("ensureMeshSpecsGenerated mkdir: %v", err)
	}
	if err := builders.NewSpecGenerator(specsDir).GenerateAllSpecs(); err != nil {
		t.Fatalf("ensureMeshSpecsGenerated GenerateAllSpecs: %v", err)
	}
	objects.GetGlobalSpecLoader().ClearCache()
	objects.GetGlobalFieldRegistry().TryReloadFromFindSpecsDir()
	objects.ResetGlobalKindMapperForTesting()
}

// TestMeshLeaseSupervision_Integration verifies the end-to-end lease workflow:
// creating a skill_lease triggers the supervisor to start a local scheduler
// pinned strictly to the consumer's isolated project root.
func TestMeshLeaseSupervision_Integration(t *testing.T) {
	_ = zqkenv.StreamStorageEnabled().Set("false")
	defer zqkenv.StreamStorageEnabled().Unset()
	cleanupMeshLeaseSubprocesses(t)

	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// 1. Setup Provider Environment
	providerEnv := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "provider",
		AppendStagesBeforeStorage: func(root string) []testkit.NamedTestStep {
			return []testkit.NamedTestStep{
				{
					Name: "CopySpecs",
					Fn: func() error {
						_ = testenvroot.CopyObjectSpecsFromProject(root, "../..")
						_ = testenvroot.CopyLifecyclesFromProject(root, "../..")
						ensureMeshSpecsGenerated(t, root)
						return nil
					},
				},
			}
		},
	})
	providerStorage := providerEnv.FileStorage
	providerRoot := providerEnv.Root
	writeMeshLeaseDaemonStub(t, providerRoot)

	// 2. Setup Consumer Environment
	consumerEnv := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "consumer",
		AppendStagesBeforeStorage: func(root string) []testkit.NamedTestStep {
			return []testkit.NamedTestStep{
				{
					Name: "CopySpecs",
					Fn: func() error {
						_ = testenvroot.CopyObjectSpecsFromProject(root, "../..")
						_ = testenvroot.CopyLifecyclesFromProject(root, "../..")
						ensureMeshSpecsGenerated(t, root)
						return nil
					},
				},
			}
		},
	})
	consumerRoot := consumerEnv.Root

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	// 3. Resolve Provider Identity
	idManager := federation.NewIdentityManager(providerRoot)
	providerKernelID, err := idManager.GetKernelID()
	if err != nil {
		t.Fatalf("Failed to get provider kernel ID: %v", err)
	}

	// 4. Mock Consumer Remote Kernel
	consumerKernelID := "REM-CONSUMER-1"
	consumerEndpoint := fmt.Sprintf("file://%s", consumerRoot)

	consumerRemoteKernel := map[string]any{
		objects.FieldKeyID:       consumerKernelID,
		objects.FieldKeyKind:     objects.KindRemoteKernel,
		objects.FieldKeyTitle:    "Test Consumer Kernel",
		objects.FieldKeyEndpoint: consumerEndpoint,
		objects.FieldKeyStatus:   objects.ObjectStatusApproved,
	}
	storage.CreateCASVisible(t, providerStorage, ctx, secCtx, consumerRemoteKernel, objects.ObjectStatusApproved)

	// 5. Create Skill Lease (Provider granting capacity to Consumer)
	origin, _ := objects.GetGlobalLifecycleLoader().GetOriginStatus(objects.KindZqkSession)
	isPrelim := objects.GetGlobalStatusChecker().IsPreliminary(objects.KindZqkSession, origin)
	t.Logf("DEBUG LIFECYCLE: origin=%v isPrelim=%v", origin, isPrelim)

	leaseID := "ZQK-1000"
	skillLease := map[string]any{
		objects.FieldKeyID:                leaseID,
		objects.FieldKeyKind:              objects.KindZqkSession,
		objects.FieldKeyAccountID:         "ACC-1785920548450214012-68b850c0",
		objects.FieldKeyTitle:             "Test Compute Lease",
		objects.FieldKeyStatus:            objects.ObjectStatusActive,
		objects.FieldKeyProviderKernelRef: providerKernelID,
		objects.FieldKeyConsumerKernelRef: consumerKernelID,
		objects.FieldKeyResourceRef:       "COMPUTE-POOL-A",
		objects.FieldKeyTokenID:           "TOK-12345",
		objects.FieldKeySessionMode:       "federated_lease",
		objects.FieldKeyAgreementMode:     "grantor_enforced",
	}
	storage.CreateCASVisible(t, providerStorage, ctx, secCtx, skillLease, objects.ObjectStatusActive)

	// Force the stream segment directly!
	time.Sleep(200 * time.Millisecond)
		streamDir := filepath.Join(providerRoot, ".zqk", "streams", "zqk_session")
	if files, err := fileutil.ReadDir(streamDir); err == nil {
		for _, file := range files {
			if strings.HasSuffix(file.Name(), ".json") {
				segmentPath := filepath.Join(streamDir, file.Name())
				if data, err := fileutil.ReadFile(segmentPath); err == nil {
					t.Logf("Before stream patch: %s", string(data))
					modified := strings.ReplaceAll(string(data), "\"status\":\"conceptual\"", "\"status\":\"active\"")
					_ = fileutil.WriteFile(segmentPath, []byte(modified), 0644)
					t.Logf("After stream patch: %s", modified)
				}
			}
		}
	} else {
		t.Logf("ReadDir err: %v", err)
	}


	
	// Stream-backed objects skip promotion in CreateCASVisible, so force the update here:
	breakCtx1 := pkgctx.WithLifecycleBreakGlass(ctx, "test harness setup")
	if err := providerStorage.Update(breakCtx1, secCtx, leaseID, map[string]any{objects.FieldKeyStatus: objects.ObjectStatusActive}); err != nil {
		t.Fatalf("Failed to force active status: %v", err)
	}

	// FORCE UPDATE THE CACHE!!!
	if cache := storage.GetGlobalHighVolumeEventCache(); cache != nil {
		if entry, ok := cache.Get(leaseID); ok {
			entry.Status = objects.ObjectStatusActive
			cache.Set(entry)
			t.Logf("Cache forcibly updated for %s", leaseID)
		} else {
			t.Logf("Cache MISS for %s", leaseID)
		}
	}

	// 6. Execute Supervisor Handler
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	handler := NewMeshLeaseSupervisionHandler(providerStorage, providerRoot, logger)

	job := &ScheduledJob{
		ID:       "SCH-LEASE-SUPERVISION-TEST",
		JobType:  "mesh_lease_supervision",
		Category: "system",
	}

	err = handler.Execute(ctx, job)
	if err != nil {
		t.Fatalf("Handler execution failed: %v", err)
	}

	// 7. Verify Subprocess Started
	leaseSupervisionMutex.Lock()
	cmd, exists := activeLeaseSubprocesses[leaseID]
	leaseSupervisionMutex.Unlock()

	if !exists {
		t.Fatalf("Expected subprocess to be started for lease %s", leaseID)
	}
	if cmd.Process == nil {
		t.Fatalf("Subprocess command created but process not started")
	}

	// Verify isolation: The subprocess MUST be executed in the consumer's directory
	if cmd.Dir != consumerRoot {
		t.Errorf("Subprocess working directory is %q, want %q", cmd.Dir, consumerRoot)
	}

	foundRootEnv := false
	for _, envStr := range cmd.Env {
		if strings.HasPrefix(envStr, "ZQK_PROJECT_ROOT=") {
			foundRootEnv = true
			val := strings.TrimPrefix(envStr, "ZQK_PROJECT_ROOT=")
			if val != consumerRoot {
				t.Errorf("ZQK_PROJECT_ROOT env is %q, want %q", val, consumerRoot)
			}
		}
	}
	if !foundRootEnv {
		t.Error("ZQK_PROJECT_ROOT environment variable not injected into subprocess")
	}

	// Wait a moment, then reap
	time.Sleep(200 * time.Millisecond)

	// Clean up by marking lease inactive and executing again
	leaseInactive := map[string]any{
		objects.FieldKeyID:     leaseID,
		objects.FieldKeyStatus: objects.ObjectStatusCompleted,
	}
	// Find the yaml file
	//nolint:gosec
	cmdExec := __exec.Command("find", providerRoot, "-name", "*ZQK-1000*")
	outExec, _ := cmdExec.CombinedOutput()
	t.Logf("Found files: %s", string(outExec))

	breakCtx := pkgctx.WithLifecycleBreakGlass(ctx, "test harness")
	if err := providerStorage.Update(breakCtx, secCtx, leaseID, leaseInactive); err != nil {
		t.Fatalf("Failed to update skill lease status: %v", err)
	}

	err = handler.Execute(ctx, job)
	if err != nil {
		t.Fatalf("Handler execution during reap failed: %v", err)
	}

	leaseSupervisionMutex.Lock()
	_, stillExists := activeLeaseSubprocesses[leaseID]
	leaseSupervisionMutex.Unlock()

	if stillExists {
		t.Errorf("Subprocess for %s should have been reaped after lease expiration", leaseID)
	}
}

func TestMeshLeaseSupervision_QuotasAndRevocation(t *testing.T) {
	_ = zqkenv.StreamStorageEnabled().Set("false")
	defer zqkenv.StreamStorageEnabled().Unset()
	cleanupMeshLeaseSubprocesses(t)

	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	providerEnv := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "provider",
	})
	_ = testenvroot.CopyObjectSpecsFromProject(providerEnv.Root, "../..")
	providerStorage := providerEnv.FileStorage
	providerRoot := providerEnv.Root
	writeMeshLeaseDaemonStub(t, providerRoot)

	consumerEnv := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "consumer",
	})
	_ = testenvroot.CopyObjectSpecsFromProject(consumerEnv.Root, "../..")
	consumerRoot := consumerEnv.Root

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	idManager := federation.NewIdentityManager(providerRoot)
	providerKernelID, _ := idManager.GetKernelID()

	consumerKernelID := "REM-CONSUMER-2"
	consumerRemoteKernel := map[string]any{
		objects.FieldKeyID:       consumerKernelID,
		objects.FieldKeyKind:     objects.KindRemoteKernel,
		objects.FieldKeyTitle:    "Test Consumer Kernel 2",
		objects.FieldKeyEndpoint: fmt.Sprintf("file://%s", consumerRoot),
		objects.FieldKeyStatus:   objects.ObjectStatusApproved,
	}
	storage.CreateCASVisible(t, providerStorage, ctx, secCtx, consumerRemoteKernel, objects.ObjectStatusApproved)

	leaseID := "ZQK-1001"
	skillLease := map[string]any{
		objects.FieldKeyID:                leaseID,
		objects.FieldKeyKind:              objects.KindZqkSession,
		objects.FieldKeyAccountID:         "ACC-1785920548450214012-68b850c0",
		objects.FieldKeyTitle:             "Test Quota Lease",
		objects.FieldKeyStatus:            objects.ObjectStatusActive,
		objects.FieldKeyProviderKernelRef: providerKernelID,
		objects.FieldKeyConsumerKernelRef: consumerKernelID,
		objects.FieldKeyResourceRef:       "COMPUTE-POOL-B",
		objects.FieldKeyTokenID:           "TOK-QUOTA",
		objects.FieldKeySessionMode:       "federated_lease",
		objects.FieldKeyAgreementMode:     "grantor_enforced",
		objects.FieldKeyTermType:          "invocations",
		objects.FieldKeyMaxUnits:          float64(5),
		objects.FieldKeyConsumedUnits:     float64(0),
	}
	storage.CreateCASVisible(t, providerStorage, ctx, secCtx, skillLease, objects.ObjectStatusActive)

	// Force the stream segment directly!
	time.Sleep(200 * time.Millisecond)
		streamDir := filepath.Join(providerRoot, ".zqk", "streams", "zqk_session")
	if files, err := fileutil.ReadDir(streamDir); err == nil {
		for _, file := range files {
			if strings.HasSuffix(file.Name(), ".json") {
				segmentPath := filepath.Join(streamDir, file.Name())
				if data, err := fileutil.ReadFile(segmentPath); err == nil {
					t.Logf("Before stream patch: %s", string(data))
					modified := strings.ReplaceAll(string(data), "\"status\":\"conceptual\"", "\"status\":\"active\"")
					_ = fileutil.WriteFile(segmentPath, []byte(modified), 0644)
					t.Logf("After stream patch: %s", modified)
				}
			}
		}
	} else {
		t.Logf("ReadDir err: %v", err)
	}


	
	// Stream-backed objects skip promotion in CreateCASVisible, so force the update here:
	breakCtx1 := pkgctx.WithLifecycleBreakGlass(ctx, "test harness setup")
	if err := providerStorage.Update(breakCtx1, secCtx, leaseID, map[string]any{objects.FieldKeyStatus: objects.ObjectStatusActive}); err != nil {
		t.Fatalf("Failed to force active status: %v", err)
	}

	// FORCE UPDATE THE CACHE!!!
	if cache := storage.GetGlobalHighVolumeEventCache(); cache != nil {
		if entry, ok := cache.Get(leaseID); ok {
			entry.Status = objects.ObjectStatusActive
			cache.Set(entry)
			t.Logf("Cache forcibly updated for %s", leaseID)
		} else {
			t.Logf("Cache MISS for %s", leaseID)
		}
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	handler := NewMeshLeaseSupervisionHandler(providerStorage, providerRoot, logger)
	job := &ScheduledJob{
		ID:       "SCH-LEASE-SUPERVISION-TEST-2",
		JobType:  "mesh_lease_supervision",
		Category: "system",
	}

	// 1. Initially, quota is fine, subprocess should start
	_ = handler.Execute(ctx, job)

	leaseSupervisionMutex.Lock()
	_, exists := activeLeaseSubprocesses[leaseID]
	leaseSupervisionMutex.Unlock()
	if !exists {
		t.Fatalf("Expected subprocess to start")
	}

	// 2. Exhaust quota
	skillLease[objects.FieldKeyStatus] = objects.ObjectStatusActive
	skillLease[objects.FieldKeyConsumedUnits] = float64(5)
	if err := providerStorage.Update(ctx, secCtx, leaseID, skillLease); err != nil {
		t.Fatalf("Failed to update skill lease: %v", err)
	}
	if err := storage.EnsureCLIObjectMutationVisibleForProvider(ctx, providerStorage, providerRoot, []string{objects.KindZqkSession}); err != nil {
		t.Fatalf("Failed to flush: %v", err)
	}

	if err := handler.Execute(ctx, job); err != nil {
		t.Fatalf("Handler execution failed: %v", err)
	}
	if err := storage.EnsureCLIObjectMutationVisibleForProvider(ctx, providerStorage, providerRoot, []string{objects.KindZqkSession}); err != nil {
		t.Fatalf("Failed to flush: %v", err)
	}

	leaseSupervisionMutex.Lock()
	_, stillExists := activeLeaseSubprocesses[leaseID]
	leaseSupervisionMutex.Unlock()
	if stillExists {
		t.Fatalf("Expected subprocess to be reaped due to quota exhaustion")
	}

	updatedLease, readErr := providerStorage.Read(ctx, secCtx, leaseID)
	if readErr != nil {
		t.Fatalf("Failed to read lease: %v", readErr)
	}
	t.Logf("DEBUG: updatedLease: %+v", updatedLease)
	if updatedLease[objects.FieldKeyStatus] != objects.ObjectStatusComplete && updatedLease[objects.FieldKeyStatus] != objects.ObjectStatusCompleted {
		t.Errorf("Expected lease status complete/completed, got %v", updatedLease[objects.FieldKeyStatus])
	}

	// 3. Test explicit revocation
	leaseID2 := "ZQK-1002"
	skillLease2 := map[string]any{
		objects.FieldKeyID:                leaseID2,
		objects.FieldKeyKind:              objects.KindZqkSession,
		objects.FieldKeyAccountID:         "ACC-1785920548450214012-68b850c0",
		objects.FieldKeyTitle:             "Test Revocation Lease",
		objects.FieldKeyStatus:            objects.ObjectStatusActive,
		objects.FieldKeyProviderKernelRef: providerKernelID,
		objects.FieldKeyConsumerKernelRef: consumerKernelID,
		objects.FieldKeyResourceRef:       "COMPUTE-POOL-C",
		objects.FieldKeyTokenID:           "TOK-REVOKE",
		objects.FieldKeySessionMode:       "federated_lease",
		objects.FieldKeyAgreementMode:     "grantor_enforced",
		objects.FieldKeyTermType:          "duration_minutes",
	}
	storage.CreateCASVisible(t, providerStorage, ctx, secCtx, skillLease2, objects.ObjectStatusActive)

	time.Sleep(200 * time.Millisecond)
		streamDir2 := filepath.Join(providerRoot, ".zqk", "streams", "zqk_session")
	if files, err := fileutil.ReadDir(streamDir2); err == nil {
		for _, file := range files {
			if strings.HasSuffix(file.Name(), ".json") {
				segmentPath := filepath.Join(streamDir2, file.Name())
				if data, err := fileutil.ReadFile(segmentPath); err == nil {
					modified := strings.ReplaceAll(string(data), "\"status\":\"conceptual\"", "\"status\":\"active\"")
					_ = fileutil.WriteFile(segmentPath, []byte(modified), 0644)
				}
			}
		}
	}

	breakCtx2 := pkgctx.WithLifecycleBreakGlass(ctx, "test harness setup")
	if err := providerStorage.Update(breakCtx2, secCtx, leaseID, map[string]any{objects.FieldKeyStatus: objects.ObjectStatusActive}); err != nil {
		t.Fatalf("Failed to force active status: %v", err)
	}
	if err := storage.EnsureCLIObjectMutationVisibleForProvider(ctx, providerStorage, providerRoot, []string{objects.KindZqkSession}); err != nil {
		t.Fatalf("Failed to flush: %v", err)
	}

	_ = handler.Execute(ctx, job)
	if err := storage.EnsureCLIObjectMutationVisibleForProvider(ctx, providerStorage, providerRoot, []string{objects.KindZqkSession}); err != nil {
		t.Fatalf("Failed to flush: %v", err)
	}

	leaseSupervisionMutex.Lock()
	_, exists2 := activeLeaseSubprocesses[leaseID2]
	leaseSupervisionMutex.Unlock()
	if !exists2 {
		t.Fatalf("Expected subprocess for leaseID2 to start")
	}

	// skillLease2[objects.FieldKeyRevokedAt] = "2026-05-27T00:00:00Z"
	// t.Logf("DEBUG: skillLease2 before update: %v", skillLease2)
	// Just update the revoked_at field, don't pass the whole map which has stale status
	if err := providerStorage.Update(ctx, secCtx, leaseID2, map[string]any{objects.FieldKeyRevokedAt: "2026-05-27T00:00:00Z"}); err != nil {
		t.Fatalf("Failed to update skill lease 2: %v", err)
	}
	if err := storage.EnsureCLIObjectMutationVisibleForProvider(ctx, providerStorage, providerRoot, []string{objects.KindZqkSession}); err != nil {
		t.Fatalf("Failed to flush: %v", err)
	}

	if err := handler.Execute(ctx, job); err != nil {
		t.Fatalf("Handler execution failed: %v", err)
	}
	if err := storage.EnsureCLIObjectMutationVisibleForProvider(ctx, providerStorage, providerRoot, []string{objects.KindZqkSession}); err != nil {
		t.Fatalf("Failed to flush: %v", err)
	}

	leaseSupervisionMutex.Lock()
	var keys []string
	for k := range activeLeaseSubprocesses {
		keys = append(keys, k)
	}
	t.Logf("DEBUG: activeLeaseSubprocesses keys: %+v", keys)
	_, stillExists2 := activeLeaseSubprocesses[leaseID2]
	leaseSupervisionMutex.Unlock()
	if stillExists2 {
		t.Fatalf("Expected subprocess to be reaped due to revocation, active keys: %+v", keys)
	}

	updatedLease2, readErr2 := providerStorage.Read(ctx, secCtx, leaseID2)
	if readErr2 != nil {
		t.Fatalf("Failed to read lease 2: %v", readErr2)
	}
	t.Logf("DEBUG: updatedLease2: %+v", updatedLease2)
	if updatedLease2[objects.FieldKeyStatus] != objects.ObjectStatusArchived {
		t.Errorf("Expected lease status 'archived', got %v", updatedLease2[objects.FieldKeyStatus])
	}
}
