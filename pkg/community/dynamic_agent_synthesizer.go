package community

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/strutil"
)

var (
	// ErrInvalidSynthesizerConfig is returned when synthesizer configuration violates invariants.
	ErrInvalidSynthesizerConfig = errors.New("synthesizer: invalid configuration")
	// ErrInvalidManifest is returned when an agent manifest fails structural validation.
	ErrInvalidManifest = errors.New("synthesizer: invalid agent manifest")
	// ErrCapabilityDenied is returned when a worker requests or declares a disallowed capability.
	ErrCapabilityDenied = errors.New("synthesizer: capability denied")
	// ErrSandboxViolation is returned when a manifest violates sandbox isolation policies.
	ErrSandboxViolation = errors.New("synthesizer: sandbox policy violation")
	// ErrTokenBudgetExceeded is returned when prompt compilation or execution exceeds token budget.
	ErrTokenBudgetExceeded = errors.New("synthesizer: token budget exceeded")
	// ErrWorkerTornDown is returned when executing against a torn-down transient worker.
	ErrWorkerTornDown = errors.New("synthesizer: worker is torn down")
	// ErrSynthesizerWorkerNotFound is returned when a transient worker ID does not exist.
	ErrSynthesizerWorkerNotFound = errors.New("synthesizer: worker not found")
	// ErrWorkerCapacityReached is returned when concurrent active workers reach configured capacity.
	ErrWorkerCapacityReached = errors.New("synthesizer: concurrent worker capacity reached")
)

// WorkerLifecycleState represents the execution lifecycle state of a transient worker.
type WorkerLifecycleState string

const (
	// WorkerStatePending indicates the worker is materialized and awaiting execution.
	WorkerStatePending WorkerLifecycleState = "pending"
	// WorkerStateRunning indicates the worker has executed at least one capability invocation.
	WorkerStateRunning WorkerLifecycleState = "running"
	// WorkerStateTornDown indicates the worker has been deterministically cleaned up.
	WorkerStateTornDown WorkerLifecycleState = "torn_down"
)

// SandboxPolicy defines mandatory isolation boundaries for a transient worker.
type SandboxPolicy struct {
	NetworkIsolation bool     `json:"network_isolation"`
	ReadOnlyRootFS   bool     `json:"read_only_root_fs"`
	MaxMemoryMB      int      `json:"max_memory_mb"`
	AllowedPaths     []string `json:"allowed_paths,omitempty"`
}

// Validate enforces fail-closed sandbox isolation invariants.
func (p SandboxPolicy) Validate() error {
	if !p.NetworkIsolation {
		return fmt.Errorf("%w: network_isolation must be enabled", ErrSandboxViolation)
	}
	if !p.ReadOnlyRootFS {
		return fmt.Errorf("%w: read_only_root_fs must be enabled", ErrSandboxViolation)
	}
	if p.MaxMemoryMB <= 0 {
		return fmt.Errorf("%w: max_memory_mb must be positive", ErrSandboxViolation)
	}
	for i, path := range p.AllowedPaths {
		if err := strutil.ErrorOnBlank(path, "%w: allowed_paths[%d] cannot be blank", ErrSandboxViolation, i); err != nil {
			return err
		}
	}
	return nil
}

// AgentManifest specifies the role, token budget, capabilities, and sandbox for a transient worker.
type AgentManifest struct {
	ManifestID          string        `json:"manifest_id"`
	Role                string        `json:"role"`
	MaxTokenBudget      int           `json:"max_token_budget"`
	AllowedCapabilities []string      `json:"allowed_capabilities"`
	Sandbox             SandboxPolicy `json:"sandbox"`
}

// Validate checks manifest structural invariants, capability bounds, and sandbox policy.
func (m AgentManifest) Validate() error {
	if err := strutil.ErrorOnBlank(m.ManifestID, "%w: manifest_id is required", ErrInvalidManifest); err != nil {
		return err
	}
	if err := strutil.ErrorOnBlank(m.Role, "%w: role is required", ErrInvalidManifest); err != nil {
		return err
	}
	if m.MaxTokenBudget <= 0 {
		return fmt.Errorf("%w: max_token_budget must be positive", ErrInvalidManifest)
	}
	if len(m.AllowedCapabilities) == 0 {
		return fmt.Errorf("%w: allowed_capabilities must declare at least one capability", ErrInvalidManifest)
	}
	for i, capName := range m.AllowedCapabilities {
		if err := strutil.ErrorOnBlank(capName, "%w: capability at index %d cannot be blank", ErrInvalidManifest, i); err != nil {
			return err
		}
	}
	return m.Sandbox.Validate()
}

// SynthesizerConfig configures the dynamic agent synthesizer bounds.
type SynthesizerConfig struct {
	SynthesizerID             string   `json:"synthesizer_id"`
	DefaultTokenBudget        int      `json:"default_token_budget"`
	MaxConcurrentWorkers      int      `json:"max_concurrent_workers"`
	MaxMemoryMB               int      `json:"max_memory_mb"`
	GlobalAllowedCapabilities []string `json:"global_allowed_capabilities,omitempty"`
}

// Validate ensures synthesizer configuration invariants hold.
func (c SynthesizerConfig) Validate() error {
	if err := strutil.ErrorOnBlank(c.SynthesizerID, "%w: synthesizer_id is required", ErrInvalidSynthesizerConfig); err != nil {
		return err
	}
	if c.DefaultTokenBudget <= 0 {
		return fmt.Errorf("%w: default_token_budget must be positive", ErrInvalidSynthesizerConfig)
	}
	if c.MaxConcurrentWorkers <= 0 {
		return fmt.Errorf("%w: max_concurrent_workers must be positive", ErrInvalidSynthesizerConfig)
	}
	for i, capName := range c.GlobalAllowedCapabilities {
		if err := strutil.ErrorOnBlank(capName, "%w: global capability at index %d cannot be blank", ErrInvalidSynthesizerConfig, i); err != nil {
			return err
		}
	}
	return nil
}

// TransientWorker represents an ephemeral materialized worker runtime.
type TransientWorker struct {
	WorkerID       string               `json:"worker_id"`
	Manifest       AgentManifest        `json:"manifest"`
	SystemPrompt   string               `json:"system_prompt"`
	RuntimeDigest  string               `json:"runtime_digest"`
	State          WorkerLifecycleState `json:"state"`
	TokensConsumed int                  `json:"tokens_consumed"`
	CreatedAt      time.Time            `json:"created_at"`
	TornDownAt     time.Time            `json:"torn_down_at,omitempty"`
	capSet         map[string]struct{}
}

// HasCapability reports whether the worker is permitted to invoke the given capability.
func (w *TransientWorker) HasCapability(capability string) bool {
	if w == nil || w.capSet == nil {
		return false
	}
	_, ok := w.capSet[strings.TrimSpace(capability)]
	return ok
}

// DynamicAgentSynthesizer materializes, guards, and tears down ephemeral worker runtimes.
type DynamicAgentSynthesizer struct {
	mu         sync.RWMutex
	cfg        SynthesizerConfig
	globalCaps map[string]struct{}
	workers    map[string]*TransientWorker
	workerSeq  atomic.Uint64
}

// NewDynamicAgentSynthesizer constructs a validated DynamicAgentSynthesizer.
func NewDynamicAgentSynthesizer(cfg SynthesizerConfig) (*DynamicAgentSynthesizer, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &DynamicAgentSynthesizer{
		cfg:        cfg,
		globalCaps: buildCapabilitySet(cfg.GlobalAllowedCapabilities),
		workers:    make(map[string]*TransientWorker),
	}, nil
}

// MaterializeWorker validates the manifest, compiles a token-budgeted prompt, and registers a pending worker.
func (s *DynamicAgentSynthesizer) MaterializeWorker(manifest AgentManifest, prompt string) (*TransientWorker, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	if err := strutil.ErrorOnBlank(prompt, "%w: prompt cannot be blank", ErrInvalidManifest); err != nil {
		return nil, err
	}
	if err := s.enforceGlobalBounds(manifest); err != nil {
		return nil, err
	}

	compiledPrompt := compileSystemPrompt(manifest, prompt)
	promptTokens := estimateTokenCount(compiledPrompt)
	if promptTokens > manifest.MaxTokenBudget {
		return nil, fmt.Errorf("%w: compiled prompt requires %d tokens, exceeds budget %d", ErrTokenBudgetExceeded, promptTokens, manifest.MaxTokenBudget)
	}

	seq := s.workerSeq.Add(1)
	workerID := fmt.Sprintf("%s-worker-%d", strings.TrimSpace(manifest.ManifestID), seq)
	digest := computeRuntimeDigest(workerID, manifest, compiledPrompt)

	worker := &TransientWorker{
		WorkerID:       workerID,
		Manifest:       cloneManifest(manifest),
		SystemPrompt:   compiledPrompt,
		RuntimeDigest:  digest,
		State:          WorkerStatePending,
		TokensConsumed: promptTokens,
		CreatedAt:      time.Now().UTC(),
		capSet:         buildCapabilitySet(manifest.AllowedCapabilities),
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.activeWorkerCountLocked() >= s.cfg.MaxConcurrentWorkers {
		return nil, fmt.Errorf("%w: limit %d", ErrWorkerCapacityReached, s.cfg.MaxConcurrentWorkers)
	}
	s.workers[workerID] = worker

	copyWorker := cloneWorkerSnapshot(worker)
	return &copyWorker, nil
}

// ExecuteWorker enforces capability bounds, token budgets, and lifecycle state before recording execution.
func (s *DynamicAgentSynthesizer) ExecuteWorker(workerID, capability string, tokenCost int) error {
	if err := strutil.ErrorOnBlank(workerID, "%w: worker_id is required", ErrSynthesizerWorkerNotFound); err != nil {
		return err
	}
	if err := strutil.ErrorOnBlank(capability, "%w: capability is required", ErrCapabilityDenied); err != nil {
		return err
	}
	if tokenCost < 0 {
		return fmt.Errorf("%w: token cost cannot be negative", ErrTokenBudgetExceeded)
	}

	return s.withLockedWorker(workerID, func(w *TransientWorker) error {
		if w.State == WorkerStateTornDown {
			return fmt.Errorf("%w: %s", ErrWorkerTornDown, workerID)
		}
		if !w.HasCapability(capability) {
			return fmt.Errorf("%w: tool %q not permitted for worker %s", ErrCapabilityDenied, capability, workerID)
		}
		if w.TokensConsumed+tokenCost > w.Manifest.MaxTokenBudget {
			return fmt.Errorf("%w: %d + %d > %d", ErrTokenBudgetExceeded, w.TokensConsumed, tokenCost, w.Manifest.MaxTokenBudget)
		}
		w.State = WorkerStateRunning
		w.TokensConsumed += tokenCost
		return nil
	})
}

// TeardownWorker transitions a worker to torn_down state and clears ephemeral prompt state.
func (s *DynamicAgentSynthesizer) TeardownWorker(workerID string) error {
	if err := strutil.ErrorOnBlank(workerID, "%w: worker_id is required", ErrSynthesizerWorkerNotFound); err != nil {
		return err
	}
	return s.withLockedWorker(workerID, func(w *TransientWorker) error {
		markWorkerTornDown(w, time.Now().UTC())
		return nil
	})
}

// TeardownAll deterministically tears down all non-torn-down workers and returns the count cleaned up.
func (s *DynamicAgentSynthesizer) TeardownAll() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	cleaned := 0
	for _, w := range s.workers {
		if w.State != WorkerStateTornDown {
			markWorkerTornDown(w, now)
			cleaned++
		}
	}
	return cleaned
}

// GetWorker returns a thread-safe snapshot of a worker's state.
func (s *DynamicAgentSynthesizer) GetWorker(workerID string) (TransientWorker, error) {
	if err := strutil.ErrorOnBlank(workerID, "%w: worker_id is required", ErrSynthesizerWorkerNotFound); err != nil {
		return TransientWorker{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	w, ok := s.workers[workerID]
	if !ok {
		return TransientWorker{}, fmt.Errorf("%w: %s", ErrSynthesizerWorkerNotFound, workerID)
	}
	return cloneWorkerSnapshot(w), nil
}

// ActiveWorkerCount returns the number of workers not in torn_down state.
func (s *DynamicAgentSynthesizer) ActiveWorkerCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.activeWorkerCountLocked()
}

func (s *DynamicAgentSynthesizer) activeWorkerCountLocked() int {
	count := 0
	for _, w := range s.workers {
		if w.State != WorkerStateTornDown {
			count++
		}
	}
	return count
}

func (s *DynamicAgentSynthesizer) withLockedWorker(workerID string, fn func(*TransientWorker) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	w, ok := s.workers[workerID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrSynthesizerWorkerNotFound, workerID)
	}
	return fn(w)
}

func (s *DynamicAgentSynthesizer) enforceGlobalBounds(manifest AgentManifest) error {
	if s.cfg.MaxMemoryMB > 0 && manifest.Sandbox.MaxMemoryMB > s.cfg.MaxMemoryMB {
		return fmt.Errorf("%w: requested memory %dMB exceeds synthesizer cap %dMB", ErrSandboxViolation, manifest.Sandbox.MaxMemoryMB, s.cfg.MaxMemoryMB)
	}
	if len(s.globalCaps) > 0 {
		for _, capName := range manifest.AllowedCapabilities {
			if _, ok := s.globalCaps[strings.TrimSpace(capName)]; !ok {
				return fmt.Errorf("%w: capability %q outside global synthesizer allowlist", ErrCapabilityDenied, capName)
			}
		}
	}
	return nil
}

func buildCapabilitySet(caps []string) map[string]struct{} {
	if len(caps) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(caps))
	for _, c := range caps {
		set[strings.TrimSpace(c)] = struct{}{}
	}
	return set
}

func compileSystemPrompt(manifest AgentManifest, prompt string) string {
	caps := make([]string, len(manifest.AllowedCapabilities))
	for i, c := range manifest.AllowedCapabilities {
		caps[i] = strings.TrimSpace(c)
	}
	sort.Strings(caps)
	return fmt.Sprintf(
		"[role:%s|manifest:%s|caps:%s|net_iso:%t|ro_fs:%t|mem_mb:%d]\n%s",
		strings.TrimSpace(manifest.Role),
		strings.TrimSpace(manifest.ManifestID),
		strings.Join(caps, ","),
		manifest.Sandbox.NetworkIsolation,
		manifest.Sandbox.ReadOnlyRootFS,
		manifest.Sandbox.MaxMemoryMB,
		strings.TrimSpace(prompt),
	)
}

func estimateTokenCount(text string) int {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0
	}
	return (len(trimmed) + 3) / 4
}

func computeRuntimeDigest(workerID string, manifest AgentManifest, compiledPrompt string) string {
	payload := fmt.Sprintf("%s|%s|%s|%d|%s", workerID, manifest.ManifestID, manifest.Role, manifest.MaxTokenBudget, compiledPrompt)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func markWorkerTornDown(w *TransientWorker, at time.Time) {
	w.State = WorkerStateTornDown
	w.TornDownAt = at
	w.SystemPrompt = ""
}

func cloneManifest(m AgentManifest) AgentManifest {
	out := m
	out.AllowedCapabilities = append([]string(nil), m.AllowedCapabilities...)
	out.Sandbox.AllowedPaths = append([]string(nil), m.Sandbox.AllowedPaths...)
	return out
}

func cloneWorkerSnapshot(w *TransientWorker) TransientWorker {
	out := *w
	out.Manifest = cloneManifest(w.Manifest)
	out.capSet = buildCapabilitySet(w.Manifest.AllowedCapabilities)
	return out
}
