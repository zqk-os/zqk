package gantt

import (
	"os/exec"

	"github.com/zqk-os/zqk/pkg/execwrap"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// ExportPNGWithRsvgConvert renders SVG to PNG using the rsvg-convert binary (librsvg).
// Returns an error if the tool is not installed or conversion fails.
func ExportPNGWithRsvgConvert(svgPath, pngPath string) error {
	rsvg, err := exec.LookPath("rsvg-convert")
	if err != nil {
		return errfmt.Newf("rsvg-convert not in PATH").Wrap(err)
	}
	cmd := execwrap.Command(rsvg, "-o", pngPath, svgPath)
	out, runErr := cmd.CombinedOutput()
	if runErr != nil {
		return errfmt.Errorf("rsvg-convert: %w: %s", runErr, string(out))
	}
	return nil
}

// ExportPDFWithRsvgConvert renders SVG to PDF using rsvg-convert (-f pdf).
func ExportPDFWithRsvgConvert(svgPath, pdfPath string) error {
	rsvg, err := exec.LookPath("rsvg-convert")
	if err != nil {
		return errfmt.Newf("rsvg-convert not in PATH").Wrap(err)
	}
	cmd := execwrap.Command(rsvg, "-f", "pdf", "-o", pdfPath, svgPath)
	out, runErr := cmd.CombinedOutput()
	if runErr != nil {
		return errfmt.Errorf("rsvg-convert pdf: %w: %s", runErr, string(out))
	}
	return nil
}

// WriteMinimalInteractionSVGFile writes MinimalInteractionSVG to path (for export tests/tools).
func WriteMinimalInteractionSVGFile(path string) error {
	return fileutil.WriteSecureFile(path, []byte(MinimalInteractionSVG()))
}
