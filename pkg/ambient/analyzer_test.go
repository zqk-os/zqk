package ambient

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestASTAnalyzer_Analyze(t *testing.T) {
	analyzer := NewASTAnalyzer()
	tmpDir := t.TempDir()

	t.Run("Ignore non-go files", func(t *testing.T) {
		path := filepath.Join(tmpDir, "README.md")
		_ = fileutil.WriteSecureFile(path, []byte("# Hello"))
		preds, err := analyzer.Analyze(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(preds) != 0 {
			t.Errorf("expected 0 predictions, got %d", len(preds))
		}
	})

	t.Run("Predict missing test file", func(t *testing.T) {
		path := filepath.Join(tmpDir, "calculator.go")
		code := `package calc
		func Add(a, b int) int { return a + b }`
		_ = fileutil.WriteSecureFile(path, []byte(code))

		preds, err := analyzer.Analyze(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(preds) != 1 {
			t.Fatalf("expected 1 prediction, got %d", len(preds))
		}
		if preds[0].Type != "missing_test" {
			t.Errorf("expected type 'missing_test', got %q", preds[0].Type)
		}
		if filepath.Base(preds[0].FilePath) != "calculator_test.go" {
			t.Errorf("expected 'calculator_test.go', got %q", filepath.Base(preds[0].FilePath))
		}
	})

	t.Run("No prediction if test file exists", func(t *testing.T) {
		path := filepath.Join(tmpDir, "calculator.go")
		testPath := filepath.Join(tmpDir, "calculator_test.go")

		// Create both files
		_ = fileutil.WriteSecureFile(path, []byte(`package calc; func Add(a, b int) int { return a + b }`))
		_ = fileutil.WriteSecureFile(testPath, []byte(`package calc; import "testing"`))

		preds, err := analyzer.Analyze(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(preds) != 0 {
			t.Errorf("expected 0 predictions (test already exists), got %d", len(preds))
		}
	})
}
