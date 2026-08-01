package convergence

import (
	"context"
	"testing"

	"github.com/lanceman/zqk/pkg/pipeline"
)

func TestBuildSelfCorrectionPipeline_NoDrift(t *testing.T) {
	p := BuildSelfCorrectionPipeline(nil)

	payload := &SelfCorrectionPayload{
		PlanID:        "PLAN-123",
		DriftDetected: false,
	}

	ctx := &pipeline.Context{Ctx: context.Background()}
	res, err := p.Run(ctx, payload)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	outPayload, ok := res.(*SelfCorrectionPayload)
	if !ok {
		t.Fatalf("expected *SelfCorrectionPayload, got %T", res)
	}

	if outPayload.PlanID != "PLAN-123" {
		t.Errorf("expected PlanID PLAN-123, got %s", outPayload.PlanID)
	}
}

func TestBuildSelfCorrectionPipeline_DriftDetected(t *testing.T) {
	p := BuildSelfCorrectionPipeline(nil)

	payload := &SelfCorrectionPayload{
		PlanID:        "PLAN-123",
		DriftDetected: true,
	}

	ctx := &pipeline.Context{Ctx: context.Background()}
	res, err := p.Run(ctx, payload)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	outPayload, ok := res.(*SelfCorrectionPayload)
	if !ok {
		t.Fatalf("expected *SelfCorrectionPayload, got %T", res)
	}

	if outPayload.PlanID != "PLAN-123" {
		t.Errorf("expected PlanID PLAN-123, got %s", outPayload.PlanID)
	}

	if ctx.Outcome["resources_reallocated"] != true {
		t.Errorf("expected resources_reallocated to be true, got %v", ctx.Outcome["resources_reallocated"])
	}
	if ctx.Outcome["plan_updated"] != true {
		t.Errorf("expected plan_updated to be true, got %v", ctx.Outcome["plan_updated"])
	}
}
