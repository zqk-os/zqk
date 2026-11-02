package testdrive

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTestDrive_StaticFloorInvariant verifies defaults, option constraints, and length boundaries.
func TestTestDrive_StaticFloorInvariant(t *testing.T) {
	t.Parallel()

	// Default instantiation
	eng, err := NewEngine(TestDriveOptions{
		Speed:          SpeedInstant,
		NonInteractive: true,
	})
	require.NoError(t, err)
	assert.NotNil(t, eng)
	assert.Equal(t, ScenarioAPIService, eng.opts.Scenario)
	assert.Equal(t, time.Duration(0), eng.delay)

	// Speed pacing verification
	engFast, err := NewEngine(TestDriveOptions{
		Speed: SpeedFast,
	})
	require.NoError(t, err)
	assert.Equal(t, FastStepDelay, engFast.delay)

	engNormal, err := NewEngine(TestDriveOptions{
		Speed: SpeedNormal,
	})
	require.NoError(t, err)
	assert.Equal(t, NormalStepDelay, engNormal.delay)

	// Minimum intent length boundary invariant (< 3 chars)
	_, errTooShort := NewEngine(TestDriveOptions{
		Intent: "ab",
	})
	assert.Error(t, errTooShort)
	assert.Contains(t, errTooShort.Error(), "intent too short")

	// Maximum intent length boundary invariant (> 512 chars)
	hugeIntent := strings.Repeat("A", MaxIntentLength+1)
	_, errTooLong := NewEngine(TestDriveOptions{
		Intent: hugeIntent,
	})
	assert.Error(t, errTooLong)
	assert.Contains(t, errTooLong.Error(), "intent exceeds maximum length")
}

// TestTestDrive_OperationalProof_EndToEndSimulation verifies complete 4-stage execution.
func TestTestDrive_OperationalProof_EndToEndSimulation(t *testing.T) {
	t.Parallel()

	scenarios := []struct {
		name     string
		scenario string
		intent   string
	}{
		{
			name:     "Default REST API Service",
			scenario: ScenarioAPIService,
			intent:   "",
		},
		{
			name:     "CLI Utility Scenario",
			scenario: ScenarioCLITool,
			intent:   "",
		},
		{
			name:     "Streaming Pipeline Scenario",
			scenario: ScenarioStreamPipeline,
			intent:   "",
		},
		{
			name:     "Custom Enterprise Intent",
			scenario: ScenarioCustom,
			intent:   "Architect an enterprise distributed knowledge mesh with shockwave routing",
		},
	}

	for _, tc := range scenarios {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			eng, err := NewEngine(TestDriveOptions{
				Scenario:       tc.scenario,
				Intent:         tc.intent,
				Speed:          SpeedInstant,
				NonInteractive: true,
				Writer:         &out,
			})
			require.NoError(t, err)

			ctx := context.Background()
			report, err := eng.Run(ctx)
			require.NoError(t, err)
			require.NotNil(t, report)

			assert.True(t, report.Success)
			assert.NotEmpty(t, report.Intent)
			assert.Len(t, report.Stages, 4)
			assert.Len(t, report.MintedObjects, 8)
			assert.Len(t, report.SwarmWorkers, DefaultSwarmSeatCount)
			assert.NotEmpty(t, report.VDSReceiptHash)
			assert.NotEmpty(t, report.NextSteps)

			outputStr := out.String()
			assert.Contains(t, outputStr, "ZQK KNOWLEDGE KERNEL — DEVELOPER SWARM TEST-DRIVE")
			assert.Contains(t, outputStr, "[Stage 1/4] Seed Planting")
			assert.Contains(t, outputStr, "[Stage 2/4] Intent Crystallization")
			assert.Contains(t, outputStr, "[Stage 3/4] Swarm Execution")
			assert.Contains(t, outputStr, "[Stage 4/4] Cryptographic Verification Receipt")
			assert.Contains(t, outputStr, "TEST-DRIVE COMPLETED SUCCESSFULLY")
		})
	}
}

// TestTestDrive_NegativeBoundary_AdversarialScenarios verifies defensive handling of edge inputs.
func TestTestDrive_NegativeBoundary_AdversarialScenarios(t *testing.T) {
	t.Parallel()

	// Interactive mode with custom reader supplying a valid intent
	inputBuf := bytes.NewBufferString("Build a zero-trust audit mesh\n")
	var outBuf bytes.Buffer

	eng, err := NewEngine(TestDriveOptions{
		Speed:          SpeedInstant,
		NonInteractive: false,
		Reader:         inputBuf,
		Writer:         &outBuf,
	})
	require.NoError(t, err)

	report, err := eng.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "Build a zero-trust audit mesh", report.Intent)

	// Interactive mode with empty reader (falls back to default prompt)
	emptyReader := bytes.NewBufferString("\n")
	var outBuf2 bytes.Buffer

	eng2, err := NewEngine(TestDriveOptions{
		Speed:          SpeedInstant,
		NonInteractive: false,
		Reader:         emptyReader,
		Writer:         &outBuf2,
	})
	require.NoError(t, err)

	report2, err := eng2.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, DefaultIntentPrompt, report2.Intent)

	// Interactive mode with input too short in scanner
	shortReader := bytes.NewBufferString("no\n")
	var outBuf3 bytes.Buffer

	eng3, err := NewEngine(TestDriveOptions{
		Speed:          SpeedInstant,
		NonInteractive: false,
		Reader:         shortReader,
		Writer:         &outBuf3,
	})
	require.NoError(t, err)

	_, errShort := eng3.Run(context.Background())
	assert.Error(t, errShort)
	assert.Contains(t, errShort.Error(), "intent too short")

	// Interactive mode with input too long in scanner
	longReader := bytes.NewBufferString(strings.Repeat("Z", MaxIntentLength+5) + "\n")
	var outBuf4 bytes.Buffer

	eng4, err := NewEngine(TestDriveOptions{
		Speed:          SpeedInstant,
		NonInteractive: false,
		Reader:         longReader,
		Writer:         &outBuf4,
	})
	require.NoError(t, err)

	_, errLong := eng4.Run(context.Background())
	assert.Error(t, errLong)
	assert.Contains(t, errLong.Error(), "intent exceeds maximum length")
}
