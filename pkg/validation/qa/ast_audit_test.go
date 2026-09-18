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
