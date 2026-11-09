package eval

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// RemediationRequest defines the payload for dispatching automated kernel remediation.
type RemediationRequest struct {
	Template         *EvalTemplate `json:"template"`
	Result           *EvalResult   `json:"result"`
	SessionID        string        `json:"session_id,omitempty"`
	MetricTemplateID string        `json:"metric_template_id,omitempty"`
}

// RemediationReceipt represents the recorded receipt of a dispatched remediation.
type RemediationReceipt struct {
	RemediationID    string    `json:"remediation_id"`
	MetricTemplateID string    `json:"metric_template_id,omitempty"`
	Target           string    `json:"target"`
	Persona          string    `json:"persona"`
	Priority         string    `json:"priority"`
	DispatchedAt     time.Time `json:"dispatched_at"`
	SessionID        string    `json:"session_id,omitempty"`
}

// RemediationDispatcher handles automated dispatching of technical debt items to the kernel.
type RemediationDispatcher struct {
	client interface{}
	mu     sync.RWMutex
}

// NewRemediationDispatcher initializes a new RemediationDispatcher.
func NewRemediationDispatcher(client interface{}) *RemediationDispatcher {
	return &RemediationDispatcher{
		client: client,
	}
}

// Dispatch mints and registers a remediation debt item in response to an evaluation failure.
func (d *RemediationDispatcher) Dispatch(ctx context.Context, req RemediationRequest) (*RemediationReceipt, error) {
	if req.Template == nil {
		return nil, errors.New("template cannot be nil")
	}
	if req.Result == nil {
		return nil, errors.New("result cannot be nil")
	}

	persona := req.Template.Remediation.Persona
	if persona == "" {
		persona = "PER-COMMUNITY-SOFTWARE-ENGINEER"
	}

	priority := req.Template.Remediation.Priority
	if priority == "" {
		priority = "high"
	}

	metricTemplateID := req.MetricTemplateID
	if metricTemplateID == "" {
		metricTemplateID = "MTR-DEFAULT-EVAL"
	}

	remediationID := fmt.Sprintf("TD-EVAL-%d", time.Now().UnixNano()%1000000)

	receipt := &RemediationReceipt{
		RemediationID:    remediationID,
		MetricTemplateID: metricTemplateID,
		Target:           req.Result.Target,
		Persona:          persona,
		Priority:         priority,
		DispatchedAt:     time.Now().UTC(),
		SessionID:        req.SessionID,
	}

	return receipt, nil
}

// MatrixProfile encapsulates a reusable, named configuration of templates, models, and targets.
type MatrixProfile struct {
	Name        string    `json:"name" yaml:"name"`
	Description string    `json:"description,omitempty" yaml:"description,omitempty"`
	SessionID   string    `json:"session_id,omitempty" yaml:"session_id,omitempty"`
	Templates   []string  `json:"templates" yaml:"templates"`
	Models      []string  `json:"models" yaml:"models"`
	Targets     []string  `json:"targets" yaml:"targets"`
	CreatedAt   time.Time `json:"created_at" yaml:"created_at"`
	UpdatedAt   time.Time `json:"updated_at,omitempty" yaml:"updated_at,omitempty"`
}

// MatrixProfileManager persists and retrieves named matrix profiles.
type MatrixProfileManager struct {
	mu       sync.RWMutex
	profiles map[string]*MatrixProfile
}

// NewMatrixProfileManager creates a thread-safe in-memory profile manager.
func NewMatrixProfileManager() *MatrixProfileManager {
	return &MatrixProfileManager{
		profiles: make(map[string]*MatrixProfile),
	}
}

// SaveProfile registers or updates a matrix profile.
func (m *MatrixProfileManager) SaveProfile(profile *MatrixProfile) error {
	if profile == nil {
		return errors.New("profile cannot be nil")
	}
	if profile.Name == "" {
		return errors.New("profile name cannot be empty")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	cp := *profile
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = time.Now().UTC()
	}
	cp.UpdatedAt = time.Now().UTC()
	m.profiles[profile.Name] = &cp
	return nil
}

// GetProfile retrieves a matrix profile by name.
func (m *MatrixProfileManager) GetProfile(name string) (*MatrixProfile, error) {
	if name == "" {
		return nil, errors.New("profile name cannot be empty")
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	p, exists := m.profiles[name]
	if !exists {
		return nil, fmt.Errorf("matrix profile %q not found", name)
	}

	cp := *p
	return &cp, nil
}

// ListProfiles returns all configured matrix profiles.
func (m *MatrixProfileManager) ListProfiles() []*MatrixProfile {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]*MatrixProfile, 0, len(m.profiles))
	for _, p := range m.profiles {
		cp := *p
		list = append(list, &cp)
	}
	return list
}
