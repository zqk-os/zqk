package validation_test

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
	"github.com/zqk-os/zqk/pkg/validation/qa"
)

// TestConstMagicEradication_StaticFloor verifies CRIT-CEF-CONSTMAGIC-ELIMINATION:
// Zero occurrences of ConstMagic pseudo-constants in pkg/validation source files.
func TestConstMagicEradication_StaticFloor(t *testing.T) {
	validationDir := filepath.Dir(".")
	pattern := regexp.MustCompile(`ConstMagic[0-9a-fA-F]{8}`)
	var offendingFiles []string

	err := filepath.Walk(validationDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		base := filepath.Base(path)
		if base == "constmagic_eradication_test.go" || base == "ast_audit_test.go" {
			return nil
		}

		content, readErr := fileutil.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if pattern.Match(content) {
			offendingFiles = append(offendingFiles, path)
		}
		return nil
	})

	if err != nil {
		t.Fatalf("failed to scan pkg/validation: %v", err)
	}

	if len(offendingFiles) > 0 {
		t.Errorf("CRIT-CEF-CONSTMAGIC-ELIMINATION failed: found ConstMagic in files: %v", offendingFiles)
	}
}

// TestConstMagicEradication_OperationalProof verifies CRIT-CEF-CONSTMAGIC-SEMANTIC-INLINING:
// Core validator executes with inlined semantic string literals and accurately produces diagnostics.
func TestConstMagicEradication_OperationalProof(t *testing.T) {
	v := validation.NewGoValidator()
	ctx := context.Background()

	// Valid minimal object
	validObj := map[string]any{
		objects.FieldKeyID:          "GOAL-100",
		objects.FieldKeyKind:        "goal",
		objects.FieldKeyStatus:      "originated",
		objects.FieldKeyTitle:       "Operational Proof Goal",
		objects.FieldKeyDescription: "This is a valid test goal description with more than twenty characters.",
		objects.FieldKeyCreatedAt:   "2026-03-14T07:30:16Z",
		objects.FieldKeyUpdatedAt:   "2026-03-14T07:30:16Z",
		objects.FieldKeyCreatedBy:   "ACC-SYSTEM",
		objects.FieldKeyUpdatedBy:   "ACC-SYSTEM",
	}

	res, err := v.Validate(ctx, validObj, "goal", validation.DefaultValidationOptions())
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if len(res.Errors) > 0 {
		t.Errorf("expected 0 errors for valid object, got %d: %v", len(res.Errors), res.Errors)
	}

	// Invalid object missing required fields
	invalidObj := map[string]any{
		objects.FieldKeyID: "INVALID_ID",
	}
	resInvalid, err := v.Validate(ctx, invalidObj, "goal", validation.DefaultValidationOptions())
	if err != nil {
		t.Fatalf("unexpected error validating invalid object: %v", err)
	}
	if len(resInvalid.Errors) == 0 {
		t.Errorf("expected validation errors for invalid object, got 0")
	}
}

// TestConstMagicEradication_NegativeBoundary verifies CRIT-CEF-CONSTMAGIC-LINT-NEGATIVE-BOUNDARY:
// ASTAuditor rejects any file introducing synthetic ConstMagic constants or identifiers.
func TestConstMagicEradication_NegativeBoundary(t *testing.T) {
	auditor := qa.NewASTAuditor()
	tmpDir := t.TempDir()

	badConstFile := filepath.Join(tmpDir, "bad_const.go")
	badConstContent := `package tmp
const ConstMagic9999ffff = "synthetic constant"
func dummy() {}
`
	if err := fileutil.WriteFile(badConstFile, []byte(badConstContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	violations, err := auditor.AuditFile(badConstFile)
	if err != nil {
		t.Fatalf("AuditFile failed: %v", err)
	}
	if len(violations) == 0 {
		t.Fatalf("expected violation for ConstMagic declaration, got none")
	}

	foundViolation := false
	for _, v := range violations {
		if strings.Contains(v.Message, "ConstMagic") {
			foundViolation = true
			break
		}
	}
	if !foundViolation {
		t.Errorf("expected ConstMagic violation message, got: %v", violations)
	}
}
