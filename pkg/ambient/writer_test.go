package ambient

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestArtifactWriter_MissingTest(t *testing.T) {
	tempDir, err := fileutil.MkdirTemp("", "artifactwriter_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tempDir)

	hub := NewEventHub()
	_ = NewArtifactWriter(hub)

	testFilePath := filepath.Join(tempDir, "calc", "math_test.go")

	event := Event{
		Type: EventTypeFilesystem,
		Payload: map[string]any{
			objects.FieldKeySource: "ast_analyzer",
			objects.FieldKeyPredictions: []Prediction{
				{
					FilePath:    testFilePath,
					Type:        "missing_test",
					Description: "Predicted need for test file",
				},
			},
		},
		Timestamp: time.Now(),
	}

	ctx := context.Background()
	err = hub.Publish(ctx, event)
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Verify the file was created
	content, err := fileutil.ReadFile(testFilePath)
	if err != nil {
		t.Fatalf("Failed to read expected test file: %v", err)
	}

	expectedContent := "package calc\n\nimport \"testing\"\n"
	if string(content) != expectedContent {
		t.Errorf("Expected content %q, got %q", expectedContent, string(content))
	}
}

func TestArtifactWriter_IgnoresOtherSources(t *testing.T) {
	tempDir, err := fileutil.MkdirTemp("", "artifactwriter_test2")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tempDir)

	hub := NewEventHub()
	_ = NewArtifactWriter(hub)

	testFilePath := filepath.Join(tempDir, "calc", "math_test.go")

	event := Event{
		Type: EventTypeFilesystem,
		Payload: map[string]any{
			objects.FieldKeySource: "other_source",
			objects.FieldKeyPredictions: []Prediction{
				{
					FilePath:    testFilePath,
					Type:        "missing_test",
					Description: "Predicted need for test file",
				},
			},
		},
		Timestamp: time.Now(),
	}

	ctx := context.Background()
	err = hub.Publish(ctx, event)
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Verify the file was NOT created
	if _, err := fileutil.Stat(testFilePath); !fileutil.IsNotExist(err) {
		t.Fatalf("Expected file to not exist, but it does (or stat returned an error: %v)", err)
	}
}

func TestArtifactWriter_IgnoresOtherTypes(t *testing.T) {
	tempDir, err := fileutil.MkdirTemp("", "artifactwriter_test3")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tempDir)

	hub := NewEventHub()
	_ = NewArtifactWriter(hub)

	testFilePath := filepath.Join(tempDir, "calc", "math_test.go")

	event := Event{
		Type: EventTypeFilesystem,
		Payload: map[string]any{
			objects.FieldKeySource: "ast_analyzer",
			objects.FieldKeyPredictions: []Prediction{
				{
					FilePath:    testFilePath,
					Type:        "some_other_type",
					Description: "Predicted need for something else",
				},
			},
		},
		Timestamp: time.Now(),
	}

	ctx := context.Background()
	err = hub.Publish(ctx, event)
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	if _, err := fileutil.Stat(testFilePath); !fileutil.IsNotExist(err) {
		t.Fatalf("Expected file to not exist, but it does")
	}
}

func TestArtifactWriter_DoesNotOverwrite(t *testing.T) {
	tempDir, err := fileutil.MkdirTemp("", "artifactwriter_test4")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tempDir)

	hub := NewEventHub()
	_ = NewArtifactWriter(hub)

	calcDir := filepath.Join(tempDir, "calc")
	if err := fileutil.EnsureDir(calcDir); err != nil {
		t.Fatalf("failed to create calc dir: %v", err)
	}

	testFilePath := filepath.Join(calcDir, "math_test.go")
	originalContent := []byte("package calc\n// custom test file\n")
	if err := fileutil.WriteSecureFile(testFilePath, originalContent); err != nil {
		t.Fatalf("failed to write original file: %v", err)
	}

	event := Event{
		Type: EventTypeFilesystem,
		Payload: map[string]any{
			objects.FieldKeySource: "ast_analyzer",
			objects.FieldKeyPredictions: []Prediction{
				{
					FilePath:    testFilePath,
					Type:        "missing_test",
					Description: "Predicted need for test file",
				},
			},
		},
		Timestamp: time.Now(),
	}

	ctx := context.Background()
	err = hub.Publish(ctx, event)
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Verify the file was NOT overwritten
	content, err := fileutil.ReadFile(testFilePath)
	if err != nil {
		t.Fatalf("Failed to read expected test file: %v", err)
	}

	if !bytes.Equal(content, originalContent) {
		t.Errorf("Expected content %q, got %q", string(originalContent), string(content))
	}
}

func TestArtifactWriter_RootPathPackageName(t *testing.T) {
	tempDir, err := fileutil.MkdirTemp("", "artifactwriter_test5")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tempDir)

	hub := NewEventHub()
	_ = NewArtifactWriter(hub)

	// A file directly in the temp directory has the package name based on tempDir base name,
	// but to test the "." or "/" logic, let's use a filepath that resolves to a dir like "."

	testFilePath := "test_in_root_test.go" // relative path, dir is "."

	event := Event{
		Type: EventTypeFilesystem,
		Payload: map[string]any{
			objects.FieldKeySource: "ast_analyzer",
			objects.FieldKeyPredictions: []Prediction{
				{
					FilePath:    testFilePath,
					Type:        "missing_test",
					Description: "Predicted need for test file",
				},
			},
		},
		Timestamp: time.Now(),
	}

	ctx := context.Background()
	err = hub.Publish(ctx, event)
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}
	defer fileutil.Remove(testFilePath) // clean up

	content, err := fileutil.ReadFile(testFilePath)
	if err != nil {
		t.Fatalf("Failed to read expected test file: %v", err)
	}

	expectedContent := "package main\n\nimport \"testing\"\n"
	if string(content) != expectedContent {
		t.Errorf("Expected content %q, got %q", expectedContent, string(content))
	}
}

func TestArtifactWriter_InvalidPayload(t *testing.T) {
	hub := NewEventHub()
	_ = NewArtifactWriter(hub)

	ctx := context.Background()

	// 1. Not a map
	err := hub.Publish(ctx, Event{
		Type:    EventTypeFilesystem,
		Payload: "invalid payload string",
	})
	if err != nil {
		t.Errorf("Publish failed on invalid payload: %v", err)
	}

	// 2. Map without source
	err = hub.Publish(ctx, Event{
		Type: EventTypeFilesystem,
		Payload: map[string]any{
			objects.FieldKeyPredictions: []Prediction{},
		},
	})
	if err != nil {
		t.Errorf("Publish failed on map without source: %v", err)
	}

	// 3. Map with wrong source type
	err = hub.Publish(ctx, Event{
		Type: EventTypeFilesystem,
		Payload: map[string]any{
			objects.FieldKeySource: 123,
		},
	})
	if err != nil {
		t.Errorf("Publish failed on map with wrong source type: %v", err)
	}

	// 4. Map with correct source but no predictions
	err = hub.Publish(ctx, Event{
		Type: EventTypeFilesystem,
		Payload: map[string]any{
			objects.FieldKeySource: "ast_analyzer",
		},
	})
	if err != nil {
		t.Errorf("Publish failed on map with missing predictions: %v", err)
	}

	// 5. Map with wrong predictions type
	err = hub.Publish(ctx, Event{
		Type: EventTypeFilesystem,
		Payload: map[string]any{
			objects.FieldKeySource:      "ast_analyzer",
			objects.FieldKeyPredictions: "not a slice",
		},
	})
	if err != nil {
		t.Errorf("Publish failed on map with wrong predictions type: %v", err)
	}
}
