package cef

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestEvidenceLift_TST elevates TST (Test) evidence toward envelope 5
func TestEvidenceLift_TST(t *testing.T) {
	lifted := EvidenceTier{
		Test:     3.5,
		Receive:  3.0,
		Security: 2.8,
	}

	newState := lifted.LiftEvidence()

	assert.True(t, newState.Test > lifted.Test, "TST evidence must lift after elevation")
	assert.Equal(t, EnvelopeFloor5, newState.CurrentLevel, "post-lift envelope must be at least Floor5")
}

// TestEvidenceLift_RCV elevates RCV (Receive) evidence toward envelope 5
func TestEvidenceLift_RCV(t *testing.T) {
	lifted := EvidenceTier{
		Test:     3.8,
		Receive:  2.5,
		Security: 3.0,
	}

	newState := lifted.LiftEvidence()

	assert.True(t, newState.Receive > lifted.Receive, "RCV evidence must lift after elevation")
	assert.Equal(t, EnvelopeFloor5, newState.CurrentLevel, "post-lift envelope must be at least Floor5")
}

// TestEvidenceLift_SEC elevates SEC (Security) evidence toward envelope 5
func TestEvidenceLift_SEC(t *testing.T) {
	lifted := EvidenceTier{
		Test:     4.0,
		Receive:  3.5,
		Security: 2.2,
	}

	newState := lifted.LiftEvidence()

	assert.True(t, newState.Security > lifted.Security, "SEC evidence must lift after elevation")
	assert.Equal(t, EnvelopeFloor5, newState.CurrentLevel, "post-lift envelope must be at least Floor5")
}

// TestEvidenceLift_allDims elevates all three evidence dimensions together
func TestEvidenceLift_allDims(t *testing.T) {
	lifted := EvidenceTier{
		Test:     3.0,
		Receive:  3.0,
		Security: 3.0,
	}

	newState := lifted.LiftEvidence()

	assert.True(t, newState.Test > lifted.Test)
	assert.True(t, newState.Receive > lifted.Receive)
	assert.True(t, newState.Security > lifted.Security)
	assert.Equal(t, EnvelopeFloor5, newState.CurrentLevel)
}

// TestEvidenceLift_alreadyFloor5 does not double-lift when already at Floor 5
func TestEvidenceLift_alreadyFloor5(t *testing.T) {
	lifted := EvidenceTier{
		Test:     4.8,
		Receive:  4.7,
		Security: 4.9,
	}

	newState := lifted.LiftEvidence()

	assert.Equal(t, EnvelopeFloor5, newState.CurrentLevel)
	assert.InDelta(t, 4.9, newState.Security, 0.11, "SEC near-ceiling should barely adjust")
}
