package scheduler

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"context"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation/qa"
)

// ExecutionDecision represents the policy's decision for a scheduler job run.
type ExecutionDecision struct {
	Action     string // execute, skip, defer, scheduled-defer
	Reason     string
	DeferUntil *time.Time
	RetryAfter *time.Duration
}

// PolicyRule is a single rule within an execution policy (placeholder for phase-2).
type PolicyRule struct {
	Type      string         `yaml:"type"`
	Condition map[string]any `yaml:"condition,omitempty"`
	Action    string         `yaml:"action,omitempty"`
	Reason    string         `yaml:"reason,omitempty"`
}

// ExecutionPolicy is a policy definition for a jobID (placeholder for phase-2).
type ExecutionPolicy struct {
	SchemaVersion string       `yaml:"schema_version,omitempty"`
	JobID         string       `yaml:"job_id"`
	Rules         []PolicyRule `yaml:"rules,omitempty"`
	DefaultAction string       `yaml:"default_action,omitempty"`
}

// PolicyEngine evaluates execution policies using JobStateRegistry.
//
// Minimal behavior (phase-1): skip if job is already in_progress or deferred.
type PolicyEngine struct {
	stateRegistry JobStateRegistryInterface
	policies      map[string]*ExecutionPolicy

	mu sync.RWMutex

	negotiator NegotiatorInterface
	qaGate     qa.Gate

	storage storagepkg.ObjectStorageProvider

	// policiesDir is the persistent storage root for execution policies:
	//   {projectRoot}/.zqk/scheduler/policies/{jobID}.yaml
	policiesDir string
}

const (
	decisionExecute        = "execute"
	decisionSkip           = "skip"
	decisionDefer          = "defer"
	decisionScheduledDefer = "scheduled-defer"

	executionPolicySchemaVersion = objects.DefaultSchemaVersion
	maxPolicyFilenameJobIDLen    = 200

	// staleInProgressOtherPIDAfter is how old an in_progress row must be before we treat a
	// different process_id as stale (daemon restart / crash). Keeps unit tests that register
	// a fake PID from flaking while fixing "orphan state blocks timer jobs" after reboot.
	staleInProgressOtherPIDAfter = 5 * time.Second
)

// NewPolicyEngine creates a new policy engine
func NewPolicyEngine(stateRegistry JobStateRegistryInterface, projectRoot string, s storagepkg.ObjectStorageProvider) PolicyEngineInterface {
	var policiesDir string
	if projectRoot != emptyValue {
		policiesDir = filepath.Join(projectRoot, paths.ProjectDataDir, paths.SchedulerDir, "policies")
	}
	pe := &PolicyEngine{
		stateRegistry: stateRegistry,
		policies:      make(map[string]*ExecutionPolicy),
		policiesDir:   policiesDir,
		storage:       s,
	}
	// Initial load
	_ = pe.Reload()
	return pe
}

// Reload reloads all policies from disk
func (pe *PolicyEngine) Reload() error {
	if pe.policiesDir == emptyValue {
		return nil
	}
	pe.mu.Lock()
	defer pe.mu.Unlock()
	pe.policies = make(map[string]*ExecutionPolicy)
	// We don't actually need to load them all into memory now,
	// LoadPolicy will load them on demand.
	return nil
}

// Evaluate returns the policy decision for a given jobID.
func (pe *PolicyEngine) Evaluate(jobID string, job *ScheduledJob) (*ExecutionDecision, error) {
	if pe == nil || pe.stateRegistry == nil {
		return nil, errfmt.Errorf("policy engine requires JobStateRegistry")
	}
	if strings.TrimSpace(jobID) == emptyValue {
		return nil, errfmt.Errorf("jobID required")
	}
	_ = job // phase-1 doesn't use job spec; included to match planned API shape.

	// Mandatory QA Gate for Backlog Items
	if strings.HasPrefix(jobID, "BLI-") {
		if pe.qaGate == nil {
			return &ExecutionDecision{Action: decisionSkip, Reason: "QA Gate Fail: QA Gate is not configured"}, nil
		}
		if err := pe.qaGate.VerifyComplete(context.TODO(), jobID); err != nil {
			return &ExecutionDecision{Action: decisionSkip, Reason: "QA Gate Fail: " + err.Error()}, nil
		}
	}

	st, err := pe.stateRegistry.GetExecutionState(jobID)
	if err != nil {
		return nil, err
	}
	if st != nil {
		switch st.State {
		case jobExecutionStateInProgress:
			if pe.inProgressStateIsStale(st, job) {
				logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
				SchedulerPolicyEngineLog(logger).Warn(LogEventSchedulerPolicyReconcilingStaleInProgress).
					JobID(jobID).
					String("execution_id", st.ExecutionID).
					Int("recorded_process_id", st.ProcessID).
					Int("current_process_id", os.Getpid()).
					String("started_at", st.StartedAt.Format(time.RFC3339)).
					Log()
				_ = pe.stateRegistry.CompleteExecution(jobID, st.ExecutionID, "failed_stale_orphan")
				// Fall through to default execute path below.
			} else {
				return &ExecutionDecision{Action: decisionSkip, Reason: "job already in_progress"}, nil
			}
		case jobExecutionStateDeferred:
			return &ExecutionDecision{Action: decisionSkip, Reason: "job already deferred"}, nil
		}
	}

	// Optional .zqk/scheduler/skip_window.yaml — skip matching jobs until `until` (instability / bulk pause).
	if job != nil && pe.policiesDir != emptyValue {
		schedDir := filepath.Dir(pe.policiesDir)
		if sw, _ := LoadSchedulerSkipWindow(schedDir, time.Now().UTC()); JobMatchesSchedulerSkipWindow(job, sw) {
			return &ExecutionDecision{Action: decisionSkip, Reason: FormatSkipWindowReason(sw)}, nil
		}
	}

	// Phase-2-ish (still phase-1 compatible): if no execution state exists yet,
	// allow persisted policy default_action to influence the decision.
	pol, lerr := pe.LoadPolicy(jobID)
	if lerr == nil && pol != nil {
		switch strings.ToLower(strings.TrimSpace(pol.DefaultAction)) {
		case decisionSkip:
			return &ExecutionDecision{Action: decisionSkip, Reason: "policy default_action=skip"}, nil
		case decisionExecute:
			// Explicitly honor execute.
			return &ExecutionDecision{Action: decisionExecute, Reason: "policy default_action=execute"}, nil
		}
	}

	return &ExecutionDecision{Action: decisionExecute, Reason: "default execution"}, nil
}

// inProgressStateIsStale returns true when persisted in_progress almost certainly does not
// represent a live run (other PID after grace, or same PID far beyond 2×max_runtime slack).
func (pe *PolicyEngine) inProgressStateIsStale(st *JobExecutionState, job *ScheduledJob) bool {
	if st == nil || st.State != jobExecutionStateInProgress {
		return false
	}
	now := time.Now().UTC()
	// Active lease means the run is legitimately occupied until CompleteExecution or max_runtime.
	if st.Lease != nil && st.Lease.IsValid(now) {
		return false
	}
	age := now.Sub(st.StartedAt)
	if st.ProcessID > 0 && st.ProcessID != os.Getpid() && age >= staleInProgressOtherPIDAfter {
		return true
	}
	maxSec := 0
	if job != nil {
		maxSec = job.MaxRuntimeSeconds
	}
	if maxSec <= 0 {
		maxSec = 3600
	}
	// Same process: allow long runs, but clear impossible ghosts (e.g. panic before CompleteExecution
	// before we added recovery — see job_execution coordination defer).
	samePIDLimit := time.Duration(maxSec)*2*time.Second + 15*time.Minute
	if st.ProcessID == os.Getpid() && age > samePIDLimit {
		return true
	}
	return false
}

func policyFilenameForJobID(jobID string) string {
	if len(jobID) <= maxPolicyFilenameJobIDLen {
		return jobID + ".yaml"
	}
	sum := sha256.Sum256([]byte(jobID))
	return hex.EncodeToString(sum[:]) + ".yaml"
}

func (pe *PolicyEngine) policyFilePath(jobID string) string {
	if pe == nil || pe.policiesDir == emptyValue {
		return ""
	}
	return filepath.Join(pe.policiesDir, policyFilenameForJobID(jobID))
}

func (pe *PolicyEngine) LoadPolicy(jobID string) (*ExecutionPolicy, error) {
	if strings.TrimSpace(jobID) == emptyValue {
		return nil, errfmt.Errorf("jobID required")
	}

	if pe.policiesDir == emptyValue {
		return nil, nil // persistence disabled
	}

	pe.mu.RLock()
	if p := pe.policies[jobID]; p != nil {
		pe.mu.RUnlock()
		return p, nil
	}
	pe.mu.RUnlock()

	path := pe.policyFilePath(jobID)
	if path == emptyValue {
		return nil, nil
	}

	b, err := fileutil.ReadFile(path)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return nil, nil
		}
		return nil, errfmt.Errorf("read policy %q: %w", path, err)
	}

	var loaded ExecutionPolicy
	if err := yaml.Unmarshal(b, &loaded); err != nil {
		return nil, errfmt.Errorf("unmarshal policy %q: %w", path, err)
	}
	if strings.TrimSpace(loaded.JobID) == emptyValue {
		loaded.JobID = jobID
	}
	if loaded.SchemaVersion == emptyValue {
		loaded.SchemaVersion = executionPolicySchemaVersion
	}

	pe.mu.Lock()
	pe.policies[jobID] = &loaded
	pe.mu.Unlock()

	return &loaded, nil
}

func (pe *PolicyEngine) SavePolicy(policy *ExecutionPolicy) error {
	if policy == nil {
		return errfmt.Errorf("policy required")
	}
	if strings.TrimSpace(policy.JobID) == emptyValue {
		return errfmt.Errorf("policy.JobID required")
	}

	if pe.policiesDir == emptyValue {
		return errfmt.Errorf("policy persistence disabled (empty projectRoot)")
	}

	policy.SchemaVersion = executionPolicySchemaVersion

	if err := fileutil.EnsureDir(pe.policiesDir); err != nil {
		return errfmt.Errorf("create policies dir %q: %w", pe.policiesDir, err)
	}

	targetPath := pe.policyFilePath(policy.JobID)
	if targetPath == emptyValue {
		return errfmt.Errorf("policy file path unavailable")
	}

	// Marshal to YAML and atomically persist (temp file + rename) so readers never see partial writes.
	b, err := yaml.Marshal(policy)
	if err != nil {
		return errfmt.Errorf("marshal policy %q: %w", policy.JobID, err)
	}

	tmp, err := fileutil.CreateTemp(pe.policiesDir, fmt.Sprintf("%s-*.yaml.tmp", policy.JobID))
	if err != nil {
		return errfmt.Newf("create temp policy file").Wrap(err)
	}
	tmpPath := tmp.Name()

	writeErr := func() error {
		defer tmp.Close()
		if _, err := tmp.Write(b); err != nil {
			return errfmt.Newf("write temp policy file").Wrap(err)
		}
		return nil
	}()
	if writeErr != nil {
		_ = fileutil.Remove(tmpPath)
		return writeErr
	}

	if err := fileutil.Rename(tmpPath, targetPath); err != nil {
		_ = fileutil.Remove(tmpPath)
		return errfmt.Newf("rename temp policy file").Wrap(err)
	}

	pe.mu.Lock()
	pe.policies[policy.JobID] = policy
	pe.mu.Unlock()

	return nil
}
