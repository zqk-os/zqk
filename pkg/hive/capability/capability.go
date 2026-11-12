package capability

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/zqk-os/zqk/pkg/concurrency"
)

var (
	ErrCapabilityNotFound = errors.New("capability not found")
	ErrCapabilityExists   = errors.New("capability already registered")
	ErrPolicyRejected     = errors.New("capability policy evaluation rejected request")
	ErrTestCaseRequired   = errors.New("capability synthesis policy requires at least one associated test_case")
)

// EvalContext provides the context for evaluating a capability policy.
type EvalContext struct {
	AgentID        string
	WorkspaceDir   string
	RequestedScope map[string]string
}

// Capability defines a permissioned ability within the Hive Mind.
type Capability interface {
	Name() string
	Description() string
	EvaluatePolicy(ctx context.Context, evalCtx EvalContext) (bool, error)
}

// Registry manages the registration and lookup of capabilities.
type Registry interface {
	Register(c Capability) error
	Get(name string) (Capability, error)
	List() []Capability
}

type defaultRegistry struct {
	mu           sync.RWMutex
	capabilities map[string]Capability
}

// NewRegistry creates a new capability registry.
func NewRegistry() Registry {
	return &defaultRegistry{
		capabilities: make(map[string]Capability),
	}
}

func (r *defaultRegistry) Register(c Capability) error {
	return concurrency.RunInLock(&r.mu, func() error {
		if _, exists := r.capabilities[c.Name()]; exists {
			return ErrCapabilityExists
		}
		r.capabilities[c.Name()] = c
		return nil
	})
}

func (r *defaultRegistry) Get(name string) (Capability, error) {
	var c Capability
	err := concurrency.RunInRLock(&r.mu, func() error {
		capVal, ok := r.capabilities[name]
		if !ok {
			return ErrCapabilityNotFound
		}
		c = capVal
		return nil
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (r *defaultRegistry) List() []Capability {
	var list []Capability
	_ = concurrency.RunInRLock(&r.mu, func() error {
		list = make([]Capability, 0, len(r.capabilities))
		for _, c := range r.capabilities {
			list = append(list, c)
		}
		return nil
	})
	return list
}

// Authorize evaluates whether the named capability may proceed.
// Deny-by-default: nil registry, empty name, unknown capability, or a false
// policy result all return ErrPolicyRejected. Policy evaluation errors fail closed
// as ErrPolicyRejected (wrapped) so callers can recover on a typed denial.
func Authorize(ctx context.Context, reg Registry, name string, evalCtx EvalContext) error {
	if reg == nil {
		return ErrPolicyRejected
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrPolicyRejected
	}
	c, err := reg.Get(name)
	if err != nil {
		return ErrPolicyRejected
	}
	ok, err := c.EvaluatePolicy(ctx, evalCtx)
	if err != nil {
		return errors.Join(ErrPolicyRejected, err)
	}
	if !ok {
		return ErrPolicyRejected
	}
	return nil
}

// StaticCapability is a fixed allow/deny capability for demos and tests.
type StaticCapability struct {
	CapName        string
	CapDescription string
	Allow          bool
}

// Name implements Capability.
func (s StaticCapability) Name() string { return s.CapName }

// Description implements Capability.
func (s StaticCapability) Description() string { return s.CapDescription }

// EvaluatePolicy implements Capability.
func (s StaticCapability) EvaluatePolicy(ctx context.Context, evalCtx EvalContext) (bool, error) {
	return s.Allow, nil
}

// SynthesisPipeline enforces that capability synthesis requires a valid test_case reference.
type SynthesisPipeline struct {
	registry Registry
}

// NewSynthesisPipeline creates a new capability synthesis pipeline.
func NewSynthesisPipeline(r Registry) *SynthesisPipeline {
	return &SynthesisPipeline{registry: r}
}

// Synthesize validates that capability synthesis has at least one associated test_case before registration.
func (p *SynthesisPipeline) Synthesize(c Capability, testCaseRefs []string) error {
	validCount := 0
	for _, ref := range testCaseRefs {
		if strings.TrimSpace(ref) != "" {
			validCount++
		}
	}
	if validCount == 0 {
		return ErrTestCaseRequired
	}
	return p.registry.Register(c)
}
