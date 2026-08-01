package scheduler

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/federation"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testenvroot"
	"github.com/lanceman/zqk/pkg/testkit"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// TestMeshLeaseSupervision_Integration verifies the end-to-end lease workflow:
// creating a skill_lease triggers the supervisor to start a local scheduler
// pinned strictly to the consumer's isolated project root.
func TestMeshLeaseSupervision_Integration(t *testing.T) {
	os.Setenv(zqkenv.StreamStorageEnabled(), "false")
	defer os.Unsetenv(zqkenv.StreamStorageEnabled())

	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// 1. Setup Provider Environment
	providerEnv := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "provider",
	})
	_ = testenvroot.CopyObjectSpecsFromProject(providerEnv.Root, "../..")
	providerStorage := providerEnv.FileStorage
	providerRoot := providerEnv.Root

	// 2. Setup Consumer Environment
	consumerEnv := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "consumer",
	})
	_ = testenvroot.CopyObjectSpecsFromProject(consumerEnv.Root, "../..")
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
		objects.FieldKeyStatus:   "implemented",
	}

	if err := providerStorage.Create(ctx, secCtx, consumerRemoteKernel); err != nil {
		t.Fatalf("Failed to create consumer remote kernel: %v", err)
	}

	// 5. Create Skill Lease (Provider granting capacity to Consumer)
	leaseID := "ZQK-1000"
	skillLease := map[string]any{
		objects.FieldKeyID:                leaseID,
		objects.FieldKeyKind:              objects.KindZqkSession,
		objects.FieldKeyAccountID:         "account:system",
		objects.FieldKeyTitle:             "Test Compute Lease",
		objects.FieldKeyStatus:            "implemented",
		objects.FieldKeyProviderKernelRef: providerKernelID,
		objects.FieldKeyConsumerKernelRef: consumerKernelID,
		objects.FieldKeyResourceRef:       "COMPUTE-POOL-A",
		objects.FieldKeyTokenID:           "TOK-12345",
		objects.FieldKeySessionMode:       "federated_lease",
		objects.FieldKeyAgreementMode:     "grantor_enforced",
	}

	if err := providerStorage.Create(ctx, secCtx, skillLease); err != nil {
		t.Fatalf("Failed to create lease: %v", err)
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
		objects.FieldKeyStatus: "expired",
	}
	if err := providerStorage.Update(ctx, secCtx, leaseID, leaseInactive); err != nil {
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
	os.Setenv(zqkenv.StreamStorageEnabled(), "false")
	defer os.Unsetenv(zqkenv.StreamStorageEnabled())

	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	providerEnv := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "provider",
	})
	_ = testenvroot.CopyObjectSpecsFromProject(providerEnv.Root, "../..")
	providerStorage := providerEnv.FileStorage
	providerRoot := providerEnv.Root

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
		objects.FieldKeyStatus:   "implemented",
	}
	_ = providerStorage.Create(ctx, secCtx, consumerRemoteKernel)

	leaseID := "ZQK-1001"
	skillLease := map[string]any{
		objects.FieldKeyID:                leaseID,
		objects.FieldKeyKind:              objects.KindZqkSession,
		objects.FieldKeyAccountID:         "account:system",
		objects.FieldKeyTitle:             "Test Quota Lease",
		objects.FieldKeyStatus:            "implemented",
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
	_ = providerStorage.Create(ctx, secCtx, skillLease)

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
	if updatedLease[objects.FieldKeyStatus] != "expired" {
		t.Errorf("Expected lease status 'expired', got %v", updatedLease[objects.FieldKeyStatus])
	}

	// 3. Test explicit revocation
	leaseID2 := "ZQK-1002"
	skillLease2 := map[string]any{
		objects.FieldKeyID:                leaseID2,
		objects.FieldKeyKind:              objects.KindZqkSession,
		objects.FieldKeyAccountID:         "account:system",
		objects.FieldKeyTitle:             "Test Revocation Lease",
		objects.FieldKeyStatus:            "implemented",
		objects.FieldKeyProviderKernelRef: providerKernelID,
		objects.FieldKeyConsumerKernelRef: consumerKernelID,
		objects.FieldKeyResourceRef:       "COMPUTE-POOL-C",
		objects.FieldKeyTokenID:           "TOK-REVOKE",
		objects.FieldKeySessionMode:       "federated_lease",
		objects.FieldKeyAgreementMode:     "grantor_enforced",
		objects.FieldKeyTermType:          "duration_minutes",
	}
	_ = providerStorage.Create(ctx, secCtx, skillLease2)
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

	skillLease2[objects.FieldKeyRevokedAt] = "2026-05-27T00:00:00Z"
	if err := providerStorage.Update(ctx, secCtx, leaseID2, skillLease2); err != nil {
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
	if updatedLease2[objects.FieldKeyStatus] != "revoked" {
		t.Errorf("Expected lease status 'revoked', got %v", updatedLease2[objects.FieldKeyStatus])
	}
}
