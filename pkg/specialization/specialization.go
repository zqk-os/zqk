package specialization

import (
	"strings"

	"github.com/lanceman/zqk/pkg/zqkenv"
)

// Tier represents a cellular specialization tier of a ZQK node.
type Tier string

const (
	// TierAll is the default monolithic tier that runs all services.
	TierAll Tier = "all"
	// TierNeuron focuses on sensing, inference, and maturity assessment.
	TierNeuron Tier = "neuron"
	// TierMuscle focuses on execution, synthesis, and ingestion.
	TierMuscle Tier = "muscle"
	// TierHeart focuses on vitality, telemetry, and health monitoring.
	TierHeart Tier = "heart"
	// TierLung focuses on mesh communication and peering.
	TierLung Tier = "lung"
)

// Mode represents the operational mode of a specialization layer.
type Mode string

const (
	// ModeNormal is the default operational mode where actions affect the main spine.
	ModeNormal Mode = "normal"
	// ModeShielded operates within a 'Rubber Room' (ShadowSpine) to isolate side-effects.
	ModeShielded Mode = "shielded"
)

// DefaultTier is set via ldflags to specialize the binary at build time.
var DefaultTier string

// GetCurrentTier returns the active specialization tier from the environment.
func GetCurrentTier() Tier {
	envTier := zqkenv.SpecializationTier().Get()
	if envTier == "" {
		envTier = DefaultTier
	}
	t := Tier(strings.ToLower(strings.TrimSpace(envTier)))
	switch t {
	case TierNeuron, TierMuscle, TierHeart, TierLung:
		return t
	default:
		return TierAll
	}
}

// ShouldRun returns true if the given service/component should run in the current tier.
func (t Tier) ShouldRun(component string) bool {
	if t == TierAll {
		return true
	}

	switch component {
	case "reconciler", "assessor", "inference":
		return t == TierNeuron
	case "scheduler", "importer", "docman":
		return t == TierMuscle
	case "metrics", "health":
		return t == TierHeart
	case "mesh", "federation":
		return t == TierLung
	default:
		return true // Infrastructure/Shared services always run
	}
}

// GetHandlersForTier returns the set of handlers that should be engaged for the given tier.
func GetHandlersForTier(t Tier) []Handler {
	switch t {
	case TierNeuron:
		return []Handler{&NeuronHandler{}}
	case TierMuscle:
		return []Handler{&MuscleHandler{}}
	case TierHeart:
		return []Handler{&HeartHandler{}}
	case TierAll:
		return []Handler{&NeuronHandler{}, &MuscleHandler{}, &HeartHandler{}}
	default:
		return nil
	}
}
