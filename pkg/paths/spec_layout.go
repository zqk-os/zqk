package paths

import (
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// ObjectSpecDomainDirs are the on-disk buckets under object specs (studio layout).
// Community kernels may keep specs flat beside these names.
var ObjectSpecDomainDirs = []string{"dna", "kernel", "pm", "qa", "agent", "platform"}

// ObjectSpecFileName is the YAML filename for an object kind spec.
func ObjectSpecFileName(kind string) string {
	if kind == "" {
		return ""
	}
	return kind + YAMLExtension
}

// ObjectSpecPathCandidates lists flat then domain-bucket paths for a kind spec.
func ObjectSpecPathCandidates(specsDir, kind string) []string {
	name := ObjectSpecFileName(kind)
	if specsDir == "" || name == "" {
		return nil
	}
	out := make([]string, 0, 1+len(ObjectSpecDomainDirs))
	out = append(out, filepath.Join(specsDir, name))
	for _, domain := range ObjectSpecDomainDirs {
		out = append(out, filepath.Join(specsDir, domain, name))
	}
	return out
}

// FindObjectSpecFile returns the first existing candidate for kind under specsDir.
func FindObjectSpecFile(specsDir, kind string) (string, error) {
	var lastErr error
	for _, p := range ObjectSpecPathCandidates(specsDir, kind) {
		if _, err := fileutil.Stat(p); err == nil {
			return p, nil
		} else {
			lastErr = err
		}
	}
	if lastErr == nil {
		lastErr = errfmt.Errorf("object spec %s not found", kind)
	}
	return "", lastErr
}
