package paths

import (
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/stampmemo"
)

// ObjectSpecDomainDirs are the on-disk buckets under object specs (studio layout).
// Community kernels may keep specs flat beside these names. Lifecycles use the same buckets.
var ObjectSpecDomainDirs = []string{"dna", "kernel", "pm", "qa", "agent", "platform"}

// domainFiles is keyed by dir+\0+fileName (closed spec/lifecycle tree). Stamp is the parent dir.
var domainFiles stampmemo.Table[string]

// ObjectSpecFileName is the YAML filename for an object kind spec.
func ObjectSpecFileName(kind string) string {
	if kind == "" {
		return ""
	}
	return kind + YAMLExtension
}

// DomainPathCandidates lists flat then domain-bucket paths for fileName under dir.
func DomainPathCandidates(dir, fileName string) []string {
	if dir == "" || fileName == "" {
		return nil
	}
	out := make([]string, 0, 1+len(ObjectSpecDomainDirs))
	out = append(out, filepath.Join(dir, fileName))
	for _, domain := range ObjectSpecDomainDirs {
		out = append(out, filepath.Join(dir, domain, fileName))
	}
	return out
}

// ObjectSpecPathCandidates lists flat then domain-bucket paths for a kind spec.
func ObjectSpecPathCandidates(specsDir, kind string) []string {
	return DomainPathCandidates(specsDir, ObjectSpecFileName(kind))
}

// FindDomainFile returns the first existing candidate for fileName under dir.
// Missing files are "".
func FindDomainFile(dir, fileName string) string {
	if dir == "" || fileName == "" {
		return ""
	}
	hit, _ := domainFiles.Load(dir+"\x00"+fileName, stampmemo.Of(dir), func() (string, error) {
		return stampmemo.FirstExisting(DomainPathCandidates(dir, fileName)), nil
	})
	return hit
}

// FindObjectSpecFile returns the first existing candidate for kind under specsDir.
func FindObjectSpecFile(specsDir, kind string) (string, error) {
	if specsDir == "" || kind == "" {
		return "", errfmt.Errorf("object spec %s not found", kind)
	}
	if hit := FindDomainFile(specsDir, ObjectSpecFileName(kind)); hit != "" {
		return hit, nil
	}
	return "", errfmt.Errorf("object spec %s not found", kind)
}

// FindLifecycleFile returns the first existing {kind}_lifecycle.yaml under dir.
func FindLifecycleFile(dir, kind string) string {
	if kind == "" {
		return ""
	}
	return FindDomainFile(dir, kind+"_lifecycle.yaml")
}
