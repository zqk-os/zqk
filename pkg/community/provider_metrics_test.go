package community

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestProviderMetrics_Aggregation(t *testing.T) {
	cfg := DefaultMetricsCollectorConfig()
	collector := NewProviderMetricsCollector(cfg)

	// Record 3 successful invocations
	for i := 0; i < 3; i++ {
		state := collector.RecordExecution(ProviderExecutionMetric{
			ProviderID: "provider-metrics-test",
			Duration:   10 * time.Millisecond,
			Err:        nil,
			Timestamp:  time.Now(),
		})
		if state != HealthStateHealthy {
			t.Fatalf("expected state Healthy, got %v", state)
		}
	}

	summary, ok := collector.GetSummary("provider-metrics-test")
	if !ok {
		t.Fatalf("expected summary to exist")
	}

	if summary.TotalCalls != 3 || summary.SuccessfulCalls != 3 || summary.FailedCalls != 0 {
		t.Fatalf("unexpected summary counts: %+v", summary)
	}

	if summary.ErrorRate != 0.0 {
		t.Fatalf("expected 0.0 error rate, got %f", summary.ErrorRate)
	}
}

func TestProviderMetrics_AutomaticQuarantine(t *testing.T) {
	cfg := ProviderMetricsCollectorConfig{
		MaxErrorRateThreshold: 0.50,
		MinSamplesForAnomaly:  4,
		LatencyWarningCutoff:  200 * time.Millisecond,
	}
	collector := NewProviderMetricsCollector(cfg)

	dummyErr := errors.New("upstream provider fault")

	// 1 success
	collector.RecordExecution(ProviderExecutionMetric{
		ProviderID: "flaky-provider",
		Duration:   20 * time.Millisecond,
		Err:        nil,
		Timestamp:  time.Now(),
	})

	// 3 failures
	var finalState ProviderHealthState
	for i := 0; i < 3; i++ {
		finalState = collector.RecordExecution(ProviderExecutionMetric{
			ProviderID: "flaky-provider",
			Duration:   30 * time.Millisecond,
			Err:        dummyErr,
			Timestamp:  time.Now(),
		})
	}

	// 3/4 failures = 75% error rate >= 50% threshold with 4 samples >= 4 min samples
	if finalState != HealthStateQuarantined {
		t.Fatalf("expected state Quarantined, got %v", finalState)
	}

	summary, _ := collector.GetSummary("flaky-provider")
	if summary.HealthState != HealthStateQuarantined {
		t.Fatalf("expected summary health state Quarantined, got %v", summary.HealthState)
	}
}

func TestProviderMetrics_QuarantineEnforcementAndRecovery(t *testing.T) {
	cfg := ProviderMetricsCollectorConfig{
		MaxErrorRateThreshold: 0.50,
		MinSamplesForAnomaly:  2,
		LatencyWarningCutoff:  200 * time.Millisecond,
	}
	collector := NewProviderMetricsCollector(cfg)

	ctx := context.Background()
	providerID := "quarantined-provider"

	// Initially allowed
	if err := collector.CheckInvocationAllowed(ctx, providerID); err != nil {
		t.Fatalf("expected initial check to pass, got: %v", err)
	}

	// Trigger quarantine with 2 failures
	collector.RecordExecution(ProviderExecutionMetric{
		ProviderID: providerID,
		Duration:   10 * time.Millisecond,
		Err:        errors.New("fail 1"),
		Timestamp:  time.Now(),
	})
	collector.RecordExecution(ProviderExecutionMetric{
		ProviderID: providerID,
		Duration:   10 * time.Millisecond,
		Err:        errors.New("fail 2"),
		Timestamp:  time.Now(),
	})

	// Now check must fail with ErrProviderQuarantined
	err := collector.CheckInvocationAllowed(ctx, providerID)
	if !errors.Is(err, ErrProviderQuarantined) {
		t.Fatalf("expected ErrProviderQuarantined, got: %v", err)
	}

	// Reset quarantine
	collector.ResetQuarantine(providerID)

	// Now check must pass again
	if err := collector.CheckInvocationAllowed(ctx, providerID); err != nil {
		t.Fatalf("expected check to pass after quarantine reset, got: %v", err)
	}
}

func TestProviderMetrics_ContextCancellation(t *testing.T) {
	collector := NewProviderMetricsCollector(DefaultMetricsCollectorConfig())
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // canceled immediately

	err := collector.CheckInvocationAllowed(ctx, "any-provider")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}
