package storage

import (
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
)

// ScenarioAdaptor is an interface for transforming captured baseline state
// for different test scenarios. Adaptors can be chained to apply multiple transformations.
type ScenarioAdaptor interface {
	// Adapt transforms the baseline state according to the adaptor's logic
	Adapt(baselineState map[string]any, config *AdaptorConfig) (map[string]any, error)

	// Name returns the name of the adaptor (e.g., "id-remap", "field-override")
	Name() string
}

// scenarioAdaptorNameNamespace is the createAdaptor switch keyword for NamespaceAdaptor (same spelling as objects.KindNamespace).
const scenarioAdaptorNameNamespace = objects.KindNamespace

// AdaptorConfig contains configuration for adaptors
type AdaptorConfig struct {
	// IDPrefix is the prefix to use for ID remapping (e.g., "TEST-")
	IDPrefix string

	// Namespace is the namespace to apply (e.g., "test:scenario")
	Namespace string

	// FieldOverrides is a map of field names to override values
	FieldOverrides map[string]any

	// ExcludePII indicates whether to exclude PII fields
	ExcludePII bool

	// PreserveState indicates whether to preserve object state (status, timestamps)
	PreserveState bool

	// IDMapping tracks original ID -> new ID mappings for reference updates
	IDMapping map[string]string
}

// AdaptorChain chains multiple adaptors together to apply sequential transformations
type AdaptorChain struct {
	adaptors []ScenarioAdaptor
	config   *AdaptorConfig
}

// NewAdaptorChain creates a new adaptor chain with the specified adaptors
func NewAdaptorChain(adaptorNames []string, config *AdaptorConfig) (*AdaptorChain, error) {
	if config == nil {
		config = &AdaptorConfig{
			IDMapping: make(map[string]string),
		}
	} else if config.IDMapping == nil {
		config.IDMapping = make(map[string]string)
	}

	adaptors := make([]ScenarioAdaptor, 0, len(adaptorNames))

	for _, name := range adaptorNames {
		adaptor, err := createAdaptor(name)
		if err != nil {
			return nil, errfmt.Errorf(ConstMiscFailedToCreateAdaptorSW, name, err)
		}
		adaptors = append(adaptors, adaptor)
	}

	return &AdaptorChain{
		adaptors: adaptors,
		config:   config,
	}, nil
}

// Apply applies all adaptors in sequence to the baseline state
func (ac *AdaptorChain) Apply(baselineState map[string]any) (map[string]any, error) {
	result := baselineState

	for _, adaptor := range ac.adaptors {
		var err error
		result, err = adaptor.Adapt(result, ac.config)
		if err != nil {
			return nil, errfmt.Newf(ConstMiscAdaptorSFailed, adaptor.Name()).Wrap(err)
		}
	}

	return result, nil
}

// createAdaptor creates an adaptor instance by name
func createAdaptor(name string) (ScenarioAdaptor, error) {
	switch strings.ToLower(name) {
	case "id-remap", "id_remap", "idremap":
		return &IDRemappingAdaptor{}, nil
	case ConstMiscFieldOverride, ConstMiscFieldOverride1, "fieldoverride":
		return &FieldOverrideAdaptor{}, nil
	case "pii-filter", "pii_filter", "piifilter":
		return &PIIFilterAdaptor{}, nil
	case scenarioAdaptorNameNamespace, ConstMiscNamespaceAdaptor, ConstMiscNamespaceAdaptor1:
		return &NamespaceAdaptor{}, nil
	case ConstMiscStatePreserve, ConstMiscStatePreserve1, "statepreserve":
		return &StatePreservationAdaptor{}, nil
	default:
		return nil, errfmt.Errorf(ConstMiscUnknownAdaptorS, name)
	}
}

// IDRemappingAdaptor remaps object IDs to use a new prefix (e.g., TEST-001, TEST-002)
type IDRemappingAdaptor struct{}

func (a *IDRemappingAdaptor) Name() string {
	return "id-remap"
}

func (a *IDRemappingAdaptor) Adapt(obj map[string]any, config *AdaptorConfig) (map[string]any, error) {
	if config == nil || config.IDPrefix == emptyValue {
		return obj, nil // No remapping if no prefix configured
	}

	originalID, ok := obj[objects.FieldKeyID].(string)
	if !ok || originalID == emptyValue {
		return obj, nil // No ID to remap
	}

	// Check if already mapped
	if newID, exists := config.IDMapping[originalID]; exists {
		obj[objects.FieldKeyID] = newID
		return obj, nil
	}

	// Generate new ID based on prefix and sequence
	// Extract sequence number from original ID (e.g., "BLI-001" -> "001")
	parts := strings.Split(originalID, "-")
	var sequence string
	if len(parts) > 1 {
		sequence = parts[len(parts)-1]
	} else {
		// No dash, use last part or generate sequence
		sequence = fmt.Sprintf("%03d", len(config.IDMapping)+1)
	}

	newID := config.IDPrefix + sequence

	// Store mapping
	config.IDMapping[originalID] = newID

	// Update object ID
	obj[objects.FieldKeyID] = newID

	return obj, nil
}

// FieldOverrideAdaptor applies field overrides to objects
type FieldOverrideAdaptor struct{}

func (a *FieldOverrideAdaptor) Name() string {
	return ConstMiscFieldOverride
}

func (a *FieldOverrideAdaptor) Adapt(obj map[string]any, config *AdaptorConfig) (map[string]any, error) {
	if config == nil || len(config.FieldOverrides) == 0 {
		return obj, nil
	}

	// Apply each field override
	for field, value := range config.FieldOverrides {
		obj[field] = value
	}

	return obj, nil
}

// PIIFilterAdaptor removes PII (Personally Identifiable Information) fields
type PIIFilterAdaptor struct{}

func (a *PIIFilterAdaptor) Name() string {
	return "pii-filter"
}

func (a *PIIFilterAdaptor) Adapt(obj map[string]any, config *AdaptorConfig) (map[string]any, error) {
	if config == nil || !config.ExcludePII {
		return obj, nil
	}

	// Common PII field names to exclude
	piiFields := map[string]bool{
		objects.FieldKeyEmail:   true,
		"phone":                 true,
		"phone_number":          true,
		"ssn":                   true,
		ConstMiscSocialSecurity: true,
		"credit_card":           true,
		"password":              true,
		"password_hash":         true,
		"api_key":               true,
		"secret":                true,
		"token":                 true,
		"personal_info":         true,
		"address":               true,
		ConstMiscStreetAddress:  true,
		"city":                  true,
		"zip_code":              true,
		"postal_code":           true,
		"date_of_birth":         true,
		"birth_date":            true,
	}

	// Remove PII fields
	for field := range piiFields {
		delete(obj, field)
	}

	// Also check for fields with "pii" in the name (case-insensitive)
	for key := range obj {
		if strings.Contains(strings.ToLower(key), "pii") {
			delete(obj, key)
		}
	}

	return obj, nil
}

// NamespaceAdaptor changes the namespace of objects
type NamespaceAdaptor struct{}

func (a *NamespaceAdaptor) Name() string {
	return scenarioAdaptorNameNamespace
}

func (a *NamespaceAdaptor) Adapt(obj map[string]any, config *AdaptorConfig) (map[string]any, error) {
	if config == nil || config.Namespace == emptyValue {
		return obj, nil
	}

	obj[objects.FieldKeyNamespaceID] = config.Namespace

	return obj, nil
}

// StatePreservationAdaptor preserves or resets object state (status, timestamps)
type StatePreservationAdaptor struct{}

func (a *StatePreservationAdaptor) Name() string {
	return ConstMiscStatePreserve
}

func (a *StatePreservationAdaptor) Adapt(obj map[string]any, config *AdaptorConfig) (map[string]any, error) {
	if config == nil {
		return obj, nil
	}

	if !config.PreserveState {
		// Reset state fields to defaults
		// Remove status (or set to default)
		if _, exists := obj[objects.FieldKeyStatus]; exists {
			obj[objects.FieldKeyStatus] = "exploring" // Default status
		}

		// Reset timestamps to current time
		// Note: This would typically use time.Now(), but for snapshot scenarios,
		// we might want to preserve timestamps. This is a design decision.
		// For now, we'll preserve timestamps unless explicitly reset.
	}

	// If PreserveState is true, do nothing (preserve current state)

	return obj, nil
}
