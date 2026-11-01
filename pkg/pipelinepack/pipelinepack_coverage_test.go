package pipelinepack_test

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/pipelinepack"
)

func TestPipelinePackConstantsAndKinds(t *testing.T) {
	if pipelinepack.Name != "pipeline" {
		t.Fatalf("expected Name to be 'pipeline', got %q", pipelinepack.Name)
	}
	if pipelinepack.SpecDir != "packs/pipeline/specs" {
		t.Fatalf("expected SpecDir to be 'packs/pipeline/specs', got %q", pipelinepack.SpecDir)
	}
	if pipelinepack.LifecycleDir != "packs/pipeline/lifecycles" {
		t.Fatalf("expected LifecycleDir to be 'packs/pipeline/lifecycles', got %q", pipelinepack.LifecycleDir)
	}
	if pipelinepack.OmitBuildTag != "zqk_omit_pipelinepack" {
		t.Fatalf("expected OmitBuildTag to be 'zqk_omit_pipelinepack', got %q", pipelinepack.OmitBuildTag)
	}

	kinds := pipelinepack.Kinds()
	if len(kinds) != 4 {
		t.Fatalf("expected 4 kinds, got %d", len(kinds))
	}
	expected := map[string]bool{
		"pipeline":            true,
		"pipeline_definition": true,
		"pipeline_execution":  true,
		"pipeline_stage":      true,
	}
	for _, k := range kinds {
		if !expected[k] {
			t.Errorf("unexpected kind %q in pipelinepack", k)
		}
	}
}
