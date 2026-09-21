package authcred

import (
	"path/filepath"
	"strings"
	"sync"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"gopkg.in/yaml.v3"
)

// AuthStrategyRecord is a parsed auth_strategy YAML retained until the dir mtime changes.
type AuthStrategyRecord struct {
	ID      string
	Type    string
	Enabled bool
	Status  string
}

type authStrategySnap struct {
	mu       sync.Mutex
	dirMtime int64
	loaded   bool
	records  []AuthStrategyRecord
}

var authStrategySnaps sync.Map

func authStrategyCache(projectRoot string) *authStrategySnap {
	if existing, ok := authStrategySnaps.Load(projectRoot); ok {
		return existing.(*authStrategySnap)
	}
	fresh := &authStrategySnap{}
	actual, _ := authStrategySnaps.LoadOrStore(projectRoot, fresh)
	return actual.(*authStrategySnap)
}

// ListAuthStrategyRecords returns parsed auth_strategy YAML for projectRoot.
// A missing directory is an empty catalog, not an error.
func ListAuthStrategyRecords(projectRoot string) []AuthStrategyRecord {
	if strings.TrimSpace(projectRoot) == "" {
		return nil
	}
	dir := paths.AuthStrategiesDirPath(projectRoot)
	var dirMtime int64
	if info, err := fileutil.Stat(dir); err == nil {
		dirMtime = info.ModTime().UnixNano()
	}
	snap := authStrategyCache(projectRoot)
	snap.mu.Lock()
	defer snap.mu.Unlock()
	if snap.loaded && snap.dirMtime == dirMtime {
		return snap.records
	}
	entries, err := fileutil.ReadDir(dir)
	if err != nil {
		snap.loaded = true
		snap.dirMtime = dirMtime
		snap.records = nil
		return nil
	}
	out := make([]AuthStrategyRecord, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), paths.YAMLExtension) {
			continue
		}
		data, readErr := fileutil.ReadFile(filepath.Join(dir, e.Name()))
		if readErr != nil {
			continue
		}
		var strategy struct {
			ID      string `yaml:"id"`
			Type    string `yaml:"strategy_type"`
			Enabled bool   `yaml:"enabled"`
			Status  string `yaml:"status"`
		}
		if yaml.Unmarshal(data, &strategy) != nil {
			continue
		}
		out = append(out, AuthStrategyRecord{
			ID:      strategy.ID,
			Type:    strategy.Type,
			Enabled: strategy.Enabled,
			Status:  strategy.Status,
		})
	}
	snap.loaded = true
	snap.dirMtime = dirMtime
	snap.records = out
	return out
}
