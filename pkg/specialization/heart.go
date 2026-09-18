package specialization

import (
	"context"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/infrastructure"
	"github.com/zqk-os/zqk/pkg/metrics"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// HeartHandler monitors project vitality and maintains the Project Confidence Score (PCS).
type HeartHandler struct {
	store storage.ObjectStorageProvider
	spine infrastructure.SpinalSpine

	mu             sync.Mutex
	successRate    float64
	driftFrequency float64
	lastUpdate     time.Time
}

func (h *HeartHandler) Initialize(ctx context.Context, store storage.ObjectStorageProvider, spine infrastructure.SpinalSpine) error {
	h.store = store
	h.spine = spine
	h.successRate = 1.0 // Start with high confidence
	return nil
}

func (h *HeartHandler) Start(ctx context.Context) error {
	// Subscribe to operational events that influence vitality
	if err := h.spine.Subscribe(ctx, objects.KindAuditEvent, h.handleAuditEvent); err != nil {
		return err
	}
	return h.spine.Subscribe(ctx, "metrics", h.handleMetricEvent)
}

func (h *HeartHandler) handleAuditEvent(ctx context.Context, event infrastructure.Event) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	op := event.Op
	// Update rolling averages based on event types
	// These are simplified heuristics for the prototype
	switch op {
	case "convergence_success":
		h.successRate = (h.successRate * 0.95) + 0.05
	case "drift_detected":
		h.driftFrequency = (h.driftFrequency * 0.9) + 0.1
	}

	return h.updateVitalityReport(ctx)
}

func (h *HeartHandler) handleMetricEvent(ctx context.Context, event infrastructure.Event) error {
	// Placeholder for deeper metric analysis (CPU spikes, IO latency)
	return nil
}

func (h *HeartHandler) updateVitalityReport(ctx context.Context) error {
	// Throttling updates to once per 5 seconds in production, 1s for prototype
	if !h.lastUpdate.IsZero() && time.Since(h.lastUpdate) < time.Second {
		return nil
	}

	pcs := h.calculatePCS()
	report := map[string]any{
		objects.FieldKeyKind:                   objects.KindVitalityReport,
		objects.FieldKeyID:                     "system_vitality",
		objects.FieldKeyProjectConfidenceScore: pcs,
		objects.FieldKeySuccessRate:            h.successRate,
		objects.FieldKeyDriftFrequency:         h.driftFrequency,
		objects.FieldKeyLastCalculationAt:      zqktime.NowRFC3339UTC(),
	}

	h.lastUpdate = time.Now()

	// Try update first, then create if not found
	err := h.store.Update(ctx, nil, "system_vitality", report)
	if err != nil {
		return h.store.Create(ctx, nil, report)
	}
	return nil
}

func (h *HeartHandler) calculatePCS() int {
	res := metrics.GetResourceAvailability()

	// 1. Success Rate (0.0 to 1.0)
	// 2. Drift Frequency (0.0 to 1.0)

	// Goroutine budget: AvailableCompute / TotalGoroutines
	budgetPercent := 1.0
	if res.TotalGoroutines > 0 {
		budgetPercent = float64(res.AvailableCompute) / float64(res.TotalGoroutines)
	}

	// Memory saturation: 2GB soft limit for prototype
	const softMemoryLimit = 2 * 1024 * 1024 * 1024
	memSaturation := float64(res.MemoryAllocBytes) / float64(softMemoryLimit)
	if memSaturation > 1.0 {
		memSaturation = 1.0
	}

	goroutineSaturation := 1.0 - budgetPercent
	resourceSaturation := goroutineSaturation
	if memSaturation > resourceSaturation {
		resourceSaturation = memSaturation
	}

	// Heuristic: Weighted average of success (60%), drift (20%), resource saturation (20%)
	score := (h.successRate * 60) + ((1.0 - h.driftFrequency) * 20) + ((1.0 - resourceSaturation) * 20)

	// CRITICAL: If goroutine budget < 10%, PCS MUST drop below 50 regardless of success rate
	if budgetPercent < 0.10 && score >= 50 {
		score = 49
	}

	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}
	return int(score)
}

func (h *HeartHandler) Stop() error {
	return nil
}
