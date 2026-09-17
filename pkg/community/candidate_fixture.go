package community

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// Product checkout the community TPM owns. Studio sync must not target this.
const publicCandidateProductDirName = "zqk-public-candidate"

// Disposable studio export. sync-public-candidate.sh defaults here.
const publicCandidateExportDirName = "zqk-public-candidate-export"

// seatedCommunityKernel reports a dest that is a live kernel, not a disposable export.
// TRACK: BLI-1789619419231762000-7f87694b — remove when: sync-public-candidate never targets a seated checkout.
func seatedCommunityKernel(dir string) bool {
	if dir == "" {
		return false
	}
	if fileutil.Exists(filepath.Join(dir, ".zqk", "process")) {
		return true
	}
	return fileutil.Exists(filepath.Join(dir, ".env"))
}

func siblingNamed(studioRoot, name string) string {
	return filepath.Join(filepath.Dir(studioRoot), name)
}

// publicCandidateFixture returns the studio export tree, never the TPM product checkout.
func publicCandidateFixture(t *testing.T) string {
	t.Helper()
	root := paths.ResolveProjectRoot(".")
	product := siblingNamed(root, publicCandidateProductDirName)
	dir := zqkenv.PublicCandidateDir().Get()
	if dir == "" {
		dir = siblingNamed(root, publicCandidateExportDirName)
	}
	if filepath.Clean(dir) == filepath.Clean(product) {
		t.Skipf("refusing fixture %s: that path is the community product checkout, not the export", dir)
	}
	if !fileutil.Exists(dir) {
		t.Skipf("no studio export at %s (run sync-public-candidate.sh; dest is not %s)", dir, product)
	}
	if seatedCommunityKernel(dir) {
		t.Skipf("skipping: %s is a seated kernel (.zqk/process or .env), not a disposable export", dir)
	}
	return dir
}
