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

func TestCompileProductDeltaGutCheck_ProcessOnlyResidualIsTheaterRisk(t *testing.T) {
	v := CompileProductDeltaGutCheck(ProductDeltaSignals{
		WindowCommits:      5,
		ProcessOnlyCommits: 5,
		ProductCommits:     0,
		ResidualMinted:     2,
	})
	if !v.TheaterRisk {
		t.Fatalf("process-only residual mint must be theater risk, got %+v", v)
	}
}

func TestCompileProductDeltaGutCheck_ProductCommitsPass(t *testing.T) {
	v := CompileProductDeltaGutCheck(ProductDeltaSignals{
		WindowCommits:  4,
		ProductCommits: 2,
	})
	if v.TheaterRisk || !v.ProductDelta {
		t.Fatalf("product commits must pass gut-check, got %+v", v)
	}
}

func TestCompileProductDeltaGutCheck_RedGateWithCodeFixPasses(t *testing.T) {
	v := CompileProductDeltaGutCheck(ProductDeltaSignals{WindowCommits: 3, RedGates: 1})
	if v.TheaterRisk || !v.ProductDelta || !v.Capability {
		t.Fatalf("red gate with code fix must pass, got %+v", v)
	}
}

func TestCompileProductDeltaGutCheck_UndeliveredProductREQPassesCapability(t *testing.T) {
	v := CompileProductDeltaGutCheck(ProductDeltaSignals{
		WindowCommits:         3,
		UndeliveredProductREQ: 1,
	})
	if v.TheaterRisk || !v.Capability {
		t.Fatalf("undelivered product REQ runway must pass capability, got %+v", v)
	}
}

func TestCompileProductDeltaGutCheck_AuditLeadAssignedExempt(t *testing.T) {
	v := CompileProductDeltaGutCheck(ProductDeltaSignals{
		WindowCommits:      3,
		ProcessOnlyCommits: 3,
		ResidualMinted:     1,
		AuditLeadAssigned:  true,
	})
	if v.TheaterRisk || !v.AuditExempt {
		t.Fatalf("named audit lead must exempt residual seals, got %+v", v)
	}
}

func TestCompileProductDeltaGutCheck_ResidualMintedWithZeroProductCommitsForcesRisk(t *testing.T) {
	// Even with undelivered REQ runway, residual mint + zero product commits is theater.
	v := CompileProductDeltaGutCheck(ProductDeltaSignals{
		WindowCommits:         6,
		ProductCommits:        0,
		ResidualMinted:        1,
		UndeliveredProductREQ: 2,
	})
	if !v.TheaterRisk {
		t.Fatalf("residual minted with zero product commits must force theater risk, got %+v", v)
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

func TestAppendGutCheck_PreservesAmbientSuffix(t *testing.T) {
	drive := AppendStratplanAmbient("shape next column", StratplanAmbient{LeadID: "PRI-1", LeadStatus: "in_progress"})
	got := AppendGutCheck(drive, GutCheckVerdict{TheaterRisk: true})
	if !strings.Contains(got, GutCheckStopProse) {
		t.Fatalf("missing stop prose: %q", got)
	}
	if !strings.Contains(got, "Ambient:") || !strings.Contains(got, "product_delta_gut_check=required") {
		t.Fatalf("must keep Ambient suffix for greppability: %q", got)
	}
	if AppendGutCheck("keep me", GutCheckVerdict{}) != "keep me" {
		t.Fatal("zero gut-check must leave drive unchanged")
	}
}

func TestCompileStratplanCommandHint_TheaterRiskPreemptsAlignRefresh(t *testing.T) {
	hint := CompileStratplanCommandHint(StratplanAmbient{
		GutCheck: GutCheckVerdict{TheaterRisk: true},
	})
	if hint != HintProductReplenish() {
		t.Fatalf("hint=%q want product replenish", hint)
	}
	if CommandHintIsDispatchRemint(hint) {
		t.Fatalf("must not be dispatch remint: %q", hint)
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
