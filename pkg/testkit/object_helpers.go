package testkit

import (
	"context"
	"path/filepath"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/migration/parser"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// WriteTestObject parses YAML content into an object and persists it using FileObjectStorage.
// It ensures proper CAS index registration (avoiding "hand-CAS" materialization errors).
// Returns the expected file path of the written object.
func WriteTestObject(t testing.TB, fileStorage *storage.FileObjectStorage, content string) string {
	t.Helper()
	parsed, err := parser.NewYAMLParser().ParseBytes([]byte(content))
	if err != nil {
		t.Fatalf("Failed to parse test object YAML: %v", err)
	}
	obj := map[string]any{}
	for k, v := range parsed.Properties {
		obj[k] = v
	}
	if parsed.ID != "" {
		obj[objects.FieldKeyID] = parsed.ID
	}
	if parsed.Kind != "" {
		obj[objects.FieldKeyKind] = parsed.Kind
	}
	if parsed.Title != "" {
		obj[objects.FieldKeyTitle] = parsed.Title
	}
	if parsed.Status != "" {
		obj[objects.FieldKeyStatus] = parsed.Status
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{AccountID: pkgctx.SystemAccountID}

	if err := fileStorage.Create(ctx, secCtx, obj); err != nil {
		t.Fatalf("Failed to create test object: %v", err)
	}

	kind, _ := obj[objects.FieldKeyKind].(string)
	id, _ := obj[objects.FieldKeyID].(string)
	kindDir := filepath.Join(fileStorage.GetProjectRoot(), paths.ProcessDir, objects.GetDirectoryFromKind(kind))
	if objects.GetDirectoryFromKind(kind) == "" {
		kindDir = filepath.Join(fileStorage.GetProjectRoot(), paths.ProcessDir, kind)
	}
	return filepath.Join(kindDir, id+".yaml")
}

// WriteTestObjectStandalone creates a test object by writing it directly to disk
// and manually updating the CAS hash registry, avoiding the overhead of initializing
// a full FileObjectStorage (which requires object specs and cache warming).
func WriteTestObjectStandalone(t testing.TB, projectRoot, content string) string {
	t.Helper()
	obj, err := parser.NewYAMLParser().ParseBytes([]byte(content))
	if err != nil {
		t.Fatalf("Failed to parse test object YAML: %v", err)
	}

	kindDir := filepath.Join(projectRoot, paths.ProcessDir, obj.Kind)
	if err := fileutil.MkdirAll(kindDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create kind directory: %v", err)
	}

	filePath := filepath.Join(kindDir, obj.ID+".yaml")
	if err := fileutil.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test object: %v", err)
	}

	hash := storage.CalculateSHA256Hash([]byte(content))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	registry := storage.NewHashRegistry(ctx, obj.Kind, kindDir)
	registry.SetSkipShutdownCoordinatorCheck(true)
	defer registry.InitiateShutdown()

	// We need to load it first so we don't overwrite existing hashes
	if err := registry.Load(); err != nil {
		// ignore load errors if file doesn't exist
	}

	registry.SetHash(obj.ID, hash)

	// Save to file
	if err := registry.Save(); err != nil {
		t.Fatalf("Failed to save CAS registry: %v", err)
	}

	return filePath
}
