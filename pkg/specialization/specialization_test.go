package specialization

import (
	"os"
	"testing"

	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestGetCurrentTier(t *testing.T) {
	envVar := zqkenv.SpecializationTier()

	tests := []struct {
		name     string
		envVal   string
		defVal   string
		expected Tier
	}{
		{"No env, no def", "", "", TierAll},
		{"Env neuron", "neuron", "", TierNeuron},
		{"Def muscle", "", "muscle", TierMuscle},
		{"Env over def", "heart", "lung", TierHeart},
		{"Invalid env", "invalid", "", TierAll},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv(envVar, tt.envVal)
			DefaultTier = tt.defVal
			if got := GetCurrentTier(); got != tt.expected {
				t.Errorf("GetCurrentTier() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestTier_ShouldRun(t *testing.T) {
	tests := []struct {
		tier      Tier
		component string
		expected  bool
	}{
		{TierAll, "reconciler", true},
		{TierNeuron, "reconciler", true},
		{TierNeuron, "scheduler", false},
		{TierMuscle, "scheduler", true},
		{TierMuscle, "reconciler", false},
		{TierHeart, "metrics", true},
		{TierLung, "mesh", true},
		{TierNeuron, "shared-infra", true},
	}

	for _, tt := range tests {
		t.Run(string(tt.tier)+"-"+tt.component, func(t *testing.T) {
			if got := tt.tier.ShouldRun(tt.component); got != tt.expected {
				t.Errorf("Tier.ShouldRun() = %v, want %v", got, tt.expected)
			}
		})
	}
}
