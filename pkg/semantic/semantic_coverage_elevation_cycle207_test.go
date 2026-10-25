// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package semantic

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// cycle207StorageMock is a thread-safe mock storage provider for cycle 207 tests.
type cycle207StorageMock struct {
	storage.NoopObjectStorage
	mu          sync.Mutex
	objects     []map[string]any
	errOnList   error
	errOnUpdate error
}

func newCycle207StorageMock() *cycle207StorageMock {
	return &cycle207StorageMock{
		objects: make([]map[string]any, 0),
	}
}

func (m *cycle207StorageMock) addObject(obj map[string]any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects = append(m.objects, obj)
}

func (m *cycle207StorageMock) List(_ context.Context, _ *pkgctx.SecurityContext, _ *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.errOnList != nil {
		return nil, m.errOnList
	}

	result := make([]map[string]any, 0)
	for _, obj := range m.objects {
		if obj[objects.FieldKeyKind] != filter.Kind {
			continue
		}
		match := true
		for k, v := range filter.Filters {
			if obj[k] != v {
				match = false
				break
			}
		}
		if match {
			result = append(result, obj)
		}
	}
	return &storage.QueryResult{Objects: result}, nil
}

func (m *cycle207StorageMock) Update(_ context.Context, _ *pkgctx.SecurityContext, id string, updates map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.errOnUpdate != nil {
		return m.errOnUpdate
	}

	for i, obj := range m.objects {
		if obj[objects.FieldKeyID] == id {
			for k, v := range updates {
				m.objects[i][k] = v
			}
			return nil
		}
	}
	return storage.ErrObjectNotFound
}

func TestOntologyImporter_Elevation(t *testing.T) {
	t.Parallel()
	importer := NewOntologyImporter()

	t.Run("Import_FromBuffer_OpenAPI_WithComponents", func(t *testing.T) {
		openAPISpec := []byte(`
openapi: "3.0.0"
info:
  title: "Test System"
  version: "1.0.0"
components:
  schemas:
    Device:
      type: object
      properties:
        id:
          type: string
`)
		res, err := importer.Import("spec.yaml", "openapi", openAPISpec)
		if err != nil {
			t.Fatalf("unexpected import error: %v", err)
		}
		if res == nil {
			t.Fatal("expected non-nil OntologyImport")
		}
		if res.Kind != "ontology_import" {
			t.Fatalf("expected kind ontology_import, got %s", res.Kind)
		}
		if res.Status != objects.ObjectStatusTranslated {
			t.Fatalf("expected status %s, got %s", objects.ObjectStatusTranslated, res.Status)
		}
		if len(res.TranslatedObjects) == 0 {
			t.Fatal("expected translated objects to contain extracted entities and specs")
		}
		if res.ImportedAt.IsZero() {
			t.Fatal("expected non-zero imported_at timestamp")
		}
	})

	t.Run("Import_FromFilePath_Success", func(t *testing.T) {
		tmpDir := t.TempDir()
		specPath := filepath.Join(tmpDir, "service_schema.json")
		content := []byte(`{
  "$id": "https://example.com/widget.schema.json",
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "Widget",
  "type": "object"
}`)
		if err := fileutil.WriteFile(specPath, content, paths.FilePerm644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		res, err := importer.Import(specPath, "json_schema", nil)
		if err != nil {
			t.Fatalf("unexpected import error: %v", err)
		}
		if res.SourceFile != specPath {
			t.Fatalf("expected source file %s, got %s", specPath, res.SourceFile)
		}
	})

	t.Run("Import_FromFilePath_FileNotFound", func(t *testing.T) {
		nonExistent := filepath.Join(t.TempDir(), "does_not_exist.ttl")
		_, err := importer.Import(nonExistent, "turtle", nil)
		if err == nil {
			t.Fatal("expected error reading nonexistent file, got nil")
		}
	})

	t.Run("Import_UnsupportedFormat", func(t *testing.T) {
		_, err := importer.Import("test.unknown", "unknown_format_xyz", []byte("data"))
		if err == nil {
			t.Fatal("expected error for unsupported format, got nil")
		}
	})
}

type mockCustomTranslator struct{}

func (m *mockCustomTranslator) Translate(data []byte) ([]map[string]any, error) {
	if len(data) == 0 {
		return nil, errors.New("empty data")
	}
	return []map[string]any{{"key": "value"}}, nil
}

func TestOntologyTranslator_Interface(t *testing.T) {
	t.Parallel()
	var translator OntologyTranslator = &mockCustomTranslator{}
	results, err := translator.Translate([]byte("sample"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	_, errEmpty := translator.Translate(nil)
	if errEmpty == nil {
		t.Fatal("expected error on empty data")
	}
}

func TestInferenceEngine_Elevation(t *testing.T) {
	t.Parallel()

	t.Run("Infer_Level0_Naive_Fallback", func(t *testing.T) {
		engine := NewInferenceEngine(nil)
		assessment := &MaturityAssessment{Level: 0, LevelName: "Naive"}
		inferences := engine.Infer(context.Background(), assessment)

		if len(inferences) != 1 {
			t.Fatalf("expected 1 fallback inference for level 0, got %d", len(inferences))
		}
		title, ok := inferences[0][objects.FieldKeyTitle].(string)
		if !ok || title != "Standardize Core Objects" {
			t.Fatalf("unexpected title: %v", inferences[0][objects.FieldKeyTitle])
		}
		tier, ok := inferences[0][objects.FieldKeyPriorityTier].(string)
		if !ok || tier != "P1" {
			t.Fatalf("unexpected priority tier: %v", inferences[0][objects.FieldKeyPriorityTier])
		}
	})

	t.Run("Infer_DynamicHeuristics_FromStore", func(t *testing.T) {
		store := newCycle207StorageMock()
		store.addObject(map[string]any{
			objects.FieldKeyKind:                objects.KindInferenceHeuristic,
			objects.FieldKeyTargetMaturityLevel: 2,
			objects.FieldKeyProposedTitle:       "Adopt Formal JSON Schemas",
			objects.FieldKeyProposedDescription: "Schema validation elevates maturity to Practicing",
			objects.FieldKeyProposedPriority:    "P2",
		})
		store.addObject(map[string]any{
			objects.FieldKeyKind:                objects.KindInferenceHeuristic,
			objects.FieldKeyTargetMaturityLevel: 2,
			objects.FieldKeyProposedTitle:       "", // Empty title should be skipped
			objects.FieldKeyProposedDescription: "Should not be added",
			objects.FieldKeyProposedPriority:    "P3",
		})

		engine := NewInferenceEngine(store)
		assessment := &MaturityAssessment{Level: 2, LevelName: "Practicing"}
		inferences := engine.Infer(context.Background(), assessment)

		if len(inferences) != 1 {
			t.Fatalf("expected 1 inference, got %d", len(inferences))
		}
		if inferences[0][objects.FieldKeyTitle] != "Adopt Formal JSON Schemas" {
			t.Fatalf("unexpected title: %v", inferences[0][objects.FieldKeyTitle])
		}
	})

	t.Run("Infer_StoreError_GracefulFallback", func(t *testing.T) {
		store := newCycle207StorageMock()
		store.errOnList = errors.New("database connection refused")

		engine := NewInferenceEngine(store)
		assessment := &MaturityAssessment{Level: 1, LevelName: "Aware"}
		inferences := engine.Infer(context.Background(), assessment)

		if len(inferences) != 0 {
			t.Fatalf("expected 0 inferences on store error for level 1, got %d", len(inferences))
		}
	})

	t.Run("InferFromConvergence_MixedResults", func(t *testing.T) {
		engine := NewInferenceEngine(nil)
		results := []ConvergenceResult{
			{SessionID: "CVS-101", Title: "Entity Ingestion", TrendingAway: true},
			{SessionID: "CVS-102", Title: "Schema Sync", TrendingAway: false},
			{SessionID: "CVS-103", Title: "Catalog Alignment", TrendingAway: true},
		}

		inferences := engine.InferFromConvergence(results)
		if len(inferences) != 2 {
			t.Fatalf("expected 2 inferences from trending away results, got %d", len(inferences))
		}

		firstTitle, _ := inferences[0][objects.FieldKeyTitle].(string)
		if firstTitle != "Remediate Entity Ingestion" {
			t.Fatalf("unexpected first inference title: %s", firstTitle)
		}
		secondTitle, _ := inferences[1][objects.FieldKeyTitle].(string)
		if secondTitle != "Remediate Catalog Alignment" {
			t.Fatalf("unexpected second inference title: %s", secondTitle)
		}
	})
}

type errScannerMock struct{}

func (s *errScannerMock) Scan(_ string) ([]Indicator, error) {
	return nil, errors.New("simulated scan error")
}

type staticScannerMock struct {
	indicators []Indicator
}

func (s *staticScannerMock) Scan(_ string) ([]Indicator, error) {
	return s.indicators, nil
}

func TestMaturityAssessor_Elevation(t *testing.T) {
	t.Parallel()

	t.Run("Assess_ScannerErrorTolerance", func(t *testing.T) {
		scanners := []MaturityScanner{
			&errScannerMock{},
			&staticScannerMock{
				indicators: []Indicator{
					{Type: "formal_schemas", Found: true, Sophistication: "intermediate"},
				},
			},
		}
		assessor := NewMaturityAssessor(scanners)
		res, err := assessor.Assess("/virtual/root")
		if err != nil {
			t.Fatalf("unexpected assessment error: %v", err)
		}
		if res.Level != 2 || res.LevelName != "Practicing" {
			t.Fatalf("expected Level 2 Practicing despite scanner error, got Level %d %s", res.Level, res.LevelName)
		}
	})

	t.Run("Assess_AllLevels_And_Recommendations", func(t *testing.T) {
		testCases := []struct {
			indicators    []Indicator
			expectedLevel int
			expectedName  string
		}{
			{
				indicators:    nil,
				expectedLevel: 0,
				expectedName:  "Naive",
			},
			{
				indicators:    []Indicator{{Type: "structured_data", Found: true}},
				expectedLevel: 1,
				expectedName:  "Aware",
			},
			{
				indicators:    []Indicator{{Type: "formal_schemas", Found: true}},
				expectedLevel: 2,
				expectedName:  "Practicing",
			},
			{
				indicators:    []Indicator{{Type: "ontologies", Found: true}},
				expectedLevel: 3,
				expectedName:  "Advanced",
			},
			{
				indicators:    []Indicator{{Type: "semantic_repositories", Found: true}},
				expectedLevel: 4,
				expectedName:  "Expert",
			},
			{
				// Multiple present: Expert should take precedence
				indicators: []Indicator{
					{Type: "structured_data", Found: true},
					{Type: "formal_schemas", Found: true},
					{Type: "ontologies", Found: true},
					{Type: "semantic_repositories", Found: true},
				},
				expectedLevel: 4,
				expectedName:  "Expert",
			},
		}

		for _, tc := range testCases {
			lvl, name := determineMaturityLevel(tc.indicators)
			if lvl != tc.expectedLevel || name != tc.expectedName {
				t.Fatalf("expected level %d %s, got %d %s", tc.expectedLevel, tc.expectedName, lvl, name)
			}
			recs := generateRecommendations(lvl, tc.indicators)
			if len(recs) == 0 {
				t.Fatalf("expected recommendations for level %d, got 0", lvl)
			}
			for _, rec := range recs {
				if rec.Recommendation == "" || rec.Priority == "" || rec.Effort == "" || rec.Benefit == "" {
					t.Fatalf("incomplete recommendation struct: %+v", rec)
				}
			}
		}

		// Edge case: unmapped level returns empty recommendations
		emptyRecs := generateRecommendations(999, nil)
		if len(emptyRecs) != 0 {
			t.Fatalf("expected 0 recommendations for invalid level 999, got %d", len(emptyRecs))
		}
	})
}

func TestSemanticReconciler_CheckDrift_Elevation(t *testing.T) {
	t.Parallel()
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := context.Background()

	t.Run("StoreListError", func(t *testing.T) {
		store := newCycle207StorageMock()
		store.errOnList = errors.New("list failure")

		reconciler := NewSemanticReconciler(store)
		_, err := reconciler.CheckDrift(ctx, secCtx)
		if err == nil {
			t.Fatal("expected error on store list failure, got nil")
		}
	})

	t.Run("EmptySourceFileSkipped", func(t *testing.T) {
		store := newCycle207StorageMock()
		store.addObject(map[string]any{
			objects.FieldKeyID:         "TRACK-001",
			objects.FieldKeyKind:       objects.KindImportTracking,
			objects.FieldKeySourceFile: "",
		})

		reconciler := NewSemanticReconciler(store)
		results, err := reconciler.CheckDrift(ctx, secCtx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 0 {
			t.Fatalf("expected empty source_file to be skipped, got %d results", len(results))
		}
	})

	t.Run("StatFailure_RecordError", func(t *testing.T) {
		store := newCycle207StorageMock()
		missingPath := filepath.Join(t.TempDir(), "nonexistent_file.yaml")
		store.addObject(map[string]any{
			objects.FieldKeyID:         "TRACK-002",
			objects.FieldKeyKind:       objects.KindImportTracking,
			objects.FieldKeySourceFile: missingPath,
		})

		reconciler := NewSemanticReconciler(store)
		results, err := reconciler.CheckDrift(ctx, secCtx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if results[0].Error == nil {
			t.Fatal("expected result.Error on missing file stat")
		}
	})

	t.Run("MTimeMatch_SkipsHashCalculation", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "unchanged.yaml")
		if err := fileutil.WriteFile(filePath, []byte("key: original\n"), paths.FilePerm644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
		info, err := fileutil.Stat(filePath)
		if err != nil {
			t.Fatalf("failed to stat file: %v", err)
		}
		mtimeStr := zqktime.FormatRFC3339UTC(info.ModTime())

		store := newCycle207StorageMock()
		store.addObject(map[string]any{
			objects.FieldKeyID:          "TRACK-003",
			objects.FieldKeyKind:        objects.KindImportTracking,
			objects.FieldKeySourceFile:  filePath,
			objects.FieldKeySourceMtime: mtimeStr,
			objects.FieldKeySourceHash:  "initialhash",
		})

		reconciler := NewSemanticReconciler(store)
		results, err := reconciler.CheckDrift(ctx, secCtx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if results[0].Drifted {
			t.Fatal("expected drifted=false when mtime matches")
		}
		if results[0].Error != nil {
			t.Fatalf("unexpected result error: %v", results[0].Error)
		}
	})

	t.Run("DriftDetected_StoreUpdated", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "drifted.yaml")
		if err := fileutil.WriteFile(filePath, []byte("key: modified_content\n"), paths.FilePerm644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}

		store := newCycle207StorageMock()
		store.addObject(map[string]any{
			objects.FieldKeyID:          "TRACK-004",
			objects.FieldKeyKind:        objects.KindImportTracking,
			objects.FieldKeySourceFile:  filePath,
			objects.FieldKeySourceMtime: "2020-01-01T00:00:00Z",
			objects.FieldKeySourceHash:  "stale_hash_value",
		})

		reconciler := NewSemanticReconciler(store)
		results, err := reconciler.CheckDrift(ctx, secCtx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if !results[0].Drifted {
			t.Fatal("expected drifted=true when hash differs")
		}
	})

	t.Run("StoreUpdateFailure_RecordsError", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "update_fail.yaml")
		if err := fileutil.WriteFile(filePath, []byte("data: 123\n"), paths.FilePerm644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}

		store := newCycle207StorageMock()
		store.errOnUpdate = errors.New("storage disk full")
		store.addObject(map[string]any{
			objects.FieldKeyID:          "TRACK-005",
			objects.FieldKeyKind:        objects.KindImportTracking,
			objects.FieldKeySourceFile:  filePath,
			objects.FieldKeySourceMtime: "1999-01-01T00:00:00Z",
			objects.FieldKeySourceHash:  "old_hash",
		})

		reconciler := NewSemanticReconciler(store)
		results, err := reconciler.CheckDrift(ctx, secCtx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if results[0].Error == nil {
			t.Fatal("expected error recorded on store update failure")
		}
	})

	t.Run("DirectoryPassedAsFile_CalculateHashError", func(t *testing.T) {
		tmpDir := t.TempDir()
		dirPath := filepath.Join(tmpDir, "directory_target")
		if err := fileutil.Mkdir(dirPath, paths.DirPerm755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}

		store := newCycle207StorageMock()
		store.addObject(map[string]any{
			objects.FieldKeyID:          "TRACK-006",
			objects.FieldKeyKind:        objects.KindImportTracking,
			objects.FieldKeySourceFile:  dirPath,
			objects.FieldKeySourceMtime: "",
			objects.FieldKeySourceHash:  "",
		})

		reconciler := NewSemanticReconciler(store)
		results, err := reconciler.CheckDrift(ctx, secCtx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if results[0].Error == nil {
			t.Fatal("expected error when reading directory as file in calculateHash")
		}
	})
}

func TestSemanticReconciler_CheckConvergenceDrift_Elevation(t *testing.T) {
	t.Parallel()
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := context.Background()

	t.Run("ListError", func(t *testing.T) {
		store := newCycle207StorageMock()
		store.errOnList = errors.New("list convergence error")

		reconciler := NewSemanticReconciler(store)
		_, err := reconciler.CheckConvergenceDrift(ctx, secCtx)
		if err == nil {
			t.Fatal("expected error on list convergence failure, got nil")
		}
	})

	t.Run("TrendingAway_And_StableSessions", func(t *testing.T) {
		store := newCycle207StorageMock()
		store.addObject(map[string]any{
			objects.FieldKeyID:              "CVS-A",
			objects.FieldKeyKind:            objects.KindConvergenceSession,
			objects.FieldKeyStatus:          "active",
			objects.FieldKeyTitle:           "Session Alpha",
			objects.FieldKeyDeltaAssessment: "trending_away",
		})
		store.addObject(map[string]any{
			objects.FieldKeyID:              "CVS-B",
			objects.FieldKeyKind:            objects.KindConvergenceSession,
			objects.FieldKeyStatus:          "active",
			objects.FieldKeyTitle:           "Session Beta",
			objects.FieldKeyDeltaAssessment: "converging",
		})

		reconciler := NewSemanticReconciler(store)
		results, err := reconciler.CheckConvergenceDrift(ctx, secCtx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 2 {
			t.Fatalf("expected 2 results, got %d", len(results))
		}
		if !results[0].TrendingAway {
			t.Fatal("expected session A to be trending away")
		}
		if results[1].TrendingAway {
			t.Fatal("expected session B not to be trending away")
		}
	})
}

func TestCalculateHash_Helper(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "hash_test.txt")
	if err := fileutil.WriteFile(filePath, []byte("deterministic content"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	hashVal, err := calculateHash(filePath)
	if err != nil {
		t.Fatalf("unexpected calculateHash error: %v", err)
	}
	if len(hashVal) != 64 {
		t.Fatalf("expected 64-character sha256 hex string, got %d chars: %s", len(hashVal), hashVal)
	}

	nonExistent := filepath.Join(tmpDir, "missing.txt")
	_, errMissing := calculateHash(nonExistent)
	if errMissing == nil {
		t.Fatal("expected error on missing file")
	}
}

func TestScanners_Elevation(t *testing.T) {
	t.Parallel()

	t.Run("StructuredDataScanner_JSONOnly", func(t *testing.T) {
		root := t.TempDir()
		// Only JSON files present to exercise sophistication none -> basic transition
		for i := 1; i <= 4; i++ {
			p := filepath.Join(root, fmt.Sprintf("data%d.json", i))
			if err := fileutil.WriteFile(p, []byte("{}"), paths.FilePerm644); err != nil {
				t.Fatalf("failed to write json: %v", err)
			}
		}

		scanner := &StructuredDataScanner{}
		indicators, err := scanner.Scan(root)
		if err != nil {
			t.Fatalf("unexpected scan error: %v", err)
		}
		if len(indicators) != 1 {
			t.Fatalf("expected 1 indicator, got %d", len(indicators))
		}
		ind := indicators[0]
		if !ind.Found {
			t.Fatal("expected indicator.Found = true")
		}
		if ind.Sophistication != "basic" {
			t.Fatalf("expected sophistication basic, got %s", ind.Sophistication)
		}
		if len(ind.Examples) > 3 {
			t.Fatalf("expected at most 3 examples, got %d", len(ind.Examples))
		}
	})

	t.Run("StructuredDataScanner_EmptyDir", func(t *testing.T) {
		root := t.TempDir()
		scanner := &StructuredDataScanner{}
		indicators, err := scanner.Scan(root)
		if err != nil {
			t.Fatalf("unexpected scan error: %v", err)
		}
		if indicators != nil {
			t.Fatalf("expected nil indicators on empty dir, got %v", indicators)
		}
	})

	t.Run("SchemaScanner_EmptyDir", func(t *testing.T) {
		root := t.TempDir()
		scanner := &SchemaScanner{}
		indicators, err := scanner.Scan(root)
		if err != nil {
			t.Fatalf("unexpected scan error: %v", err)
		}
		if indicators != nil {
			t.Fatalf("expected nil indicators on empty dir, got %v", indicators)
		}
	})

	t.Run("OntologyScanner_EmptyDir", func(t *testing.T) {
		root := t.TempDir()
		scanner := &OntologyScanner{}
		indicators, err := scanner.Scan(root)
		if err != nil {
			t.Fatalf("unexpected scan error: %v", err)
		}
		if indicators != nil {
			t.Fatalf("expected nil indicators on empty dir, got %v", indicators)
		}
	})

	t.Run("SemanticRepositoryScanner_Endpoints", func(t *testing.T) {
		root := t.TempDir()

		// Write config files matching keywords
		envPath := filepath.Join(root, ".env")
		if err := fileutil.WriteFile(envPath, []byte("ENDPOINT=http://fuseki:3030\n"), paths.FilePerm644); err != nil {
			t.Fatalf("failed to write .env: %v", err)
		}

		scanner := &SemanticRepositoryScanner{}
		indicators, err := scanner.Scan(root)
		if err != nil {
			t.Fatalf("unexpected scan error: %v", err)
		}
		if len(indicators) != 1 {
			t.Fatalf("expected 1 indicator, got %d", len(indicators))
		}
		if !indicators[0].Found || indicators[0].Sophistication != "expert" {
			t.Fatalf("expected expert indicator, got %+v", indicators[0])
		}
	})

	t.Run("SemanticRepositoryScanner_EmptyDir", func(t *testing.T) {
		root := t.TempDir()
		scanner := &SemanticRepositoryScanner{}
		indicators, err := scanner.Scan(root)
		if err != nil {
			t.Fatalf("unexpected scan error: %v", err)
		}
		if indicators != nil {
			t.Fatalf("expected nil indicators on empty dir, got %v", indicators)
		}
	})

	t.Run("FindFiles_Exclusions", func(t *testing.T) {
		root := t.TempDir()
		gitDir := filepath.Join(root, ".git")
		vendorDir := filepath.Join(root, "vendor")
		nodeDir := filepath.Join(root, "node_modules")

		for _, dir := range []string{gitDir, vendorDir, nodeDir} {
			if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
				t.Fatalf("failed to make dir: %v", err)
			}
			f := filepath.Join(dir, "ignored.yaml")
			if err := fileutil.WriteFile(f, []byte("skip: true"), paths.FilePerm644); err != nil {
				t.Fatalf("failed to write ignored file: %v", err)
			}
		}

		validFile := filepath.Join(root, "valid.yaml")
		if err := fileutil.WriteFile(validFile, []byte("valid: true"), paths.FilePerm644); err != nil {
			t.Fatalf("failed to write valid file: %v", err)
		}

		files := findFiles(root, []string{".yaml"})
		if len(files) != 1 || files[0] != "valid.yaml" {
			t.Fatalf("expected only valid.yaml, got %v", files)
		}
	})

	t.Run("MinInt_Boundary", func(t *testing.T) {
		if minInt(5, 10) != 5 {
			t.Fatal("minInt(5, 10) should be 5")
		}
		if minInt(10, 5) != 5 {
			t.Fatal("minInt(10, 5) should be 5")
		}
		if minInt(7, 7) != 7 {
			t.Fatal("minInt(7, 7) should be 7")
		}
	})
}

func TestConcurrency_SemanticSubsystem(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	sourceFile := filepath.Join(tmpDir, "concurrent_spec.yaml")
	if err := fileutil.WriteFile(sourceFile, []byte("status: active\n"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write spec: %v", err)
	}

	store := newCycle207StorageMock()
	store.addObject(map[string]any{
		objects.FieldKeyID:          "TRACK-CONCURRENT",
		objects.FieldKeyKind:        objects.KindImportTracking,
		objects.FieldKeySourceFile:  sourceFile,
		objects.FieldKeySourceMtime: "1970-01-01T00:00:00Z",
		objects.FieldKeySourceHash:  "old",
	})
	store.addObject(map[string]any{
		objects.FieldKeyID:                  "CVS-CONCURRENT",
		objects.FieldKeyKind:                objects.KindConvergenceSession,
		objects.FieldKeyStatus:              "active",
		objects.FieldKeyTitle:               "Concurrent Test",
		objects.FieldKeyDeltaAssessment:     "trending_away",
		objects.FieldKeyTargetMaturityLevel: 1,
		objects.FieldKeyProposedTitle:       "Concurrent Task",
		objects.FieldKeyProposedPriority:    "P2",
	})

	reconciler := NewSemanticReconciler(store)
	engine := NewInferenceEngine(store)
	assessor := NewMaturityAssessor([]MaturityScanner{
		&StructuredDataScanner{},
	})

	var wg sync.WaitGroup
	workers := 8
	iterations := 20
	errChan := make(chan error, workers*iterations*3)

	for workerID := 0; workerID < workers; workerID++ {
		wg.Add(1)
		wID := workerID
		builder := goroutinelabels.NewGoroutine("semantic_test_worker", fmt.Sprintf("worker-%d", wID))
		builder.StartSimple(func() {
			defer wg.Done()
			ctx := context.Background()
			secCtx := pkgctx.NewSystemSecurityContext()

			for j := 0; j < iterations; j++ {
				// 1. Run Reconciler CheckDrift & ConvergenceDrift
				if _, err := reconciler.CheckDrift(ctx, secCtx); err != nil {
					errChan <- fmt.Errorf("reconciler drift error: %w", err)
					return
				}
				if _, err := reconciler.CheckConvergenceDrift(ctx, secCtx); err != nil {
					errChan <- fmt.Errorf("convergence drift error: %w", err)
					return
				}

				// 2. Run Inference Engine
				assessment := &MaturityAssessment{Level: 0, LevelName: "Naive"}
				inferences := engine.Infer(ctx, assessment)
				if len(inferences) == 0 {
					errChan <- errors.New("expected inferences from engine")
					return
				}

				// 3. Run Maturity Assessor
				assessRes, err := assessor.Assess(tmpDir)
				if err != nil {
					errChan <- fmt.Errorf("assessor error: %w", err)
					return
				}
				if assessRes == nil {
					errChan <- errors.New("expected non-nil assessment")
					return
				}
				time.Sleep(1 * time.Millisecond)
			}
		})
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		if err != nil {
			t.Fatalf("concurrency worker failure: %v", err)
		}
	}
}
