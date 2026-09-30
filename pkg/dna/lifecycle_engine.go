package dna

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
)


func (p Plane) String() string {
	switch p {
	case PlaneDraft:
		return "Draft"
	case PlaneStaged:
		return "Staged"
	case PlanePromoted:
		return "Promoted"
	case PlaneApoptotic:
		return "Apoptotic"
	default:
		return "Unknown"
	}
}

// ParsePlane parses a plane string representation.
func ParsePlane(s string) (Plane, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "draft", "planedraft", "0":
		return PlaneDraft, nil
	case "staged", "planestaged", "1":
		return PlaneStaged, nil
	case "promoted", "planepromoted", "2":
		return PlanePromoted, nil
	case "apoptotic", "planeapoptotic", "3":
		return PlaneApoptotic, nil
	default:
		return PlaneDraft, fmt.Errorf("invalid plane: %q", s)
	}
}

// IsKnown reports whether p is a declared storage profile.
func (p StorageProfile) IsKnown() bool {
	switch p {
	case StorageProfileCASEntity, StorageProfileStream, StorageProfileLightFile:
		return true
	default:
		return false
	}
}

// String returns the wire string representation.
func (p StorageProfile) String() string {
	return string(p)
}

// ParseStorageProfile validates and converts a string into a StorageProfile.
func ParseStorageProfile(s string) (StorageProfile, error) {
	trimmed := strings.ToLower(strings.TrimSpace(s))
	if trimmed == "" {
		return "", nil
	}
	p := StorageProfile(trimmed)
	if !p.IsKnown() {
		return "", errfmt.Errorf("unknown storage_profile %q (expected cas_entity, stream, or light_file)", s)
	}
	return p, nil
}

// NewBaseObject instantiates a BaseObject initialized to PlaneDraft.
func NewBaseObject(urn URN, schemaRef string) BaseObject {
	now := time.Now().UTC()
	return BaseObject{
		URN:       urn,
		Kind:      urn.Kind,
		SchemaRef: schemaRef,
		Plane:     PlaneDraft,
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// Validate checks standard invariants on BaseObject.
func (b *BaseObject) Validate() error {
	if b.URN.String() == "" {
		return fmt.Errorf("base_object: missing urn")
	}
	if b.Kind != "" && b.Kind != b.URN.Kind {
		return fmt.Errorf("base_object: kind mismatch (urn says %q, field says %q)", b.URN.Kind, b.Kind)
	}
	if b.URN.Cell == "" {
		return fmt.Errorf("base_object: urn cell_id is empty")
	}
	return nil
}

// GetURN returns the canonical URN for this entity.
func (b BaseObject) GetURN() URN { return b.URN }

// GetKind returns the kind of this entity.
func (b BaseObject) GetKind() string { return b.Kind }

// GetPlane returns the transactional isolation plane of this entity.
func (b BaseObject) GetPlane() Plane { return b.Plane }

// GetVersion returns the entity version.
func (b BaseObject) GetVersion() uint64 { return b.Version }

// GetCreatedAt returns the timestamp when this entity was instantiated.
func (b BaseObject) GetCreatedAt() time.Time { return b.CreatedAt }

// GetUpdatedAt returns the timestamp when this entity was last updated.
func (b BaseObject) GetUpdatedAt() time.Time { return b.UpdatedAt }

// NewLifecycleFacet instantiates a LifecycleFacet.
func NewLifecycleFacet(macroPhase MacroPhase, subStatus string, plane Plane, transitions map[string][]string) LifecycleFacet {
	return LifecycleFacet{
		MacroPhase:         macroPhase,
		SubStatus:          subStatus,
		Plane:              plane,
		AllowedTransitions: transitions,
		gates:              make(map[string][]InvariantGateFunc),
		mu:                 &sync.RWMutex{},
	}
}

// NewLifecycle constructs a new Lifecycle instance.
func NewLifecycle(initialStatus string, allowedTransitions map[string][]string) Lifecycle {
	return Lifecycle{
		Status:             initialStatus,
		AllowedTransitions: allowedTransitions,
		gates:              make(map[string][]InvariantGateFunc),
		mu:                 &sync.RWMutex{},
	}
}

func (l *Lifecycle) Phase() string {
	if l.mu != nil {
		l.mu.RLock()
		defer l.mu.RUnlock()
	}
	return l.Status
}

func (l *Lifecycle) CanTransition(target string) bool {
	if l.mu != nil {
		l.mu.RLock()
		defer l.mu.RUnlock()
	}
	allowed, exists := l.AllowedTransitions[l.Status]
	if !exists {
		return false
	}
	for _, next := range allowed {
		if strings.EqualFold(next, target) {
			return true
		}
	}
	return false
}

func (l *Lifecycle) RegisterInvariantGate(status string, fn InvariantGateFunc) {
	if l.mu == nil {
		l.mu = &sync.RWMutex{}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.gates == nil {
		l.gates = make(map[string][]InvariantGateFunc)
	}
	l.gates[status] = append(l.gates[status], fn)
}

func (l *Lifecycle) Transition(ctx context.Context, obj any, targetStatus string) error {
	if !l.CanTransition(targetStatus) {
		return errfmt.Errorf("illegal lifecycle transition from %q to %q", l.Status, targetStatus)
	}

	if l.mu != nil {
		l.mu.RLock()
	}
	gates := l.gates[targetStatus]
	if l.mu != nil {
		l.mu.RUnlock()
	}

	for _, gate := range gates {
		if err := gate(ctx, obj); err != nil {
			return err
		}
	}

	if l.mu == nil {
		l.mu = &sync.RWMutex{}
	}
	l.mu.Lock()
	l.Status = targetStatus
	l.mu.Unlock()

	return nil
}

func (l *Lifecycle) ApoptosisPolicy() string {
	return "quarantine_on_stale_claim_300s"
}
