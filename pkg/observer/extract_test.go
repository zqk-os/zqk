package observer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestExtractFromDir_GoOnly(t *testing.T) {
	dir := t.TempDir()
	err := fileutil.WriteFile(filepath.Join(dir, "a.go"), []byte("package p\nfunc F() {}"), paths.FilePerm644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}
	fsys := os.DirFS(dir)
	ctx := context.Background()
	extractors := []Extractor{GoExtractor{}}
	result, err := ExtractFromDir(ctx, fsys, ".", extractors)
	if err != nil {
		t.Fatalf("ExtractFromDir: %v", err)
	}
	if len(result.Entities) < 1 {
		t.Errorf("expected at least 1 entity, got %d", len(result.Entities))
	}
}

// TestExtractFromDir_MultiFile verifies AST parsing infrastructure across multiple Go files (BLI-810).
func TestExtractFromDir_MultiFile(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"a.go": "package pkg\nfunc Foo() {}\ntype Bar struct{}\nfunc (b *Bar) M() {}",
		"b.go": "package pkg\nfunc Baz(x int) error { return nil }",
	}
	for name, content := range files {
		if err := fileutil.WriteFile(filepath.Join(dir, name), []byte(content), paths.FilePerm644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	fsys := os.DirFS(dir)
	ctx := context.Background()
	extractors := []Extractor{GoExtractor{}}
	result, err := ExtractFromDir(ctx, fsys, ".", extractors)
	if err != nil {
		t.Fatalf("ExtractFromDir: %v", err)
	}
	// Expect at least: Foo, Bar, M, Baz (4 entities)
	if len(result.Entities) < 4 {
		t.Errorf("expected at least 4 entities from two files, got %d", len(result.Entities))
	}
	names := make(map[string]bool)
	for _, e := range result.Entities {
		names[e.Name] = true
	}
	for _, want := range []string{"Foo", "Bar", "M", "Baz"} {
		if !names[want] {
			t.Errorf("expected entity name %q in result", want)
		}
	}
}
