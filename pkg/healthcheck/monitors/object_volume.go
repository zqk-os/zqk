package monitors

import (
	"context"

	"github.com/zqk-os/zqk/pkg/healthcheck"
	"github.com/zqk-os/zqk/pkg/paths"
)

// objectVolumeMonitor checks recent object volume for high-volume kinds
// using the compact object_volume time series written by system object-count-report.
//
// NOTE: This is a first pass focused on audit_event only. Before expanding it,
// align thresholds with retention_tolerance config and WAL/maintenance capacity
// (see PRI-218 backlog items).
type objectVolumeMonitor struct{}

func (m *objectVolumeMonitor) ID() string   { return "object_volume" }
func (m *objectVolumeMonitor) Name() string { return "Object volume and accumulation" }

func (m *objectVolumeMonitor) Run(ctx context.Context, projectRoot string) (*healthcheck.Result, error) {
	return evaluateVolumeMonitor(projectRoot, volumeMonitorConfig{
		name:              "object_volume",
		metricsSubdir:     paths.MetricsObjectVolumeSubdir,
		seriesFmt:         objectVolumeSeriesFmt,
		kind:              kindAuditEvent,
		failThreshold:     5000,
		degradedThreshold: 2000,
		failRate:          8000,
		degradedRate:      4000,
		summaryPrefix:     "",
	})
}

func init() {
	healthcheck.DefaultRegistry.Register(&objectVolumeMonitor{})
}
