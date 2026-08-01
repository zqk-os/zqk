package objects

import "fmt"

// LoaderOrderError represents an error when loading order is violated
type LoaderOrderError struct {
	Message string
	Step    string // Which step failed
}

func (e *LoaderOrderError) Error() string {
	return fmt.Sprintf("loading order violation at %s: %s", e.Step, e.Message)
}

// LoadingOrder defines the required order for loading system components
// This ensures specs are loaded before instances, preventing validation issues
type LoadingOrder struct {
	specsLoaded map[string]bool // Track which specs have been loaded
}

// NewLoadingOrder creates a new loading order tracker
func NewLoadingOrder() *LoadingOrder {
	return &LoadingOrder{
		specsLoaded: make(map[string]bool),
	}
}

// MarkSpecLoaded marks a spec as loaded
func (lo *LoadingOrder) MarkSpecLoaded(ontology string) {
	lo.specsLoaded[ontology] = true
}

// RequireSpecLoaded ensures a spec is loaded before proceeding
func (lo *LoadingOrder) RequireSpecLoaded(ontology string) error {
	if !lo.specsLoaded[ontology] {
		return &LoaderOrderError{
			Message: fmt.Sprintf("spec for ontology '%s' must be loaded before loading instances", ontology),
			Step:    "instance_loading",
		}
	}
	return nil
}

// GetRequiredSpecs returns the list of specs that must be loaded before loading instances
func GetRequiredSpecs() []string {
	return []string{
		KindCriteria,         // Must load criteria.yaml before CRIT-*.yaml instances
		KindBaseObject,       // Base for most objects
		KindAuditable,        // Base for auditable objects
		KindExtensibleObject, // Base for extensible objects
	}
}
