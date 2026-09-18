package objects

import (
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
)

// PrewarmGlobalsForProjectRoot warms the global kind mapper, spec loader, and field registry
// for the given project root. Preferred pattern: load once (here), cache (singleton registry),
// hot path uses GetFieldsForKindIfLoaded so it never blocks on LoadFields (CLI_PERFORMANCE §2).
//
// Heavy work (spec loader loop, field registry LoadFields) runs in background so PreRunE
// does not block 30–90s. Resolver and mapper init stay sync so storage is usable immediately.
// Call this when storage is created (CLI OnStorageCreated) or in tests.
// See OBJECT_OPERATIONS_PERFORMANCE.md and CACHE_MANAGEMENT_STRATEGY.
func PrewarmGlobalsForProjectRoot(projectRoot string) {
	if projectRoot == emptyValue {
		return
	}
	processDir := datacell.ProcessPrimaryDir(projectRoot)
	specsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	if mapper := GetGlobalKindMapper(); mapper != nil {
		mapper.SetDirectories(processDir, specsDir)
		_ = mapper.Initialize()
	}
	// Run heavy prewarm (83 specs + field registry) in background so we don't block the CLI.
	// Hot path uses GetFieldsForKindIfLoaded; when this completes, cache is warm for later use.
	goroutinelabels.NewGoroutine("objects", "prewarm").StartSimple(func() {
		loader := GetGlobalSpecLoader()
		if mapper := GetGlobalKindMapper(); mapper != nil {
			for _, kind := range mapper.GetAllKinds() {
				_, _ = loader.LoadSpecWithInheritance(kind + ".yaml")
			}
		}
		if fr := GetGlobalFieldRegistry(); fr != nil {
			_ = fr.LoadFields()
		}
	})
}
