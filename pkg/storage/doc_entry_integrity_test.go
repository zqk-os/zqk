package storage_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"gopkg.in/yaml.v3"
)

// TestDocEntryIntegrity_Suite is the primary catalyst test case (TST-DOC-INTEGRITY-001)
// satisfying Requirement REQ-DOC-INTEGRITY-001 and validating criteria:
// - CRIT-DOC-INTEGRITY-001 (Spec definition of content_hash and content_size)
// - CRIT-DOC-INTEGRITY-002 (Lifecycle transition preconditions)
// - CRIT-DOC-INTEGRITY-003 (Verification and drift detection)
// - CRIT-DOC-INTEGRITY-004 (Automated sealing of hash and size)
func TestDocEntryIntegrity_Suite(t *testing.T) {
	// Subtest 1: CRIT-DOC-INTEGRITY-001
	t.Run("CRIT-DOC-INTEGRITY-001_SpecDefinition", func(t *testing.T) {
		specPath := filepath.Join("..", "..", "packs", "library", "specs", "doc_entry.yaml")
		if _, err := os.Stat(specPath); err != nil {
			specPath = filepath.Join("..", "..", paths.ProcessInternalObjectSpecsDir, "pm", "doc_entry.yaml")
		}
		//nolint:gosec
		data, err := os.ReadFile(specPath)
		if err != nil {
			t.Fatalf("Failed to read doc_entry spec at %s: %v", specPath, err)
		}

		var spec struct {
			Fields map[string]struct {
				Type       string `yaml:"type"`
				Validation struct {
					Pattern string `yaml:"pattern"`
				} `yaml:"validation"`
			} `yaml:"fields"`
		}

		if err := yaml.Unmarshal(data, &spec); err != nil {
			t.Fatalf("Failed to parse doc_entry spec YAML: %v", err)
		}

		hashField, hasHash := spec.Fields["content_hash"]
		if !hasHash {
			t.Fatalf("doc_entry spec is missing required field: content_hash")
		}
		if hashField.Type != "string" {
			t.Errorf("expected content_hash type string, got %s", hashField.Type)
		}
		if hashField.Validation.Pattern != "^[a-f0-9]{64}$" {
			t.Errorf("expected content_hash pattern ^[a-f0-9]{64}$, got %s", hashField.Validation.Pattern)
		}

		sizeField, hasSize := spec.Fields["content_size"]
		if !hasSize {
			t.Fatalf("doc_entry spec is missing required field: content_size")
		}
		if sizeField.Type != "integer" && sizeField.Type != "int" {
			t.Errorf("expected content_size type integer, got %s", sizeField.Type)
		}
	})

	// Subtest 2: CRIT-DOC-INTEGRITY-002
	t.Run("CRIT-DOC-INTEGRITY-002_LifecyclePreconditions", func(t *testing.T) {
		lifecyclePath := filepath.Join("..", "..", "packs", "library", "lifecycles", "doc_entry_lifecycle.yaml")
		if _, err := os.Stat(lifecyclePath); err != nil {
			lifecyclePath = filepath.Join("..", "..", paths.ProcessInternalLifecyclesDir, "pm", "doc_entry_lifecycle.yaml")
		}
		//nolint:gosec
		data, err := os.ReadFile(lifecyclePath)
		if err != nil {
			t.Fatalf("Failed to read doc_entry lifecycle at %s: %v", lifecyclePath, err)
		}

		var lc struct {
			Transitions []struct {
				From          string   `yaml:"from"`
				To            string   `yaml:"to"`
				Preconditions []string `yaml:"preconditions"`
			} `yaml:"transitions"`
		}

		if err := yaml.Unmarshal(data, &lc); err != nil {
			t.Fatalf("Failed to parse doc_entry lifecycle YAML: %v", err)
		}

		foundReviewToPublished := false
		foundOriginatedToDraft := false
		foundDraftToReview := false

		for _, tr := range lc.Transitions {
			if tr.From == "originated" && tr.To == "draft" {
				foundOriginatedToDraft = true
				if len(tr.Preconditions) == 0 {
					t.Errorf("originated -> draft transition missing preconditions")
				}
			}
			if tr.From == "draft" && tr.To == "review" {
				foundDraftToReview = true
				if len(tr.Preconditions) == 0 {
					t.Errorf("draft -> review transition missing preconditions")
				}
			}
			if tr.From == "review" && tr.To == "published" {
				foundReviewToPublished = true
				if len(tr.Preconditions) == 0 {
					t.Errorf("review -> published transition missing preconditions")
				}
				hasHashPrecondition := false
				for _, p := range tr.Preconditions {
					if matched, _ := regexp.MatchString("(?i)hash|content_hash|sealed", p); matched {
						hasHashPrecondition = true
						break
					}
				}
				if !hasHashPrecondition {
					t.Errorf("review -> published transition requires content_hash sealing precondition, got %v", tr.Preconditions)
				}
			}
		}

		if !foundOriginatedToDraft {
			t.Errorf("missing originated -> draft transition")
		}
		if !foundDraftToReview {
			t.Errorf("missing draft -> review transition")
		}
		if !foundReviewToPublished {
			t.Errorf("missing review -> published transition")
		}
	})

	// Subtest 3: CRIT-DOC-INTEGRITY-003
	t.Run("CRIT-DOC-INTEGRITY-003_DriftDetection", func(t *testing.T) {
		tmpDir := t.TempDir()
		relDoc := "docs/architecture/sample.md"
		fullDocPath := filepath.Join(tmpDir, relDoc)
		if err := os.MkdirAll(filepath.Dir(fullDocPath), paths.DirPerm750); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}

		initialContent := "# Architecture Sample\nOriginal content."
		if err := fileutil.WriteFile(fullDocPath, []byte(initialContent), paths.FilePerm644); err != nil {
			t.Fatalf("failed to write initial doc: %v", err)
		}

		h := sha256.Sum256([]byte(initialContent))
		expectedHash := hex.EncodeToString(h[:])
		expectedSize := int64(len(initialContent))

		docObj := map[string]any{
			objects.FieldKeyID:     "DOC-SAMPLE-001",
			objects.FieldKeyKind:   objects.KindDocEntry,
			objects.FieldKeyStatus: "published",
			objects.FieldKeyPath:   "prefix:" + relDoc,
			"content_hash":         expectedHash,
			"content_size":         expectedSize,
		}

		// 1. Clean verification
		violations := storage.ValidateDocEntryIntegrity(tmpDir, docObj)
		if len(violations) != 0 {
			t.Errorf("expected 0 violations for pristine doc, got %d: %+v", len(violations), violations)
		}

		// 2. Drift verification: content modified
		modifiedContent := "# Architecture Sample\nTampered content!"
		if err := fileutil.WriteFile(fullDocPath, []byte(modifiedContent), paths.FilePerm644); err != nil {
			t.Fatalf("failed to tamper doc: %v", err)
		}
		driftViolations := storage.ValidateDocEntryIntegrity(tmpDir, docObj)
		if len(driftViolations) == 0 {
			t.Errorf("expected drift violations for modified content, got 0")
		} else {
			foundHashMismatch := false
			for _, v := range driftViolations {
				if v.Code == storage.DocIntegrityCodeHashMismatch {
					foundHashMismatch = true
					break
				}
			}
			if !foundHashMismatch {
				t.Errorf("expected DocIntegrityCodeHashMismatch, got %+v", driftViolations)
			}
		}

		// 3. Unreachable target: file removed
		if err := os.Remove(fullDocPath); err != nil {
			t.Fatalf("failed to remove doc: %v", err)
		}
		missingViolations := storage.ValidateDocEntryIntegrity(tmpDir, docObj)
		if len(missingViolations) == 0 {
			t.Errorf("expected missing target violation, got 0")
		} else {
			foundTargetMissing := false
			for _, v := range missingViolations {
				if v.Code == storage.DocIntegrityCodeTargetMissing {
					foundTargetMissing = true
					break
				}
			}
			if !foundTargetMissing {
				t.Errorf("expected DocIntegrityCodeTargetMissing, got %+v", missingViolations)
			}
		}

		// 4. Unsealed published doc
		docObjUnsealed := map[string]any{
			objects.FieldKeyID:     "DOC-SAMPLE-002",
			objects.FieldKeyKind:   objects.KindDocEntry,
			objects.FieldKeyStatus: "published",
			objects.FieldKeyPath:   "prefix:" + relDoc,
			// content_hash missing
		}
		// recreate file
		_ = fileutil.WriteFile(fullDocPath, []byte(initialContent), paths.FilePerm644)
		unsealedViolations := storage.ValidateDocEntryIntegrity(tmpDir, docObjUnsealed)
		foundUnsealed := false
		for _, v := range unsealedViolations {
			if v.Code == storage.DocIntegrityCodeUnsealedPublished {
				foundUnsealed = true
				break
			}
		}
		if !foundUnsealed {
			t.Errorf("expected DocIntegrityCodeUnsealedPublished for published doc with missing hash, got %+v", unsealedViolations)
		}
	})

	// Subtest 4: CRIT-DOC-INTEGRITY-004
	t.Run("CRIT-DOC-INTEGRITY-004_AutomatedSealing", func(t *testing.T) {
		tmpDir := t.TempDir()
		relDoc := "docs/architecture/seal_test.md"
		fullDocPath := filepath.Join(tmpDir, relDoc)
		if err := os.MkdirAll(filepath.Dir(fullDocPath), paths.DirPerm750); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		content := "# Seal Test Doc\nTesting automated SHA-256 sealing."
		if err := fileutil.WriteFile(fullDocPath, []byte(content), paths.FilePerm644); err != nil {
			t.Fatalf("failed to write doc: %v", err)
		}

		computedHash, computedSize, err := storage.ComputeFileIntegrity(fullDocPath)
		if err != nil {
			t.Fatalf("ComputeFileIntegrity failed: %v", err)
		}

		expectedHashBytes := sha256.Sum256([]byte(content))
		expectedHash := hex.EncodeToString(expectedHashBytes[:])
		if computedHash != expectedHash {
			t.Errorf("expected hash %s, got %s", expectedHash, computedHash)
		}
		if computedSize != int64(len(content)) {
			t.Errorf("expected size %d, got %d", len(content), computedSize)
		}
	})
}
