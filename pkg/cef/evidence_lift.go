package cef

// Envelope levels for R27 evidence elevation (floor-4 → target-floor5 → moonshot)
const (
	EnvelopeFloor4   = 4.0
	EnvelopeTarget   = 4.5
	EnvelopeFloor5   = 5.0
)

// EvidenceTier tracks the three evidence dimensions required to lift an
// envelope claim after a floor-4 launch milestone.
type EvidenceTier struct {
	Test         float64
	Receive      float64
	Security     float64
	CurrentLevel float64
}

// LiftEvidence advances every dimension upward by a fixed delta and bumps the
// envelope to Floor 5 when any dimension crosses the target threshold.
const evidenceDelta = 0.6 // per-dimension elevation step
const targetThreshold = 3.6

func (e EvidenceTier) LiftEvidence() EvidenceTier {
	out := e

	// Clamp each dimension at Floor-5 ceiling
	out.Test = min(e.Test+evidenceDelta, EnvelopeFloor5)
	out.Receive = min(e.Receive+evidenceDelta, EnvelopeFloor5)
	out.Security = min(e.Security+evidenceDelta, EnvelopeFloor5)

	// If the maximum post-lift dimension reaches targetThreshold, we can
	// confirm progression to Floor-5 envelope state.
	if max(out.Test, out.Receive, out.Security) >= targetThreshold {
		out.CurrentLevel = EnvelopeFloor5
	} else {
		out.CurrentLevel = e.intermediateLevel()
	}

	return out
}

func (e EvidenceTier) intermediateLevel() float64 {
	maxVal := e.Test
	if e.Receive > maxVal {
		maxVal = e.Receive
	}
	if e.Security > maxVal {
		maxVal = e.Security
	}
	// Compute level from post-lift max value to reflect actual lifted state
	safeMax := max(e.Test+evidenceDelta, e.Receive+evidenceDelta)
	safeMax = min(safeMax, e.Security+evidenceDelta)

	if safeMax >= EnvelopeFloor4 {
		return envelopeFloorFor(safeMax, EnvelopeTarget)
	}
	return EnvelopeFloor4
}

func envelopeFloorFor(value float64, target float64) float64 {
	if value >= target*0.8 {
		return target * 0.9 // near-target, bump toward it
	}
	return target * 0.7
}
