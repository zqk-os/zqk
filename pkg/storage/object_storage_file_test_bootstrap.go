// Extracted from object_storage_file.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"path/filepath"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testenvroot"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewFileObjectStorageForTest creates file-based storage for tests. Hash registries are not
// registered with the global shutdown coordinator, and global audit buffer/IO queue are not
// wired, so tests do not share or overwrite global state.
func NewFileObjectStorageForTest(projectRoot string) (*FileObjectStorage, error) {
	fileObjectStorageForTestOnce.Do(func() {
		caspkg.SetListingIndexWriteQueueFactoryToPerProjectRoot()
	})
	if projectRoot != "" {
		if err := ensureHermeticTestRootLayout(projectRoot); err != nil {
			return nil, errfmt.Newf("failed to bootstrap hermetic test root layout").Wrap(err)
		}
	}
	f, err := NewFileObjectStorage(projectRoot, &FileObjectStorageOptions{SkipGlobalWiring: true})
	if err != nil {
		return nil, err
	}
	f.skipHashRegistryShutdownRegistration = true
	// SkipGlobalWiring returns before BindReverseReferenceIndexProjectRoot; delete/dependents
	// still need the reverse-ref index bound for this temp root.
	BindReverseReferenceIndexProjectRoot(projectRoot)
	ensureTestIdentityCacheHandler()
	return f, nil
}

func ensureHermeticTestRootLayout(testRoot string) error {
	targetSpecs := filepath.Join(testRoot, paths.ProcessInternalObjectSpecsDir)
	targetLifecycles := filepath.Join(testRoot, paths.ProcessInternalLifecyclesDir)
	specsExist, lifecyclesExist := false, false
	if _, err := fileutil.Stat(filepath.Join(targetSpecs, "backlog_item.yaml")); err == nil {
		specsExist = true
	} else if _, err := fileutil.Stat(filepath.Join(targetSpecs, "pm", "backlog_item.yaml")); err == nil {
		specsExist = true
	}
	if _, err := fileutil.Stat(filepath.Join(targetLifecycles, "base_object_lifecycle.yaml")); err == nil {
		lifecyclesExist = true
	} else if _, err := fileutil.Stat(filepath.Join(targetLifecycles, "kernel", "base_object_lifecycle.yaml")); err == nil {
		lifecyclesExist = true
	}
	if specsExist && lifecyclesExist {
		return nil
	}
	srcRoot := findModuleRootForTestBootstrap()
	if srcRoot == "" || srcRoot == testRoot {
		return nil
	}
	if err := paths.EnsureProcessAndObjectSpecsLayout(testRoot); err != nil {
		return err
	}
	// Destination is the first argument. Passing srcRoot first copies the temp root's
	// fixture specs over the repository's real ones.
	//
	// Each concern is filled only when absent: a test that authored its own specs must
	// keep them even when lifecycles still need seeding.
	if !specsExist {
		if err := testenvroot.CopyObjectSpecsFromProject(testRoot, srcRoot); err != nil {
			return err
		}
	}
	if !lifecyclesExist {
		if err := testenvroot.CopyLifecyclesFromProject(testRoot, srcRoot); err != nil {
			return err
		}
	}
	return nil
}

func findModuleRootForTestBootstrap() string {
	if pr := zqkenv.ProjectRoot().Get(); pr != "" {
		if st, err := fileutil.Stat(filepath.Join(pr, paths.ProcessInternalLifecyclesDir)); err == nil && st.IsDir() {
			return pr
		}
	}
	dir, err := fileutil.Getwd()
	if err != nil {
		return ""
	}
	for {
		if st, err := fileutil.Stat(filepath.Join(dir, paths.ProcessInternalLifecyclesDir)); err == nil && st.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}
