package monitors

import (
	"context"

	"github.com/zqk-os/zqk/pkg/healthcheck"
	"github.com/zqk-os/zqk/pkg/paths"
)

// streamVolumeMonitor checks recent stream volume for high-volume kinds (currently audit_event)
// using the compact time series written from the high-volume event cache (stream_volume/*).
// This is complementary to object_volume (CAS-backed kinds) and focuses on stream-backed data
// so we can detect accumulation that would not show up in the object ID cache.
type streamVolumeMonitor struct{}

func (m *streamVolumeMonitor) ID() string   { return "stream_volume" }
func (m *streamVolumeMonitor) Name() string { return "Stream volume and accumulation" }

func (m *streamVolumeMonitor) Run(ctx context.Context, projectRoot string) (*healthcheck.Result, error) {
	return evaluateVolumeMonitor(projectRoot, volumeMonitorConfig{
		name:              "stream_volume",
		metricsSubdir:     paths.MetricsStreamVolumeSubdir,
		seriesFmt:         streamVolumeSeriesFmt,
		kind:              kindAuditEvent,
		failThreshold:     50000,
		degradedThreshold: 20000,
		failRate:          8000,
		degradedRate:      4000,
		summaryPrefix:     "stream ",
	})
}

func init() {
	healthcheck.DefaultRegistry.Register(&streamVolumeMonitor{})
}
