package llm

import (
	"testing"
)

func TestOpenWeightPresets_ValidateAndRegister(t *testing.T) {
	presets := OpenWeightPresets()
	if len(presets) < 5 {
		t.Fatalf("expected at least 5 open-weight presets, got %d", len(presets))
	}

	reg := NewEngineRegistry()
	if err := RegisterOpenWeightPresets(reg); err != nil {
		t.Fatalf("RegisterOpenWeightPresets failed: %v", err)
	}

	for _, eng := range []ProviderEngineType{EngineOllama, EngineTogether, EngineFireworks, EngineBaseten, EngineGroq, EngineOpenRouter, EngineDeepSeek} {
		preset, ok := LookupOpenWeightPreset(eng)
		if !ok {
			t.Fatalf("expected preset %q to be discoverable", eng)
		}
		if preset.BaseURL == "" || preset.DefaultModel == "" {
			t.Errorf("preset %q has incomplete metadata: %+v", eng, preset)
		}
		profile, regOK := reg.Get(eng)
		if !regOK {
			t.Errorf("expected preset %q registered in EngineRegistry", eng)
		}
		if profile.BaseURL != preset.BaseURL {
			t.Errorf("registered BaseURL mismatch for %q: got %q, want %q", eng, profile.BaseURL, preset.BaseURL)
		}
	}

	if err := RegisterOpenWeightPresets(nil); err == nil {
		t.Errorf("expected error when registering presets into nil registry")
	}
}
