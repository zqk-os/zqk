package gantt

import (
	"path/filepath"
	"testing"
)

func TestExportRsvg_ErrorsAndMissingTool(t *testing.T) {
	// Note: cannot use t.Parallel() when modifying environment variables via t.Setenv
	dir := t.TempDir()
	nonExistentSvg := filepath.Join(dir, "nonexistent.svg")
	outPath := filepath.Join(dir, "out.png")
	outPdf := filepath.Join(dir, "out.pdf")

	// 1. Run error paths (non-existent source SVG)
	if err := ExportPNGWithRsvgConvert(nonExistentSvg, outPath); err == nil {
		t.Errorf("expected error when source SVG does not exist")
	}
	if err := ExportPDFWithRsvgConvert(nonExistentSvg, outPdf); err == nil {
		t.Errorf("expected error when source SVG does not exist for PDF")
	}

	// 2. Missing rsvg-convert in PATH
	t.Setenv("PATH", "")
	if err := ExportPNGWithRsvgConvert(nonExistentSvg, outPath); err == nil {
		t.Errorf("expected error when rsvg-convert not in PATH")
	}
	if err := ExportPDFWithRsvgConvert(nonExistentSvg, outPdf); err == nil {
		t.Errorf("expected error when rsvg-convert not in PATH for PDF")
	}
}
