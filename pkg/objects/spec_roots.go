package objects

import (
	"path/filepath"
	"strings"
	"sync"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// extraSpecRoots are spec directories a linked pack registered.
// The kernel index wins when a name exists in both trees.
var (
	specRootMu     sync.Mutex
	extraSpecRoots []string
)

// AddSpecRoot records a directory whose YAML specs merge into the kernel spec index.
// The same directory is recorded once.
func AddSpecRoot(dir string) {
	if dir == "" {
		return
	}
	clean := filepath.Clean(dir)
	specRootMu.Lock()
	defer specRootMu.Unlock()
	for _, existing := range extraSpecRoots {
		if existing == clean {
			return
		}
	}
	extraSpecRoots = append(extraSpecRoots, clean)
}

// RemoveSpecRoot drops a directory added by AddSpecRoot. Tests use it to isolate a temp root.
func RemoveSpecRoot(dir string) {
	clean := filepath.Clean(dir)
	specRootMu.Lock()
	defer specRootMu.Unlock()
	kept := extraSpecRoots[:0]
	for _, existing := range extraSpecRoots {
		if existing == clean {
			continue
		}
		kept = append(kept, existing)
	}
	extraSpecRoots = append([]string(nil), kept...)
}

// ExtraSpecRoots returns the registered pack spec directories.
func ExtraSpecRoots() []string {
	specRootMu.Lock()
	defer specRootMu.Unlock()
	if len(extraSpecRoots) == 0 {
		return nil
	}
	out := make([]string, len(extraSpecRoots))
	copy(out, extraSpecRoots)
	return out
}

// AddModuleSpecRoot registers rel under the kernel module root when that directory exists.
func AddModuleSpecRoot(rel string) {
	addModuleDir(rel, AddSpecRoot)
}

// extraLifecycleRoots are lifecycle directories a linked pack registered.
// The kernel tree wins when the same kind exists in both trees.
var extraLifecycleRoots []string

// AddLifecycleRoot records a directory whose lifecycle YAML merges after the kernel tree.
// The kernel file wins when the same kind exists in both trees.
func AddLifecycleRoot(dir string) {
	if dir == "" {
		return
	}
	clean := filepath.Clean(dir)
	specRootMu.Lock()
	defer specRootMu.Unlock()
	for _, existing := range extraLifecycleRoots {
		if existing == clean {
			return
		}
	}
	extraLifecycleRoots = append(extraLifecycleRoots, clean)
}

// RemoveLifecycleRoot drops a directory added by AddLifecycleRoot. Tests use it to isolate a temp root.
func RemoveLifecycleRoot(dir string) {
	clean := filepath.Clean(dir)
	specRootMu.Lock()
	defer specRootMu.Unlock()
	kept := extraLifecycleRoots[:0]
	for _, existing := range extraLifecycleRoots {
		if existing == clean {
			continue
		}
		kept = append(kept, existing)
	}
	extraLifecycleRoots = append([]string(nil), kept...)
}

// ExtraLifecycleRoots returns the registered pack lifecycle directories.
func ExtraLifecycleRoots() []string {
	specRootMu.Lock()
	defer specRootMu.Unlock()
	if len(extraLifecycleRoots) == 0 {
		return nil
	}
	out := make([]string, len(extraLifecycleRoots))
	copy(out, extraLifecycleRoots)
	return out
}

// AddModuleLifecycleRoot registers rel under the kernel module root when that directory exists.
func AddModuleLifecycleRoot(rel string) {
	addModuleDir(rel, AddLifecycleRoot)
}

func addModuleDir(rel string, add func(string)) {
	if rel == "" || add == nil {
		return
	}
	root, ok := moduleRootForSpecs()
	if !ok {
		return
	}
	dir := filepath.Join(root, filepath.FromSlash(rel))
	info, err := fileutil.Stat(dir)
	if err != nil || !info.IsDir() {
		return
	}
	add(dir)
}

func moduleRootForSpecs() (string, bool) {
	dir, err := fileutil.Getwd()
	if err != nil || dir == "" {
		return "", false
	}
	for {
		data, err := fileutil.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil {
			line, _, _ := strings.Cut(string(data), "\n")
			if strings.TrimSpace(line) == "module github.com/zqk-os/zqk" {
				return dir, true
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func specIndexKey(specsDir string, roots []string) string {
	if len(roots) == 0 {
		return specsDir
	}
	return specsDir + "\x00" + strings.Join(roots, "\x00")
}

func specIndexStamp(specsDir string, roots []string) stampmemo.Stamp {
	return stampmemo.Combine(paths.DomainTreeStamp(specsDir), stampmemo.OfAll(roots...))
}

func mergeSpecIndexes(specsDir string, roots []string) map[string]string {
	idx := paths.IndexYAMLNames(specsDir)
	for _, root := range roots {
		for name, path := range paths.IndexYAMLNames(root) {
			if _, exists := idx[name]; exists {
				continue
			}
			idx[name] = path
		}
	}
	return idx
}

// FindRegisteredSpecFile resolves a kind spec in the kernel tree, then in
// spec directories a linked pack registered. The kernel file wins.
func FindRegisteredSpecFile(specsDir, kind string) (string, error) {
	if hit, err := paths.FindObjectSpecFile(specsDir, kind); err == nil {
		return hit, nil
	}
	name := paths.ObjectSpecFileName(kind)
	if name == "" {
		return "", errfmt.Errorf("object spec %s not found", kind)
	}
	for _, root := range ExtraSpecRoots() {
		cand := filepath.Join(root, name)
		info, err := fileutil.Stat(cand)
		if err != nil || info.IsDir() {
			continue
		}
		return cand, nil
	}
	return "", errfmt.Errorf("object spec %s not found", kind)
}

func pathInExtraSpecRoot(absPath string) bool {
	target, err := filepath.Abs(absPath)
	if err != nil {
		return false
	}
	target = filepath.Clean(target)
	for _, root := range ExtraSpecRoots() {
		rel, err := filepath.Rel(filepath.Clean(root), target)
		if err != nil {
			continue
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		return true
	}
	return false
}
