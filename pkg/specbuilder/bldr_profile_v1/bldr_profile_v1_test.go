package bldr_profile_v1_test

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/specbuilder/bldr_profile_v1"
)

func TestAllProfileBuilders_ConstructionAndIntegrity(t *testing.T) {
	builders := []struct {
		name string
		fn   func() any
	}{
		{"BaseRouterBuilder", func() any { return bldr_profile_v1.NewBaseRouterBuilder() }},
		{"DefaultRouterBuilder", func() any { return bldr_profile_v1.NewDefaultRouterBuilder() }},
		{"HighThroughputRouterBuilder", func() any { return bldr_profile_v1.NewHighThroughputRouterBuilder() }},
		{"LowLatencyRouterBuilder", func() any { return bldr_profile_v1.NewLowLatencyRouterBuilder() }},
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
