package metrics

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestLoadSamplerConfig_ValidAndInvalid(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Non-existent file
	_, err := LoadSamplerConfig(filepath.Join(tempDir, "missing.yaml"))
	assert.Error(t, err)

	// 2. Invalid YAML
	badYAML := filepath.Join(tempDir, "bad.yaml")
	require.NoError(t, fileutil.WriteStandardFile(badYAML, []byte("invalid: yaml: [")))
	_, err = LoadSamplerConfig(badYAML)
	assert.Error(t, err)

	// 3. Valid YAML
	validYAML := filepath.Join(tempDir, "sampler.yaml")
	content := `defaults:
  enabled: true
  batch_size: 10
  max_batch_size: 50
  flush_interval: "5m"
  group_by_object_id: true
opt_out:
  - requirement
opt_in:
  - backlog_item
samplers:
  - object_kind: backlog_item
    metric_type: counter
    enabled: true
    batch_size: 5
    max_batch_size: 20
    flush_interval: "1m"
    group_by_object_id: false
`
	require.NoError(t, fileutil.WriteStandardFile(validYAML, []byte(content)))
	cfg, err := LoadSamplerConfig(validYAML)
	require.NoError(t, err)
	assert.True(t, cfg.Defaults.Enabled)
	assert.Equal(t, 10, cfg.Defaults.BatchSize)
	assert.Equal(t, "5m", cfg.Defaults.FlushInterval)
	assert.Contains(t, cfg.OptOut, "requirement")
	assert.Contains(t, cfg.OptIn, "backlog_item")
	assert.Len(t, cfg.Samplers, 1)

	// 4. ApplySamplerConfig
	registry := NewSamplerRegistry(nil)
	err = ApplySamplerConfig(registry, cfg, tempDir)
	require.NoError(t, err)

	// 5. Invalid flush interval in ApplySamplerConfig
	badIntervalCfg := &SamplerConfigFile{}
	badIntervalCfg.Defaults.FlushInterval = "invalid-duration"
	err = ApplySamplerConfig(registry, badIntervalCfg, tempDir)
	assert.Error(t, err)
}

func TestFindSamplerConfigFile(t *testing.T) {
	tempDir := t.TempDir()

	// Missing config
	_, err := FindSamplerConfigFile(tempDir)
	assert.Error(t, err)

	// Write candidate file
	candPath := filepath.Join(tempDir, paths.ProjectDataDir, paths.MetricsDir, paths.SamplerConfigFile)
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(candPath)))
	require.NoError(t, fileutil.WriteStandardFile(candPath, []byte("defaults:\n  enabled: true\n")))

	found, err := FindSamplerConfigFile(tempDir)
	require.NoError(t, err)
	assert.Equal(t, candPath, found)
}

func TestShouldUseSampling_Branches(t *testing.T) {
	// 1. Opt-out check
	optOutCfg := &SamplerConfigFile{
		OptOut: []string{"requirement"},
	}
	optOutCfg.Defaults.Enabled = true
	assert.False(t, ShouldUseSampling(optOutCfg, "requirement"))
	assert.True(t, ShouldUseSampling(optOutCfg, "goal"))

	// 2. Opt-in check
	optInCfg := &SamplerConfigFile{
		OptIn: []string{"backlog_item"},
	}
	optInCfg.Defaults.Enabled = true
	assert.True(t, ShouldUseSampling(optInCfg, "backlog_item"))
	assert.False(t, ShouldUseSampling(optInCfg, "goal"))

	// 3. Defaults check
	defaultEnabledCfg := &SamplerConfigFile{}
	defaultEnabledCfg.Defaults.Enabled = true
	assert.True(t, ShouldUseSampling(defaultEnabledCfg, "any_kind"))

	defaultDisabledCfg := &SamplerConfigFile{}
	defaultDisabledCfg.Defaults.Enabled = false
	assert.False(t, ShouldUseSampling(defaultDisabledCfg, "any_kind"))
}
