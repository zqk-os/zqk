package docman

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestRegisterShippedDocs_EmptyRoot(t *testing.T) {
	t.Parallel()
	logger := logging.GetLoggerFromProfile("debug")
	if _, _, err := RegisterShippedDocs(context.Background(), "", logger); err == nil {
		t.Errorf("expected error on empty projectRoot")
	}
}

func TestValidateDocEntryYAML(t *testing.T) {
	t.Parallel()
	valid := []byte("title: Valid Doc\ncategory: architecture\n")
	if err := validateDocEntryYAML(valid); err != nil {
		t.Errorf("expected valid YAML, got error: %v", err)
	}

	invalid := []byte("title: [broken: yaml")
	if err := validateDocEntryYAML(invalid); err == nil {
		t.Errorf("expected error on invalid YAML")
	}
}

func TestVerifier_SubtreeFilterAndMissingFile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tmpDir := t.TempDir()

	mockStorage := newMockDocStorageProvider()

	// Doc pointing to non-existent file
	docMissing := map[string]any{
		objects.FieldKeyID:     "DOC-MISSING-001",
		objects.FieldKeyKind:   objects.KindDocEntry,
		objects.FieldKeyStatus: "active",
		objects.FieldKeyPath:   "docs/guides/missing.md",
	}
	_ = mockStorage.Create(ctx, nil, docMissing)

	// Doc pointing to real file under architecture
	relArch := "docs/architecture/real.md"
	absArch := filepath.Join(tmpDir, relArch)
	_ = fileutil.MkdirAll(filepath.Dir(absArch), paths.DirPerm750)
	_ = fileutil.WriteFile(absArch, []byte("# Real Title\nBody content."), paths.FilePerm600)

	docArch := map[string]any{
		objects.FieldKeyID:     "DOC-ARCH-001",
		objects.FieldKeyKind:   objects.KindDocEntry,
		objects.FieldKeyStatus: "active",
		objects.FieldKeyPath:   relArch,
	}
	_ = mockStorage.Create(ctx, nil, docArch)

	verifier := NewVerifier(mockStorage, tmpDir)

	// 1. Test IDs filter with empty IDs mixed in
	resIDs, err := verifier.Verify(ctx, "debug", VerifyOptions{
		IDs: []string{"", "DOC-MISSING-001", "  "},
	})
	if err != nil {
		t.Fatalf("Verify IDs failed: %v", err)
	}
	if resIDs.TotalChecked != 1 || resIDs.Missing != 1 {
		t.Errorf("expected 1 checked and 1 missing, got checked=%d, missing=%d", resIDs.TotalChecked, resIDs.Missing)
	}

	// 2. Test Subtrees filter (matching docs/architecture only)
	resSub, err := verifier.Verify(ctx, "debug", VerifyOptions{
		Subtrees: []string{"docs/architecture"},
	})
	if err != nil {
		t.Fatalf("Verify Subtrees failed: %v", err)
	}
	if resSub.TotalChecked != 1 {
		t.Errorf("expected 1 doc matching docs/architecture, got %d", resSub.TotalChecked)
	}

	// 3. Test ShippedOnly option with empty subtrees
	resShipped, err := verifier.Verify(ctx, "debug", VerifyOptions{
		ShippedOnly: true,
	})
	if err != nil {
		t.Fatalf("Verify ShippedOnly failed: %v", err)
	}
	if resShipped.TotalChecked == 0 {
		t.Errorf("expected non-zero docs checked under ShippedOnly")
	}
}

func TestParser_EdgeCases(t *testing.T) {
	t.Parallel()
	parser := NewParser()

	// takeFirstSentence edge cases
	if s := parser.takeFirstSentence("No punctuation in this sentence"); s != "No punctuation in this sentence" {
		t.Errorf("unexpected takeFirstSentence: %q", s)
	}
	if s := parser.takeFirstSentence("What is this? It has question."); s != "What is this" {
		t.Errorf("unexpected takeFirstSentence with question mark: %q", s)
	}
	if s := parser.takeFirstSentence("Attention! This has exclamation."); s != "Attention" {
		t.Errorf("unexpected takeFirstSentence with exclamation mark: %q", s)
	}

	// extractStatus edge cases
	if st := parser.extractStatus("Some doc with **Status**: Deprecated here."); st != "deprecated" {
		t.Errorf("expected deprecated status, got %q", st)
	}
	if st := parser.extractStatus("Some doc with **Status**: Superseded here."); st != "deprecated" {
		t.Errorf("expected deprecated status for superseded, got %q", st)
	}
	if st := parser.extractStatus("Some doc with **Status**: Active here."); st != "active" {
		t.Errorf("expected active status, got %q", st)
	}

	// determineGroupAndCategory edge cases
	grp, cat := parser.determineGroupAndCategory("docs/architecture/storage/doc.md")
	if grp != "architecture" || cat != "storage" {
		t.Errorf("expected architecture/storage, got %q/%q", grp, cat)
	}

	grp2, cat2 := parser.determineGroupAndCategory("docs/architecture/validation/doc.md")
	if grp2 != "architecture" || cat2 != "validation" {
		t.Errorf("expected architecture/validation, got %q/%q", grp2, cat2)
	}

	grp3, cat3 := parser.determineGroupAndCategory("docs/marketing/doc.md")
	if grp3 != "other" || cat3 != "strategic" {
		t.Errorf("expected other/strategic, got %q/%q", grp3, cat3)
	}

	grp4, cat4 := parser.determineGroupAndCategory("docs/process/_internal/doc.md")
	if grp4 != "process" || cat4 != "internal" {
		t.Errorf("expected process/internal, got %q/%q", grp4, cat4)
	}
}
