package metrics

import (
	"path/filepath"
	"strings"
	"sync"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage"
)

var (
	metricPipelineSingletonMu sync.Mutex
	metricPipelineByRoot      map[string]*MetricPipeline
)

// MetricPipelineForProject returns a process-wide [MetricPipeline] for the given project root.
// The first caller creates the pipeline and runs [MetricPipeline.InitializeSamplerConfig];
// later callers reuse the same instance. Ephemeral coordination helpers used to call
// NewMetricPipeline+InitializeSamplerConfig on every emit, which registered a full sampler
// profile (dozens of [Sampler] + flushTickerLoop goroutines) and never called [SamplerRegistry.StopAll],
// leaking until process exit.
//
// If projectRoot is empty, returns [NewMetricPipeline] without loading sampler YAML, matching
// previous call sites that skipped InitializeSamplerConfig when the root was unknown.
func MetricPipelineForProject(storage storage.ObjectStorageProvider, projectRoot string) *MetricPipeline {
	if strings.TrimSpace(projectRoot) == "" {
		return NewMetricPipeline(storage)
	}
	key := filepath.Clean(projectRoot)

	metricPipelineSingletonMu.Lock()
	defer metricPipelineSingletonMu.Unlock()
	if metricPipelineByRoot == nil {
		metricPipelineByRoot = make(map[string]*MetricPipeline)
	}
	if p, ok := metricPipelineByRoot[key]; ok {
		return p
	}

	mp := NewMetricPipeline(storage)
	if err := mp.InitializeSamplerConfig(projectRoot); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		logging.Fluent(logger).Warn("metric pipeline: InitializeSamplerConfig failed; continuing with defaults").
			WithError(err).
			String("project_root", projectRoot).
			Log()
	}
	metricPipelineByRoot[key] = mp
	return mp
}

// ResetMetricPipelineSingletonsForTest stops and clears cached pipelines. For tests only.
func ResetMetricPipelineSingletonsForTest() {
	metricPipelineSingletonMu.Lock()
	defer metricPipelineSingletonMu.Unlock()
	for _, p := range metricPipelineByRoot {
		if p == nil {
			continue
		}
		if reg := p.GetSamplerRegistry(); reg != nil {
			_ = reg.StopAll() //nolint:errcheck // best-effort test hygiene
		}
	}
	metricPipelineByRoot = nil
}
