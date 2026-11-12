package interactionpolicy

import (
	"strings"
	"testing"
)

func TestCompileProductDeltaGutCheck_LeadTitleTheater(t *testing.T) {
	v := CompileProductDeltaGutCheck(ProductDeltaSignals{LeadTitleTheater: true})
	if !v.TheaterRisk {
		t.Fatalf("expected theater risk for residual lead title, got %+v", v)
	}
	v = CompileProductDeltaGutCheck(ProductDeltaSignals{LeadTitleTheater: true, HumanAuditRequested: true})
	if v.TheaterRisk {
		t.Fatalf("audit exempt should clear theater risk, got %+v", v)
	}
}

func TestCompileProductDeltaGutCheck_UnknownWindowFailOpen(t *testing.T) {
	v := CompileProductDeltaGutCheck(ProductDeltaSignals{})
	if v.TheaterRisk {
		t.Fatalf("unknown signals must fail-open, got %+v", v)
	}
}

func TestTitleLooksLikeResidualTheater(t *testing.T) {
	if !TitleLooksLikeResidualTheater("Grooming — Stranger doctor residual (Post-3386)") {
		t.Fatal("expected residual title match")
	}
	if TitleLooksLikeResidualTheater("Product delta + operational alignment") {
		t.Fatal("product title must not match")
	}
}

func TestCompileTurnContinue_GutCheckStopsResidualFill(t *testing.T) {
	got := CompileTurnContinue(TurnContinueInput{
		CapLoopActive:         true,
		OutboxAwaitingPeerAck: 1,
		GuidingEvent:          EventStratplanAhead,
		GuidingStep:           "shape next residual column",
		FillItemCommandHint:   "object draft sweep --dry-run",
		GutCheck:              CompileProductDeltaGutCheck(ProductDeltaSignals{LeadTitleTheater: true}),
	})
	if !got.Continue {
		t.Fatal("expected continue")
	}
	if got.Reason != reasonProductDeltaStop {
		t.Fatalf("reason=%q want %q", got.Reason, reasonProductDeltaStop)
	}
	if !strings.Contains(got.Message, "STOP residual") || !strings.Contains(got.Message, PolicyCAPProductAlignment) {
		t.Fatalf("message missing stop prose: %q", got.Message)
	}
	if !strings.Contains(got.Key, "gc=1") {
		t.Fatalf("key must include gc=1, got %q", got.Key)
	}
}
