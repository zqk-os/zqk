package system

import (
	stdcontext "context"
	"testing"

	"github.com/spf13/cobra"
)

func TestRunSystemCheckPipeline_NilCtxRecordsIngestStageOutcome(t *testing.T) {
	cmd := &cobra.Command{}
	outcome, err := runSystemCheckPipelineWithOutcome(cmd, nil, []string{"all"}, "op-test", stdcontext.Background())
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	if v, ok := outcome["ingest_stage_started"]; !ok || v != true {
		t.Fatalf("expected ingest_stage_started=true, got %v (ok=%v)", v, ok)
	}
	if v, ok := outcome["ingest_error"]; !ok || v == emptyValue {
		t.Fatalf("expected ingest_error set, got %v (ok=%v)", v, ok)
	}
	if outcome["ingest_error"] != "nil ctx" {
		t.Fatalf("expected ingest_error='nil ctx', got %v", outcome["ingest_error"])
	}
}

func TestRunSystemCheckPipeline_NilCmdRecordsIngestStageOutcome(t *testing.T) {
	outcome, err := runSystemCheckPipelineWithOutcome(nil, nil, nil, "op-test", stdcontext.Background())
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	if v, ok := outcome["ingest_stage_started"]; !ok || v != true {
		t.Fatalf("expected ingest_stage_started=true, got %v (ok=%v)", v, ok)
	}
	if outcome["ingest_error"] != "nil cmd" {
		t.Fatalf("expected ingest_error='nil cmd', got %v", outcome["ingest_error"])
	}
}
