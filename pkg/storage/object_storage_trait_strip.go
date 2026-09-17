package storage

import (
	"path/filepath"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"gopkg.in/yaml.v3"
	// traitNormOnce/traitNormRegistry lazily load process traits for expanded-trait comparison (BLI-210).
	// One registry per FileObjectStorage instance (correct projectRoot in tests and multi-project daemons).
)

func (f *FileObjectStorage) traitRegistryForPersistenceNormalize() *objects.TraitRegistry {
	if f == nil {
		return nil
	}
	f.traitNormOnce.Do(func() {
		tr := objects.NewTraitRegistry()
		dir := filepath.Join(f.projectRoot, paths.ProcessInternalTraitsDir)
		var _err_83398746 = tr.LoadTraitsFromDirectory(dir)
		if _err_83398746 != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83398746).Log()
		}
		f.traitNormRegistry = tr
	})
	return f.traitNormRegistry
}

func (f *FileObjectStorage) maybeStripRedundantTopLevelTraits(obj map[string]any) {
	if f == nil || f.specLoader == nil {
		return
	}
	tr := f.traitRegistryForPersistenceNormalize()
	objects.MaybeStripRedundantTopLevelTraits(obj, f.specLoader, tr)
}

// yamlMarshalForPersistence applies optional trait normalization then marshals to YAML.
// TRACK: [REDACTED-ID] — use FormatMultiLineYAML (literal |) so multi-line
// fields are not re-serialized as double-quoted scalars with \n escapes.
func (f *FileObjectStorage) yamlMarshalForPersistence(obj map[string]any) ([]byte, error) {
	f.maybeStripRedundantTopLevelTraits(obj)
	return yaml.Marshal(obj)
}
