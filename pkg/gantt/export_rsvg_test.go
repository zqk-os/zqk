package gantt

import (
	"path/filepath"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestExportPNGWithRsvgConvert(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svgPath := filepath.Join(dir, "in.svg")
	pngPath := filepath.Join(dir, "out.png")
	if err := WriteMinimalInteractionSVGFile(svgPath); err != nil {
		t.Fatal(err)
	}
	if err := ExportPNGWithRsvgConvert(svgPath, pngPath); err != nil {
		t.Skipf("rsvg-convert unavailable: %v", err)
	}
	st, err := fileutil.Stat(pngPath)
	if err != nil || st.Size() == 0 {
		t.Fatalf("expected non-empty PNG, err=%v", err)
	}
}

func TestExportPDFWithRsvgConvert(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svgPath := filepath.Join(dir, "in.svg")
	pdfPath := filepath.Join(dir, "out.pdf")
	if err := WriteMinimalInteractionSVGFile(svgPath); err != nil {
		t.Fatal(err)
	}
	if err := ExportPDFWithRsvgConvert(svgPath, pdfPath); err != nil {
		t.Skipf("rsvg-convert unavailable: %v", err)
	}
	st, err := fileutil.Stat(pdfPath)
	if err != nil || st.Size() == 0 {
		t.Fatalf("expected non-empty PDF, err=%v", err)
	}
}
