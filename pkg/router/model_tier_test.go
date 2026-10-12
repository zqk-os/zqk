package router

import (
	"testing"
)

func TestModelTier_Validation(t *testing.T) {
	t.Parallel()

	validTiers := []ModelTier{TierFast, TierStandard, TierHeavy, TierLocal}
	for _, tier := range validTiers {
		if !tier.Valid() {
			t.Errorf("expected tier %q to be valid", tier)
		}
		if tier.String() != string(tier) {
			t.Errorf("expected String()=%q, got %q", string(tier), tier.String())
		}
	}

	invalidTiers := []ModelTier{"", "quantum", "unknown", "superfast"}
	for _, tier := range invalidTiers {
		if tier.Valid() {
			t.Errorf("expected tier %q to be invalid", tier)
		}
	}
}

func TestClassifyModelTier_Heuristics(t *testing.T) {
	t.Parallel()

	cases := []struct {
		model    string
		expected ModelTier
	}{
		// Fast tier
		{"gemini-2.5-flash", TierFast},
		{"claude-3-haiku-20240307", TierFast},
		{"gpt-4o-mini", TierFast},
		{"llama-3-8b-instruct-turbo", TierFast},
		{"custom-fast-agent", TierFast},
		{"qwen-2.5-small", TierFast},

		// Standard tier
		{"claude-3-5-sonnet", TierStandard},
		{"gemini-1.5-pro", TierStandard},
		{"gpt-4o", TierStandard},
		{"deepseek-chat", TierStandard},
		{"unknown-vendor-standard-v1", TierStandard},
		{"", TierStandard},
		{"   ", TierStandard},

		// Heavy tier
		{"o1-preview", TierHeavy},
		{"o3-mini", TierHeavy},
		{"claude-3-opus", TierHeavy},
		{"deepseek-r1-distill", TierHeavy},
		{"qwen-qwq-32b-preview", TierHeavy},
		{"thinking-agent-v2", TierHeavy},

		// Local tier
		{"ollama/llama3:8b", TierLocal},
		{"local/deepseek-coder", TierLocal},
		{"qwen-local-14b", TierLocal},
		{"llama-3.2-3b", TierLocal},
		{"phi-3-mini", TierLocal},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.model, func(t *testing.T) {
			t.Parallel()
			got := ClassifyModelTier(tc.model)
			if got != tc.expected {
				t.Errorf("ClassifyModelTier(%q) = %q, expected %q", tc.model, got, tc.expected)
			}
		})
	}
}

func TestRegisterModelTier_Override(t *testing.T) {
	defer ClearModelTierOverrides()

	const customModel = "my-special-enterprise-model"
	if got := ClassifyModelTier(customModel); got != TierStandard {
		t.Fatalf("expected default %q, got %q", TierStandard, got)
	}

	RegisterModelTier(customModel, TierHeavy)
	if got := ClassifyModelTier(customModel); got != TierHeavy {
		t.Fatalf("expected overridden %q, got %q", TierHeavy, got)
	}

	// Verify case and whitespace insensitivity
	if got := ClassifyModelTier("  MY-SPECIAL-ENTERPRISE-MODEL  "); got != TierHeavy {
		t.Fatalf("expected case-insensitive match %q, got %q", TierHeavy, got)
	}
}
