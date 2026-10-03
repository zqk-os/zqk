package objects

import (
	"path/filepath"
	"strings"
	"sync"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// extraSpecRoots are spec directories a linked pack registered.
// The kernel index wins when a name exists in both trees.
var (
	specRootMu     sync.Mutex
	extraSpecRoots []string
)

func addCleanDirToRootList(list *[]string, dir string) {
	if dir == "" {
		return
	}
	clean := filepath.Clean(dir)
	specRootMu.Lock()
	defer specRootMu.Unlock()
	for _, existing := range *list {
		if existing == clean {
			return
		}
	}
	*list = append(*list, clean)
}

func removeCleanDirFromRootList(list *[]string, dir string) {
	clean := filepath.Clean(dir)
	specRootMu.Lock()
	defer specRootMu.Unlock()
	var kept []string
	for _, existing := range *list {
		if existing != clean {
			kept = append(kept, existing)
		}
	}
	*list = kept
}

func copyRootList(list []string) []string {
	specRootMu.Lock()
	defer specRootMu.Unlock()
	if len(list) == 0 {
		return nil
	}
	out := make([]string, len(list))
	copy(out, list)
	return out
}

// AddSpecRoot records a directory whose YAML specs merge into the kernel spec index.
// The same directory is recorded once.
func AddSpecRoot(dir string) {
	addCleanDirToRootList(&extraSpecRoots, dir)
}

// RemoveSpecRoot drops a directory added by AddSpecRoot. Tests use it to isolate a temp root.
func RemoveSpecRoot(dir string) {
	removeCleanDirFromRootList(&extraSpecRoots, dir)
}

// ExtraSpecRoots returns the registered pack spec directories.
func ExtraSpecRoots() []string {
	return copyRootList(extraSpecRoots)
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
	addCleanDirToRootList(&extraLifecycleRoots, dir)
}

// RemoveLifecycleRoot drops a directory added by AddLifecycleRoot. Tests use it to isolate a temp root.
func RemoveLifecycleRoot(dir string) {
	removeCleanDirFromRootList(&extraLifecycleRoots, dir)
}

// ExtraLifecycleRoots returns the registered pack lifecycle directories.
func ExtraLifecycleRoots() []string {
	return copyRootList(extraLifecycleRoots)
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

func isPathUnderDir(baseDir, target string) bool {
	rel, err := filepath.Rel(baseDir, target)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func pathInExtraSpecRoot(absPath string) bool {
	target, err := filepath.Abs(absPath)
	if err != nil {
		return false
	}
	target = filepath.Clean(target)
	repoRoot, _ := moduleRootForSpecs()
	testRoot := zqkenv.TestRoot().Get()
	for _, root := range ExtraSpecRoots() {
		cleanRoot := filepath.Clean(root)
		if isPathUnderDir(cleanRoot, target) {
			return true
		}
		if repoRoot != "" && testRoot != "" {
			if relToRepo, err := filepath.Rel(repoRoot, cleanRoot); err == nil && !strings.HasPrefix(relToRepo, "..") {
				testEquiv := filepath.Join(testRoot, relToRepo)
				if isPathUnderDir(testEquiv, target) {
					return true
				}
			}
		}
	}
	return false
}
