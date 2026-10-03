package scheduler

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"

	"github.com/zqk-os/zqk/pkg/execwrap"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/federation"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

var (
	leaseSupervisionMutex   sync.Mutex
	activeLeaseSubprocesses = make(map[string]*exec.Cmd)
)

// MeshLeaseSupervisionHandler oversees active skill_leases and ensures compute capacity is
// provisioned via ambient zqk-scheduler subprocesses running on the same filesystem.
type MeshLeaseSupervisionHandler struct {
	storage     storage.ObjectStorageProvider
	projectRoot string
	logger      logging.Logger
}

// NewMeshLeaseSupervisionHandler creates a new mesh lease supervision handler
func NewMeshLeaseSupervisionHandler(sp storage.ObjectStorageProvider, projectRoot string, logger logging.Logger) MeshLeaseSupervisionHandlerInterface {
	return &MeshLeaseSupervisionHandler{
		storage:     sp,
		projectRoot: projectRoot,
		logger:      logger,
	}
}

func (h *MeshLeaseSupervisionHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	kernelID, err := federation.ResolveKernelID(h.projectRoot)
	if err != nil {
		return err
	}

	secCtx := pkgctx.GetSecurityContext(ctx)
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}

	filter := storage.ListFilter{
		Kind: objects.KindZqkSession,
		Filters: map[string]any{
			objects.FieldKeySessionMode:       "federated_lease",
			objects.FieldKeyStatus:            objects.ObjectStatusActive,
			objects.FieldKeyProviderKernelRef: kernelID,
		},
		Limit: 0,
	}

	res, err := h.storage.List(ctx, secCtx, nil, filter)
	if err != nil {
		return err
	}

	activeLeaseIDs := make(map[string]bool)

	for _, lease := range res.Objects {
		leaseID, _ := lease[objects.FieldKeyID].(string)

		// Read the overlay to get up-to-date consumed_units and status
		readLease, err := h.storage.Read(ctx, secCtx, leaseID)
		if err == nil && readLease != nil {
			lease = readLease
		}

		status, _ := lease[objects.FieldKeyStatus].(string)
		if status != objects.ObjectStatusActive {
			continue
		}

		// 1. Check for explicit revocation
		var revokedAt string
		if val, ok := lease[objects.FieldKeyRevokedAt]; ok && val != nil {
			if s, ok := val.(string); ok {
				revokedAt = s
			} else {
				revokedAt = fmt.Sprintf("%v", val)
			}
		}
		if revokedAt != "" {
			if h.logger != nil {
				SLog(h.logger).Info(fmt.Sprintf("Lease %s is revoked, terminating subprocess", leaseID)).Log()
			}
			// zqk_session lifecycle: active → archived (no "revoked" status).
			// TRACK: align mesh lease terminal statuses with lifecycle.
			lease[objects.FieldKeyStatus] = objects.ObjectStatusArchived
			if err := h.storage.Update(ctx, secCtx, leaseID, lease); err != nil {
				return fmt.Errorf("failed to update status to archived for lease %s: %w", leaseID, err)
			}
			continue
		}

		// 2. Check for quota exhaustion
		termType, _ := lease[objects.FieldKeyTermType].(string)
		if termType != "perpetual" {
			maxUnits := getFloat(lease[objects.FieldKeyMaxUnits])
			consumedUnits := getFloat(lease[objects.FieldKeyConsumedUnits])

			if maxUnits > 0 && consumedUnits >= maxUnits {
				if h.logger != nil {
					SLog(h.logger).Info(fmt.Sprintf("Lease %s quota exhausted (consumed %v >= max %v), terminating subprocess", leaseID, consumedUnits, maxUnits)).Log()
				}
				// zqk_session lifecycle terminal for quota end: complete (alias completed).
				lease[objects.FieldKeyStatus] = objects.ObjectStatusCompleted

				if err := h.storage.Update(ctx, secCtx, leaseID, lease); err != nil {
					return fmt.Errorf("failed to update status to complete for lease %s: %w", leaseID, err)
				}
				continue
			}
		}

		consumerRef, _ := lease[objects.FieldKeyConsumerKernelRef].(string)
		activeLeaseIDs[leaseID] = true

		if err := h.ensureSubprocess(ctx, leaseID, consumerRef); err != nil {
			if h.logger != nil {
				SLog(h.logger).Warn(fmt.Sprintf("Failed to ensure lease subprocess for %s", leaseID)).WithError(err).Log()
			}
		}
	}

	h.reapStaleSubprocesses(activeLeaseIDs)
	return nil
}

func (h *MeshLeaseSupervisionHandler) ensureSubprocess(ctx context.Context, leaseID, consumerKernelRef string) error {
	leaseSupervisionMutex.Lock()

	if cmd, exists := activeLeaseSubprocesses[leaseID]; exists {
		// Verify it's still running
		if cmd.Process != nil {
			err := cmd.Process.Signal(syscall.Signal(0))
			if err == nil {
				leaseSupervisionMutex.Unlock()
				return nil // still running
			}
		}
		// Dead, remove it
		delete(activeLeaseSubprocesses, leaseID)
	}
	leaseSupervisionMutex.Unlock()

	// Resolve the consumer project root. In a local mesh, the endpoint of the consumer kernel is a file:// URI.
	secCtx := pkgctx.GetSecurityContext(ctx)
	consumerKernel, err := h.storage.Read(ctx, secCtx, consumerKernelRef)
	if err != nil {
		return fmt.Errorf("failed to read consumer kernel: %w", err)
	}

	endpoint, _ := consumerKernel[objects.FieldKeyEndpoint].(string)
	if !strings.HasPrefix(endpoint, "file://") {
		return fmt.Errorf("cross-root subprocess federation requires file:// endpoint, got %s", endpoint)
	}

	consumerProjectRoot := strings.TrimPrefix(endpoint, "file://")
	if _, err := fileutil.Stat(consumerProjectRoot); fileutil.IsNotExist(err) {
		return fmt.Errorf("consumer project root does not exist: %s", consumerProjectRoot)
	}

	// Spawn the zqk-scheduler binary with the isolated ZQK_PROJECT_ROOT
	binaryPath, err := ResolveSchedulerDaemonBinary(h.projectRoot)
	if err != nil {
		return fmt.Errorf("failed to resolve scheduler binary: %w", err)
	}

	cmd := execwrap.Command(binaryPath, "system", "scheduler", "run")
	cmd.Dir = consumerProjectRoot
	cmd.Env = os.Environ()

	// Override specific environment variables to guarantee isolation
	envOverrides := map[string]string{
		zqkenv.ProjectRoot().Name(): consumerProjectRoot,
		// Ensure we don't leak provider-specific configurations
		zqkenv.Profile().Name(): "system",
	}

	var newEnv []string
	for _, envVar := range cmd.Env {
		key := strings.SplitN(envVar, "=", 2)[0]
		if _, override := envOverrides[key]; !override {
			newEnv = append(newEnv, envVar)
		}
	}
	for k, v := range envOverrides {
		newEnv = append(newEnv, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = newEnv

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start leased scheduler process: %w", err)
	}

	leaseSupervisionMutex.Lock()
	activeLeaseSubprocesses[leaseID] = cmd
	leaseSupervisionMutex.Unlock()

	if h.logger != nil {
		SLog(h.logger).Info(fmt.Sprintf("Started cross-root leased compute subprocess for %s (PID %d)", leaseID, cmd.Process.Pid)).Log()
	}

	// Wait for process to exit asynchronously to avoid zombies
	goroutinelabels.NewGoroutine("refactored_worker", "Refactored raw goroutine").
		StartSimple(func() {
			func(c *exec.Cmd, lID string) {
				if err := c.Wait(); err != nil {
					if h.logger != nil {
						SLog(h.logger).Debug(fmt.Sprintf("Subprocess %s exited with error: %v", lID, err)).Log()
					}
				}
				leaseSupervisionMutex.Lock()
				if activeLeaseSubprocesses[lID] == c {
					delete(activeLeaseSubprocesses, lID)
				}
				leaseSupervisionMutex.Unlock()
			}(cmd, leaseID)
		})

	return nil
}

func (h *MeshLeaseSupervisionHandler) reapStaleSubprocesses(activeLeaseIDs map[string]bool) {
	leaseSupervisionMutex.Lock()
	defer leaseSupervisionMutex.Unlock()

	for leaseID, cmd := range activeLeaseSubprocesses {
		if !activeLeaseIDs[leaseID] {
			if cmd.Process != nil {
				if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
					if h.logger != nil {
						SLog(h.logger).Warn(fmt.Sprintf("Failed to signal stale leased compute subprocess for %s (PID %d)", leaseID, cmd.Process.Pid)).WithError(err).Log()
					}
				} else if h.logger != nil {
					SLog(h.logger).Info(fmt.Sprintf("Terminated stale leased compute subprocess for %s (PID %d)", leaseID, cmd.Process.Pid)).Log()
				}
			}
			delete(activeLeaseSubprocesses, leaseID)
		}
	}
}

func getFloat(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	}
	return 0
}
