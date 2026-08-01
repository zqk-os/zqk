package mcp

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

type testStorageProvider struct{}

func (p *testStorageProvider) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter any) (any, error) {
	// Return a dummy role object to prevent panic in generateRoleSpecificGuidance
	return map[string]any{
		"objects": []map[string]any{
			{
				objects.FieldKeyID:          "role-viewer",
				objects.FieldKeyDescription: "Viewer role for tests",
				objects.FieldKeyPermissions: []any{"read:*"},
			},
		},
	}, nil
}

func (p *testStorageProvider) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	return nil
}

func (p *testStorageProvider) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	return nil, fmt.Errorf("Read not implemented")
}

func TestPersonaContextFilter_HydrationVariesByPersona(t *testing.T) {
	// Satisfies CRIT-REDACTED (Verify prompt hydration varies strictly by assignee_persona_ref)
	ctx := &ProjectContext{
		Mission: &MissionContext{ID: "M-1", Title: "Mission 1", Statement: "Mission Statement"},
		Vision:  &VisionContext{ID: "V-1", Title: "Vision 1", Statement: "Vision Statement"},
		CurrentState: &CurrentStateContext{
			ActivePriorityPlans: []PriorityPlanSummary{{ID: "PP-1", Title: "Plan 1", Status: "active", Priority: "high"}},
		},
	}

	tests := []struct {
		name    string
		persona string
		wantYes []string
		wantNo  []string
	}{
		{
			name:    "Coder Persona truncates Layer 1",
			persona: "coder",
			wantYes: []string{},
			wantNo:  []string{"Project Mission and Vision", "Active Priority Plans"},
		},
		{
			name:    "TPM Persona expands Layer 1 and Dependency Graph",
			persona: "tpm",
			wantYes: []string{"Project Mission and Vision", "Dependency Graph", "Active Priority Plans"},
			wantNo:  []string{"Observer Operational Metrics"},
		},
		{
			name:    "Observer Persona adds Metrics",
			persona: "observer",
			wantYes: []string{"Observer Operational Metrics", "Project Mission and Vision"},
			wantNo:  []string{"Dependency Graph"},
		},
		{
			name:    "IA Persona adds Ontology",
			persona: "ia",
			wantYes: []string{"Information Architecture", "Project Mission and Vision"},
			wantNo:  []string{"Observer Operational Metrics"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage := &testStorageProvider{}
			gen := NewRoleAwarePromptGenerator(ctx, []string{"viewer"}).
				WithAssigneePersona(tt.persona).
				WithStorageProvider(storage).
				WithRoleGuidanceGenerator(NewRoleGuidanceGenerator(storage)).
				WithSecurityContext(pkgctx.NewSystemSecurityContext())

			prompt := gen.GenerateBigPicturePrompt()

			for _, yes := range tt.wantYes {
				if !strings.Contains(prompt, yes) {
					t.Errorf("Expected prompt to contain %q for persona %q", yes, tt.persona)
				}
			}

			for _, no := range tt.wantNo {
				if strings.Contains(prompt, no) {
					t.Errorf("Expected prompt NOT to contain %q for persona %q", no, tt.persona)
				}
			}
		})
	}
}

func TestPersonaContextFilter_HydrationPerformance(t *testing.T) {
	// Satisfies CRIT-REDACTED (Ensure hydration logic takes < 50ms)
	ctx := &ProjectContext{
		Mission: &MissionContext{ID: "M-1", Title: "Mission 1"},
		CurrentState: &CurrentStateContext{
			ActivePriorityPlans: []PriorityPlanSummary{{ID: "PP-1", Title: "Plan 1"}},
		},
	}

	storage := &testStorageProvider{}
	gen := NewRoleAwarePromptGenerator(ctx, []string{"viewer"}).
		WithAssigneePersona("tpm").
		WithStorageProvider(storage).
		WithRoleGuidanceGenerator(NewRoleGuidanceGenerator(storage)).
		WithSecurityContext(pkgctx.NewSystemSecurityContext())

	start := time.Now()
	// Run it multiple times to ensure stability
	for i := 0; i < 100; i++ {
		_ = gen.GenerateBigPicturePrompt()
	}
	duration := time.Since(start) / 100

	if duration > 50*time.Millisecond {
		t.Errorf("Hydration took too long: %v per call, max allowed is 50ms", duration)
	}
}

// Tests storage fetching
func TestPersonaContextFilter_StorageMetricsPull(t *testing.T) {
	ctx := &ProjectContext{}
	storage := &testStorageProvider{}
	gen := NewRoleAwarePromptGenerator(ctx, []string{"viewer"}).
		WithAssigneePersona("observer").
		WithStorageProvider(storage).
		WithRoleGuidanceGenerator(NewRoleGuidanceGenerator(storage)).
		WithSecurityContext(pkgctx.NewSystemSecurityContext())

	prompt := gen.GenerateBigPicturePrompt()
	if !strings.Contains(prompt, "Observer Operational Metrics") {
		t.Errorf("Missing observer section")
	}
}
