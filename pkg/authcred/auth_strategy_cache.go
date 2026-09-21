package authcred

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	"gopkg.in/yaml.v3"
)

// AuthStrategyRecord is a parsed auth_strategy YAML retained until the dir mtime changes.
type AuthStrategyRecord struct {
	ID      string
	Type    string
	Enabled bool
	Status  string
}

// authStrategyRecords is keyed by project root. Stamp is the auth-strategy YAML dir.
var authStrategyRecords stampmemo.Table[[]AuthStrategyRecord]

// ListAuthStrategyRecords returns parsed auth_strategy YAML for projectRoot.
// A missing directory is an empty catalog, not an error.
func ListAuthStrategyRecords(projectRoot string) []AuthStrategyRecord {
	if strings.TrimSpace(projectRoot) == "" {
		return nil
	}
	dir := paths.AuthStrategiesDirPath(projectRoot)
	recs, _ := authStrategyRecords.Load(projectRoot, stampmemo.Of(dir), func() ([]AuthStrategyRecord, error) {
		var out []AuthStrategyRecord
		err := forEachYAMLFile(dir, func(_ string, data []byte) {
			var strategy struct {
				ID      string `yaml:"id"`
				Type    string `yaml:"strategy_type"`
				Enabled bool   `yaml:"enabled"`
				Status  string `yaml:"status"`
			}
			if yaml.Unmarshal(data, &strategy) != nil {
				return
			}
			out = append(out, AuthStrategyRecord{
				ID:      strategy.ID,
				Type:    strategy.Type,
				Enabled: strategy.Enabled,
				Status:  strategy.Status,
			})
		})
		if err != nil {
			return nil, nil
		}
		return out, nil
	})
	return recs
}
