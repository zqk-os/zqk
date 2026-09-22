package bldr_config_v1_test

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/specbuilder/bldr_config_v1"
)

func TestAllConfigBuilders_ConstructionAndIntegrity(t *testing.T) {
	builders := []struct {
		name string
		fn   func() any
	}{
		{"BlockingCheckConfigBuilder", func() any { return bldr_config_v1.NewBlockingCheckConfigBuilder() }},
		{"CleanupConfigBuilder", func() any { return bldr_config_v1.NewCleanupConfigBuilder() }},
		{"IdPrefixesConfigBuilder", func() any { return bldr_config_v1.NewIdPrefixesConfigBuilder() }},
		{"KindMappingsConfigBuilder", func() any { return bldr_config_v1.NewKindMappingsConfigBuilder() }},
		{"NamespacesConfigBuilder", func() any { return bldr_config_v1.NewNamespacesConfigBuilder() }},
		{"PathsConfigBuilder", func() any { return bldr_config_v1.NewPathsConfigBuilder() }},
		{"ScannerConfigBuilder", func() any { return bldr_config_v1.NewScannerConfigBuilder() }},
		{"SchedulerLogsConfigBuilder", func() any { return bldr_config_v1.NewSchedulerLogsConfigBuilder() }},
		{"SchedulerMaintenanceConfigBuilder", func() any { return bldr_config_v1.NewSchedulerMaintenanceConfigBuilder() }},
	}

	for _, tc := range builders {
		t.Run(tc.name, func(t *testing.T) {
			b := tc.fn()
			if b == nil {
				t.Fatalf("builder %s returned nil", tc.name)
			}
		})
	}
}
