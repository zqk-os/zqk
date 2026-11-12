package interactionpolicy

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/strutil"
)

// ProductDeltaSignals are kernel facts computed by whats-next / continuation hook.
// Zero WindowCommits means unknown → never force theater stop (fail-open on missing data).
type ProductDeltaSignals struct {
	WindowCommits         int
	ProductCommits        int
	ProcessOnlyCommits    int
	ResidualMinted        int
	RedGates              int
	UndeliveredProductREQ int
	HumanAuditRequested   bool
	AuditLeadAssigned     bool
	// LeadTitleTheater is true when the seated lead priority_plan title matches
	// residual/reaffirm theater (cheap signal for CAP stop-hook without git I/O).
	LeadTitleTheater bool
}

// GutCheckVerdict is the POL-AGENT-CAP-PRODUCT-ALIGNMENT-001 three-question result.
type GutCheckVerdict struct {
	ProductDelta bool
	Capability   bool
	AuditExempt  bool
	TheaterRisk  bool
	Reason       string
}

// GutCheckStopProse is the CAP drive when theater risk is true (≤~350 runes; whole drive clips at 600).
const GutCheckStopProse = "Product-delta gut-check FAILED (POL-AGENT-CAP-PRODUCT-ALIGNMENT-001): recent work did not change cmd/pkg/stranger-facing behavior and no red gate has a code fix. STOP residual/reaffirmation mint. Mint an undelivered product REQ (gen-trace-pipeline), triage a failing gate with code, or convene the Strategic Planning Meeting Team. Do not seal PASS residuals."

// CompileProductDeltaGutCheck answers the three policy questions.
// Policy reference: POL-AGENT-CAP-PRODUCT-ALIGNMENT-001
func CompileProductDeltaGutCheck(s ProductDeltaSignals) GutCheckVerdict {
	q1 := s.ProductCommits > 0 || s.RedGates > 0
	q2 := s.UndeliveredProductREQ > 0 || s.RedGates > 0
	q3 := s.HumanAuditRequested || s.AuditLeadAssigned
	v := GutCheckVerdict{
		ProductDelta: q1,
		Capability:   q2,
		AuditExempt:  q3,
	}
	if s.WindowCommits == 0 && !s.LeadTitleTheater && s.ResidualMinted == 0 {
		// Unknown window and no cheap theater signal → fail-open (do not silence CAP).
		v.Reason = "signals_unknown"
		return v
	}
	theater := !q1 && !q2 && !q3
	if s.ResidualMinted > 0 && s.ProductCommits == 0 && !q3 {
		theater = true
	}
	if s.LeadTitleTheater && !q3 {
		theater = true
	}
	v.TheaterRisk = theater
	if theater {
		v.Reason = "theater_risk"
	} else {
		v.Reason = "pass"
	}
	return v
}

// TitleLooksLikeResidualTheater reports residual/reaffirm PASS-seal column titles.
func TitleLooksLikeResidualTheater(title string) bool {
	if strutil.IsBlank(title) {
		return false
	}
	low := strings.ToLower(strings.TrimSpace(title))
	for _, tok := range []string{"residual", "reaffirm", "re-seal", "pass seal", "ship-exit residual"} {
		if strings.Contains(low, tok) {
			return true
		}
	}
	return false
}

// HintProductReplenish points at the CAP anti-theater policy (no remint-shaped steer).
func HintProductReplenish() string {
	return paths.CLIInvocation("object get " + PolicyCAPProductAlignment)
}

// AppendGutCheck replaces drive with stop prose when TheaterRisk; otherwise returns drive unchanged.
func AppendGutCheck(drive string, v GutCheckVerdict) string {
	if !v.TheaterRisk {
		return strings.TrimSpace(drive)
	}
	return GutCheckStopProse
}
