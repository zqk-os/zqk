package observer

import (
	"context"
	"testing"
)

func TestGoExtractor_ExtractFile(t *testing.T) {
	ctx := context.Background()
	ex := GoExtractor{}
	content := []byte(`
package pkg

func Foo(x int) error { return nil }

type Bar struct { X int }

func (b *Bar) Method() string { return "" }
`)
	entities, err := ex.ExtractFile(ctx, "file.go", content)
	if err != nil {
		t.Fatalf("ExtractFile: %v", err)
	}
	if len(entities) < 3 {
		t.Errorf("expected at least 3 entities (func, type, method), got %d", len(entities))
	}
	var seenFunc, seenType, seenMethod bool
	for _, e := range entities {
		if e.Language != "go" {
			t.Errorf("entity language = %q, want go", e.Language)
		}
		switch e.Kind {
		case "function":
			if e.Name == "Foo" {
				seenFunc = true
			}
		case "method":
			if e.Name == "Method" {
				seenMethod = true
			}
		case "type", "struct":
			if e.Name == "Bar" {
				seenType = true
			}
		}
	}
	if !seenFunc {
		t.Error("expected function Foo")
	}
	if !seenType {
		t.Error("expected type/struct Bar")
	}
	if !seenMethod {
		t.Error("expected method Method")
	}
}

func TestGoExtractor_ExtractFile_InvalidGo(t *testing.T) {
	ctx := context.Background()
	ex := GoExtractor{}
	content := []byte("package pkg\nfunc broken (")
	_, err := ex.ExtractFile(ctx, "bad.go", content)
	if err == nil {
		t.Error("expected parse error for invalid Go")
	}
}

func TestGoExtractor_LanguageAndSuffix(t *testing.T) {
	ex := GoExtractor{}
	if ex.Language() != "go" {
		t.Errorf("Language() = %q, want go", ex.Language())
	}
	if ex.FileSuffix() != ".go" {
		t.Errorf("FileSuffix() = %q, want .go", ex.FileSuffix())
	}
}
