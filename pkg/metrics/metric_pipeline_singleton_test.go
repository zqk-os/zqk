package metrics

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/storage"
)

func TestMetricPipelineForProject_SingletonPerRoot(t *testing.T) {
	t.Cleanup(ResetMetricPipelineSingletonsForTest)

	st := storage.NewNoopObjectStorage()
	root := "/tmp/zqk-metric-pipeline-singleton-test"

	a := MetricPipelineForProject(st, root)
	b := MetricPipelineForProject(st, root)
	if a != b {
		t.Fatal("expected same MetricPipeline instance for same project root")
	}

	emptyA := MetricPipelineForProject(st, "")
	emptyB := MetricPipelineForProject(st, "")
	if emptyA == emptyB {
		t.Fatal("expected fresh pipeline when project root is empty")
	}
}
