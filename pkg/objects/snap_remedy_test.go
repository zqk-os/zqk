package objects_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"gopkg.in/yaml.v3"
)

// TestSnapRemediesResolved asserts that historical snap-remedy technical debts
// targeting ACC-1785920548450214012-68b850c0, WS-CODE_EVAL, and PRI-CODE_EVAL
// are validated and marked resolved.
func TestSnapRemediesResolved(t *testing.T) {
	projectRoot := paths.ResolveProjectRoot("")
	require.NotEmpty(t, projectRoot, "must locate project root")

	debtDir := filepath.Join(projectRoot, ".zqk", "process", "technical_debts")
	entries, err := fileutil.ReadDir(debtDir)
	if err != nil {
		if fileutil.IsNotExist(err) {
			t.Skip("technical debts process directory not populated in this checkout")
		}
		require.NoError(t, err, "must read technical debts directory")
	}

	snapDebts := map[string]string{
		"TDE-1790035589837997000-df73663e": "ACC-1785920548450214012-68b850c0",
		"TDE-1790286537872057000-bdb4526f": "WS-CODE_EVAL",
		"TDE-1790317569918886000-4dd63be9": "PRI-CODE_EVAL",
	}

	found := make(map[string]bool)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		path := filepath.Join(debtDir, entry.Name())
		data, readErr := fileutil.ReadFile(path)
		if readErr != nil {
			continue
		}
		var doc map[string]any
		if parseErr := yaml.Unmarshal(data, &doc); parseErr != nil {
			continue
		}
		id, _ := doc["id"].(string)
		if _, ok := snapDebts[id]; ok {
			status, _ := doc["status"].(string)
			require.Equal(t, "resolved", status, "snap remedy %s must be marked resolved", id)
			found[id] = true
		}
	}

	for id := range snapDebts {
		require.True(t, found[id], "snap remedy %s must exist in technical debts directory", id)
	}
}
