package qa

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestASTAuditor_AuditFile(t *testing.T) {
	t.Parallel()
	auditor := NewASTAuditor()

	testCases := []struct {
		name          string
		code          string
		expectedTypes []string
	}{
		{
			name: "Clean Code",
			code: `package main
import "context"
func clean(ctx context.Context) error {
	return nil
}`,
			expectedTypes: nil,
		},
		{
			name: "Swallowed Error",
			code: `package main
func bad() {
	_, _ = someFunc()
}
func someFunc() (int, error) { return 0, nil }`,
			expectedTypes: []string{"error_handling_debt"},
		},
		{
			name: "Fmt Print Anti Pattern",
			code: `package main
import "fmt"
func log() {
	fmt.Println("logging")
}`,
			expectedTypes: []string{"policy_violation"},
		},
		{
			name: "Fmt Fprintf os.Stderr Anti Pattern",
			code: `package main
import (
	"fmt"
)
func log() {
	fmt.Fprintf(os.Stderr, "error: %v", "bad")
}`,
			expectedTypes: []string{"policy_violation"},
		},
		{
			name: "Fmt Fprintf to buffer is allowed",
			code: `package main
import (
	"bytes"
	"fmt"
)
func log() {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "ok: %v", "good")
}`,
			expectedTypes: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tmpDir := t.TempDir()
			path := filepath.Join(tmpDir, "test.go")
			if err := fileutil.WriteStandardFile(path, []byte(tc.code)); err != nil {
				t.Fatalf("Failed to write test file: %v", err)
			}
			violations, err := auditor.AuditFile(path)
			if err != nil {
				t.Fatalf("AuditFile failed: %v", err)
			}

			if len(violations) != len(tc.expectedTypes) {
				t.Fatalf("Expected %d violations, got %d", len(tc.expectedTypes), len(violations))
			}

			for i, v := range violations {
				if v.Type != tc.expectedTypes[i] {
					t.Errorf("Expected violation type %q at index %d, got %q", tc.expectedTypes[i], i, v.Type)
				}
			}
		})
	}
}

func TestAuditStudioInbox(t *testing.T) {
	auditor := NewASTAuditor()
	violations, err := auditor.AuditFile("../../studio/inbox.go")
	if err != nil {
		t.Fatalf("AuditFile failed: %v", err)
	}
	for _, v := range violations {
		t.Logf("Violation in inbox.go: %s:%d:%d [%s] %s", v.Pos.Filename, v.Pos.Line, v.Pos.Column, v.Type, v.Message)
	}
	if len(violations) > 0 {
		t.Fatalf("Found %d violations in inbox.go", len(violations))
	}
}

func TestASTAuditor_StructuralDuplication(t *testing.T) {
	t.Parallel()
	auditor := NewASTAuditor()

	t.Run("Identical Function Body Flagged", func(t *testing.T) {
		code := `package sample

func stepAlpha() int {
	a := 1
	b := 2
	c := a + b
	return c
}

func stepBeta() int {
	a := 1
	b := 2
	c := a + b
	return c
}
`
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "dup_body.go")
		if err := fileutil.WriteStandardFile(path, []byte(code)); err != nil {
			t.Fatalf("Failed to write file: %v", err)
		}

		violations, err := auditor.AuditFile(path)
		if err != nil {
			t.Fatalf("AuditFile failed: %v", err)
		}

		found := false
		for _, v := range violations {
			if v.Type == ViolationTypeDuplication {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("Expected %s violation for identical function bodies, got: %v", ViolationTypeDuplication, violations)
		}
	})

	t.Run("Subsequence Statement Duplication Flagged", func(t *testing.T) {
		code := `package sample

func runWorkerA() {
	setup()
	stepOne()
	stepTwo()
	stepThree()
	stepFour()
	teardownA()
}

func runWorkerB() {
	initB()
	stepOne()
	stepTwo()
	stepThree()
	stepFour()
	finalizeB()
}

func setup() {}
func stepOne() {}
func stepTwo() {}
func stepThree() {}
func stepFour() {}
func teardownA() {}
func initB() {}
func finalizeB() {}
`
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "dup_seq.go")
		if err := fileutil.WriteStandardFile(path, []byte(code)); err != nil {
			t.Fatalf("Failed to write file: %v", err)
		}

		violations, err := auditor.AuditFile(path)
		if err != nil {
			t.Fatalf("AuditFile failed: %v", err)
		}

		found := false
		for _, v := range violations {
			if v.Type == ViolationTypeDuplication {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("Expected %s violation for duplicated statement sequence, got: %v", ViolationTypeDuplication, violations)
		}
	})

	t.Run("Distinct Functions Not Flagged", func(t *testing.T) {
		code := `package sample

func calculateArea(w, h int) int {
	return w * h
}

func calculateVolume(w, h, d int) int {
	area := calculateArea(w, h)
	return area * d
}
`
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "distinct.go")
		if err := fileutil.WriteStandardFile(path, []byte(code)); err != nil {
			t.Fatalf("Failed to write file: %v", err)
		}

		violations, err := auditor.AuditFile(path)
		if err != nil {
			t.Fatalf("AuditFile failed: %v", err)
		}

		for _, v := range violations {
			if v.Type == ViolationTypeDuplication {
				t.Fatalf("Unexpected duplication violation on distinct functions: %v", v)
			}
		}
	})
}

func TestAuditQASurfacesCleanliness(t *testing.T) {
	t.Parallel()
	auditor := NewASTAuditor()
	files := []string{
		"service.go",
		"gate.go",
		"helpers.go",
		"ast_audit.go",
		"ast_audit_duplication.go",
	}

	for _, file := range files {
		violations, err := auditor.AuditFile(file)
		if err != nil {
			t.Fatalf("AuditFile failed for %s: %v", file, err)
		}
		for _, v := range violations {
			if v.Type == ViolationTypeDuplication {
				t.Fatalf("Found unexpected duplication violation in %s: %s", file, v.Message)
			}
		}
	}
}

