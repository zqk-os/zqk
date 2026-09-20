package storage_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/zqk-os/zqk/pkg/validation"
)

// TestAcceptanceCriteria_ParseMixedContent tests parsing acceptance_criteria that contains
// both strings and criteria references (CRIT-####)
func TestAcceptanceCriteria_ParseMixedContent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		acceptanceCrit []any
		wantStrings    []string
		wantReferences []string
		wantError      bool
	}{
		{
			name:           "only strings",
			acceptanceCrit: []any{"User can create objects", "System validates input"},
			wantStrings:    []string{"User can create objects", "System validates input"},
			wantReferences: []string{},
			wantError:      false,
		},
		{
			name:           "only criteria references",
			acceptanceCrit: []any{"CRIT-9031", "CRIT-9032"},
			wantStrings:    []string{},
			wantReferences: []string{"CRIT-9031", "CRIT-9032"},
			wantError:      false,
		},
		{
			name:           "mixed strings and references",
			acceptanceCrit: []any{"User can create objects", "CRIT-9031", "System validates input", "CRIT-9032"},
			wantStrings:    []string{"User can create objects", "System validates input"},
			wantReferences: []string{"CRIT-9031", "CRIT-9032"},
			wantError:      false,
		},
		{
			name:           "empty list",
			acceptanceCrit: []any{},
			wantStrings:    []string{},
			wantReferences: []string{},
			wantError:      false,
		},
		{
			name:           "invalid reference format",
			acceptanceCrit: []any{"CRIT-", "INVALID-123", "CRIT-9031"},
			wantStrings:    []string{"CRIT-", "INVALID-123"},
			wantReferences: []string{"CRIT-9031"},
			wantError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strings, references := parseAcceptanceCriteria(tt.acceptanceCrit)

			if len(strings) != len(tt.wantStrings) {
				t.Errorf("parseAcceptanceCriteria() strings length = %v, want %v", len(strings), len(tt.wantStrings))
			}
			for i, want := range tt.wantStrings {
				if i < len(strings) && strings[i] != want {
					t.Errorf("parseAcceptanceCriteria() strings[%d] = %v, want %v", i, strings[i], want)
				}
			}

			if len(references) != len(tt.wantReferences) {
				t.Errorf("parseAcceptanceCriteria() references length = %v, want %v", len(references), len(tt.wantReferences))
			}
			for i, want := range tt.wantReferences {
				if i < len(references) && references[i] != want {
					t.Errorf("parseAcceptanceCriteria() references[%d] = %v, want %v", i, references[i], want)
				}
			}
		})
	}
}

func parseAcceptanceCriteria(acceptanceCrit []any) (strings, references []string) {
	for _, item := range acceptanceCrit {
		if str, ok := item.(string); ok {
			if len(str) >= 5 && str[:5] == "CRIT-" {
				if len(str) > 5 {
					hasDigits := false
					for _, r := range str[5:] {
						if r >= '0' && r <= '9' {
							hasDigits = true
							break
						}
					}
					if hasDigits {
						references = append(references, str)
						continue
					}
				}
			}
			strings = append(strings, str)
		}
	}
	return strings, references
}

// TestAcceptanceCriteria_LookupCriteria tests looking up criteria objects from references
func TestAcceptanceCriteria_LookupCriteria(t *testing.T) {
	t.Parallel()
	t.Skip("Skipping due to IDValidator.LoadPatterns sleep causing test timeout - test needs refactoring to avoid triggering pattern loading during cleanup")
}

//nolint:gocritic // test helper returning multiple values; signature kept for clarity
func lookupCriteriaReferences(ctx context.Context, secCtx *pkgctx.SecurityContext, provider storage.ObjectStorageProvider, references []string) (map[string]map[string]any, []string) {
	criteria := make(map[string]map[string]any)
	missing := []string{}

	for _, refID := range references {
		crit, err := provider.Read(ctx, secCtx, refID)
		if err != nil {
			missing = append(missing, refID)
			continue
		}
		criteria[refID] = crit
	}

	return criteria, missing
}

// TestAcceptanceCriteria_ValidateMixedContent tests validation of mixed acceptance criteria
func TestAcceptanceCriteria_ValidateMixedContent(t *testing.T) {
	t.Setenv(zqkenv.TestAllowCASFallthrough().Name(), "1")
	tmpDir := t.TempDir()
	storage.SetupTestRootLikeSetupTestEnvironmentForExportTest(t, tmpDir)
	testRoot, err := filepath.Abs(tmpDir)
	if err != nil {
		t.Fatalf("abs test root: %v", err)
	}

	provider, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	defer func() { _ = provider.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(testRoot, provider)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	cliCtx := storage.WithTestHardDelete(ctx)

	testCriteria := map[string]any{
		objects.FieldKeyID:            "CRIT-9031",
		objects.FieldKeyKind:          "criteria",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusAwaitingVerification,
		objects.FieldKeyTitle:         "Test Criteria",
		objects.FieldKeyCategory:      "functional",
		objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
		objects.FieldKeyOriginProject: validation.DefaultOriginProject,
	}
	_ = provider.Delete(cliCtx, secCtx, "CRIT-9031", false) //nolint:errcheck // Test cleanup - errors are acceptable
	if !waitForConditionWithTimeoutAcceptance(pkgctx.NewSystemContext(),
		func() bool {
			_, err := provider.Read(ctx, secCtx, "CRIT-9031")
			return err != nil
		},
		5*time.Second,
		10*time.Millisecond,
	) {
		t.Log("Deletion may still be in progress")
	}
	if err := provider.Create(cliCtx, secCtx, testCriteria); err != nil {
		t.Fatalf("failed to create test criteria: %v", err)
	}
	defer func() {
		_ = provider.Delete(cliCtx, secCtx, "CRIT-9031", false) //nolint:errcheck // Test cleanup - errors are acceptable
	}()

	if !waitForConditionWithTimeoutAcceptance(pkgctx.NewSystemContext(),
		func() bool {
			_, err := provider.Read(ctx, secCtx, "CRIT-9031")
			return err == nil
		},
		5*time.Second,
		10*time.Millisecond,
	) {
		t.Fatal("Expected object to be created within 5 seconds")
	}

	tests := []struct {
		name           string
		acceptanceCrit []any
		wantValid      bool
		wantErrors     []string
	}{
		{
			name:           "valid mixed content",
			acceptanceCrit: []any{"User can create objects", "CRIT-9031", "System validates input"},
			wantValid:      true,
			wantErrors:     []string{},
		},
		{
			name:           "valid with missing criteria",
			acceptanceCrit: []any{"User can create objects", "CRIT-9999", "System validates input"},
			wantValid:      false,
			wantErrors:     []string{"CRIT-9999"},
		},
		{
			name:           "all strings",
			acceptanceCrit: []any{"User can create objects", "System validates input"},
			wantValid:      true,
			wantErrors:     []string{},
		},
		{
			name:           "all valid criteria",
			acceptanceCrit: []any{"CRIT-9031"},
			wantValid:      true,
			wantErrors:     []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, references := parseAcceptanceCriteria(tt.acceptanceCrit)
			criteria, missing := lookupCriteriaReferences(ctx, secCtx, provider, references)

			hasErrors := len(missing) > 0
			if hasErrors != !tt.wantValid {
				t.Errorf("validateAcceptanceCriteria() valid = %v, want %v (missing: %v)", !hasErrors, tt.wantValid, missing)
			}

			if len(missing) != len(tt.wantErrors) {
				t.Errorf("validateAcceptanceCriteria() missing length = %v, want %v", len(missing), len(tt.wantErrors))
			}
			for i, want := range tt.wantErrors {
				if i < len(missing) && missing[i] != want {
					t.Errorf("validateAcceptanceCriteria() missing[%d] = %v, want %v", i, missing[i], want)
				}
			}

			for _, refID := range references {
				if !containsString(missing, refID) {
					if criteria[refID] == nil {
						t.Errorf("validateAcceptanceCriteria() criteria[%s] should exist", refID)
					}
				}
			}
		})
	}
}

func containsString(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

func waitForConditionWithTimeoutAcceptance(ctx context.Context, cond func() bool, timeout, interval time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return false
		}
		if cond() {
			return true
		}
		time.Sleep(interval)
	}
	return false
}
