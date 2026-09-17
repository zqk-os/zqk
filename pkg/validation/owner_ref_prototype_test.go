package validation

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	_ "github.com/lanceman/zqk/pkg/specbuilder/bldr_v2" // Register builders for tests
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// TestValidateOwnerRefPrototypeRejection tests that owner_ref pointing to prototype/test ACC objects is rejected
func TestValidateOwnerRefPrototypeRejection(t *testing.T) {
	t.Parallel()

	// Use real specs and lifecycles instead of empty tmp dirs
	specsDir := filepath.Join("..", "..", paths.ProcessInternalObjectSpecsDir)
	lifecyclesDir := filepath.Join("..", "..", paths.ProcessInternalLifecyclesDir)

	specLoader := objects.NewSpecLoader(specsDir)
	// Set builder registry on spec loader to enable version-aware loading
	builderRegistry := builders.GetGlobalRegistry()
	adapter := builders.NewSpecLoaderAdapter(builderRegistry)
	specLoader.SetBuilderRegistry(adapter)

	goValidator := NewGoValidatorWithLoaders(specLoader, objects.NewLifecycleLoader(lifecyclesDir))

	tests := []struct {
		name          string
		ownerRef      string
		wantErr       bool
		errorContains string
		description   string
	}{
		{
			name:          "prototype account owner_ref rejected",
			ownerRef:      "ACC-TEST-001",
			wantErr:       true,
			errorContains: "Prototype/test ACC on production owner_ref",
			description:   "Should reject TEST prefixed ACC as owner_ref",
		},
		{
			name:          "proptypo account owner_ref rejected",
			ownerRef:      "ACC-PVT-001",
			wantErr:       true,
			errorContains: "Prototype/test ACC on production owner_ref",
			description:   "Should reject PVT prefixed ACC as owner_ref",
		},
		{
			name:          "development account owner_ref rejected",
			ownerRef:      "ACC-DEV-001",
			wantErr:       true,
			errorContains: "Prototype/test ACC on production owner_ref",
			description:   "Should reject DEV prefixed ACC as owner_ref",
		},
		{
			name:          "sandbox account owner_ref rejected",
			ownerRef:      "ACC-SBOX-001",
			wantErr:       true,
			errorContains: "Prototype/test ACC on production owner_ref",
			description:   "Should reject SBOX prefixed ACC as owner_ref",
		},
		{
			name:          "mock account owner_ref rejected",
			ownerRef:      "ACC-MOCK-001",
			wantErr:       true,
			errorContains: "Prototype/test ACC on production owner_ref",
			description:   "Should reject MOCK prefixed ACC as owner_ref",
		},
		{
			name:          "valid production account owner_ref accepted",
			ownerRef:      "ACC-PROD-001",
			wantErr:       false,
			errorContains: "",
			description:   "Should accept regular ACC as valid owner_ref",
		},
		{
			name:          "non-account owner_ref rejected",
			ownerRef:      "PRI-LIVE-001",
			wantErr:       true,
			errorContains: "not a valid account ID",
			description:   "Should reject PRI prefixed IDs as they are not valid ACC account IDs",
		},
		{
			name:          "human string owner_ref rejected",
			ownerRef:      "human",
			wantErr:       true,
			errorContains: "not a valid account ID",
			description:   "Should reject 'human' as owner_ref",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			obj := map[string]any{
				objects.FieldKeyID:            "BLI-001",
				objects.FieldKeyKind:          "backlog_item",
				objects.FieldKeyTitle:         "Test Item",
				objects.FieldKeyOwnerRef:      tt.ownerRef,
				objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			}

			ctx := context.Background()
			result, err := goValidator.Validate(ctx, obj, "backlog_item", nil)
			if err != nil {
				t.Errorf("unexpected structural error: %v", err)
				return
			}

			hasProtoErr := false
			for _, e := range result.Errors {
				if strings.Contains(e.Message, "prototype/test account") || strings.Contains(e.Message, "not a valid account ID") {
					hasProtoErr = true
					break
				}
			}

			if hasProtoErr != tt.wantErr {
				t.Errorf("expected prototype error: %v, got: %v (errors: %v)", tt.wantErr, hasProtoErr, result.Errors)
			}
		})
	}
}

// TestValidateOwnerRefPrototypeRejectionEdgeCases tests edge cases for prototype rejection
func TestValidateOwnerRefPrototypeRejectionEdgeCases(t *testing.T) {
	t.Parallel()

	specsDir := filepath.Join("..", "..", paths.ProcessInternalObjectSpecsDir)
	lifecyclesDir := filepath.Join("..", "..", paths.ProcessInternalLifecyclesDir)

	specLoader := objects.NewSpecLoader(specsDir)
	// Set builder registry on spec loader to enable version-aware loading
	builderRegistry := builders.GetGlobalRegistry()
	adapter := builders.NewSpecLoaderAdapter(builderRegistry)
	specLoader.SetBuilderRegistry(adapter)

	goValidator := NewGoValidatorWithLoaders(specLoader, objects.NewLifecycleLoader(lifecyclesDir))

	tests := []struct {
		name        string
		ownerRef    string
		wantErr     bool
		description string
	}{
		{
			name:        "empty owner_ref is allowed",
			ownerRef:    "",
			wantErr:     false,
			description: "Empty owner_ref should be valid",
		},
		{
			name:        "nil owner_ref is allowed",
			ownerRef:    "", // Will test with nil separately
			wantErr:     false,
			description: "Missing owner_ref should be valid",
		},
		{
			name:        "PROTOTYPER prefix rejected",
			ownerRef:    "ACC-PROTOTYPER-001",
			wantErr:     true,
			description: "Should reject PROTOTYPER prefixed IDs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			obj := map[string]any{
				objects.FieldKeyID:            "BLI-001",
				objects.FieldKeyKind:          "backlog_item",
				objects.FieldKeyTitle:         "Test Item",
				objects.FieldKeyOwnerRef:      tt.ownerRef,
				objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			}

			if tt.name == "nil owner_ref is allowed" {
				delete(obj, "owner_ref")
			}

			ctx := context.Background()
			result, err := goValidator.Validate(ctx, obj, "backlog_item", nil)
			if err != nil {
				t.Errorf("unexpected structural error: %v", err)
				return
			}

			hasProtoErr := false
			for _, e := range result.Errors {
				if strings.Contains(e.Message, "prototype/test account") || strings.Contains(e.Message, "not a valid account ID") {
					hasProtoErr = true
					break
				}
			}

			if hasProtoErr != tt.wantErr {
				t.Errorf("unexpected prototype validation error status: got %v (errors: %v), want %v", hasProtoErr, result.Errors, tt.wantErr)
			}
		})
	}
}

// hasPrototypeRefError checks if any error is related to prototype ACC rejection
func hasPrototypeRefError(errors []ValidationError) bool {
	for _, e := range errors {
		if e.Rule == "prototype_restricted_acc" {
			return true
		}
	}
	return false
}

// TestValidatePrototypeAccountRef directly tests the core function
func TestValidatePrototypeAccountRef(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		refID     string
		wantValid bool
	}{
		{"prototype prefix TEST", "TEST-001", false},
		{"prototype prefix PVT", "PVT-001", false},
		{"prototype prefix DEV", "DEV-001", false},
		{"prototype prefix SBOX", "SBOX-001", false},
		{"prototype prefix MOCK", "MOCK-001", false},
		{"prototype prefix PROTOTYPER", "PROTOTYPER-001", false},
		{"valid prefix ACC", "ACC-PROD-001", true},
		{"valid prefix PRI", "PRI-LIVE-001", true},
		{"valid empty string", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := validatePrototypeAccountRef(tt.refID)
			if got != tt.wantValid {
				t.Errorf("validatePrototypeAccountRef(%q) = %v, want %v", tt.refID, got, tt.wantValid)
			}
		})
	}
}
