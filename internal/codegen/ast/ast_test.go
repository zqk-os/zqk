package ast

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestASTInspector_ParseSource(t *testing.T) {
	t.Parallel()

	inspector := NewASTInspector()
	if inspector == nil || inspector.Fset == nil {
		t.Fatal("expected non-nil inspector with initialized FileSet")
	}

	// Valid Go source
	validSrc := []byte(`package testpkg
type MyStruct struct {
	Field1 string
}
`)
	node, err := inspector.ParseSource("valid.go", validSrc)
	if err != nil {
		t.Fatalf("unexpected error parsing valid source: %v", err)
	}
	if node == nil {
		t.Fatal("expected non-nil AST file node")
	}

	// Invalid Go source (syntax error)
	invalidSrc := []byte(`package testpkg
type MyStruct struct {
	Field1 string invalid_syntax!!!
`)
	_, err = inspector.ParseSource("invalid.go", invalidSrc)
	if err == nil {
		t.Error("expected error for syntax error in source, got nil")
	}
}

func TestASTInspector_ParseFile(t *testing.T) {
	t.Parallel()

	inspector := NewASTInspector()
	tmpDir := t.TempDir()

	// Valid file on disk
	filePath := filepath.Join(tmpDir, "sample.go")
	validSrc := []byte(`package samplefile
type FileStruct struct {
	Name string
}
`)
	if err := fileutil.WriteFile(filePath, validSrc, 0600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	node, err := inspector.ParseFile(filePath)
	if err != nil {
		t.Fatalf("unexpected error parsing valid file: %v", err)
	}
	if node == nil {
		t.Fatal("expected non-nil node from ParseFile")
	}

	// Non-existent file
	_, err = inspector.ParseFile(filepath.Join(tmpDir, "non_existent.go"))
	if err == nil {
		t.Error("expected error for non-existent file path, got nil")
	}
}

func TestASTInspector_FindStructNames(t *testing.T) {
	t.Parallel()

	inspector := NewASTInspector()

	src := []byte(`package models

type First struct {
	A int
}

type MyInterface interface {
	DoSomething()
}

type Second struct {
	B string
}

type ScalarType int

type Third struct {
	C bool
}
`)
	node, err := inspector.ParseSource("models.go", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	structs := inspector.FindStructNames(node)
	expected := []string{"First", "Second", "Third"}

	if len(structs) != len(expected) {
		t.Fatalf("expected %d structs, got %d: %v", len(expected), len(structs), structs)
	}
	for i, name := range expected {
		if structs[i] != name {
			t.Errorf("struct[%d] expected %q, got %q", i, name, structs[i])
		}
	}

	// Empty file has 0 structs
	emptyNode, err := inspector.ParseSource("empty.go", []byte(`package empty`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	emptyStructs := inspector.FindStructNames(emptyNode)
	if len(emptyStructs) != 0 {
		t.Errorf("expected 0 structs in empty package, got: %v", emptyStructs)
	}
}

func TestASTInspector_HasEmbeddedType(t *testing.T) {
	t.Parallel()

	inspector := NewASTInspector()

	src := []byte(`package embedtest

import "github.com/zqk-os/zqk/pkg/dna"

type EmbeddedBoth struct {
	dna.BaseObject
	Auditable
	Title string
}

type EmptyStruct struct {}

type NonEmbedded struct {
	BaseObject string
	other int
}
`)
	node, err := inspector.ParseSource("embed.go", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Selector embedding (dna.BaseObject)
	if !inspector.HasEmbeddedType(node, "EmbeddedBoth", "BaseObject") {
		t.Error("expected BaseObject to be recognized via selector embedding")
	}

	// Ident embedding (Auditable)
	if !inspector.HasEmbeddedType(node, "EmbeddedBoth", "Auditable") {
		t.Error("expected Auditable to be recognized via direct ident embedding")
	}

	// Non-embedded field
	if inspector.HasEmbeddedType(node, "EmbeddedBoth", "Title") {
		t.Error("Title is a regular field, not an embedded type")
	}

	// Missing type
	if inspector.HasEmbeddedType(node, "EmbeddedBoth", "NonExistent") {
		t.Error("NonExistent is not embedded")
	}

	// Empty struct
	if inspector.HasEmbeddedType(node, "EmptyStruct", "BaseObject") {
		t.Error("EmptyStruct has no embedded types")
	}

	// Non-embedded struct with same field name as type
	if inspector.HasEmbeddedType(node, "NonEmbedded", "BaseObject") {
		t.Error("NonEmbedded has named field BaseObject, should not be considered embedded")
	}

	// Struct name that doesn't exist
	if inspector.HasEmbeddedType(node, "NoSuchStruct", "BaseObject") {
		t.Error("NoSuchStruct does not exist")
	}
}
