package paths

import (
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/stampmemo"
)

// ObjectSpecDomainDirs are the on-disk buckets under object specs (studio layout).
// Community kernels may keep specs flat beside these names.
var ObjectSpecDomainDirs = []string{"dna", "kernel", "pm", "qa", "agent", "platform"}

var objectSpecFiles stampmemo.Table[string]

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
	if specsDir == "" || kind == "" {
		return "", errfmt.Errorf("object spec %s not found", kind)
	}
	return objectSpecFiles.Load(specsDir+"\x00"+kind, stampmemo.Of(specsDir), func() (string, error) {
		if hit := stampmemo.FirstExisting(ObjectSpecPathCandidates(specsDir, kind)); hit != "" {
			return hit, nil
		}
		return "", errfmt.Errorf("object spec %s not found", kind)
	})
}
