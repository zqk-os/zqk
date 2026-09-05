package cas

import (
	"context"
	"testing"
	"time"
)

func TestListingIndexCallback_LifetimeCounters(t *testing.T) {
	rInit, tInit := GetListingIndexCallbackStats()

	// Register a dummy callback
	dummyCallback := func(
		ctx context.Context,
		projectRoot string,
		storageProvider CASFacade,
		kind string,
		batchSize int,
		duration time.Duration,
		status string,
		err error,
	) {
	}

	SetListingIndexBatchEventCallback(dummyCallback)

	rAfterReg, _ := GetListingIndexCallbackStats()
	if rAfterReg <= rInit {
		t.Errorf("expected registered counter to increment, got rInit=%d rAfterReg=%d", rInit, rAfterReg)
	}

	// Trigger callback lookup
	cb := getListingIndexBatchEventCallback()
	if cb == nil {
		t.Fatal("expected non-nil callback")
	}

	_, tFinal := GetListingIndexCallbackStats()
	if tFinal <= tInit {
		t.Errorf("expected triggered counter to increment, got tInit=%d tFinal=%d", tInit, tFinal)
	}
}
