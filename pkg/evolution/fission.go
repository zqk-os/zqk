package evolution

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/logging"

	"github.com/lanceman/zqk/pkg/infrastructure"
	"github.com/lanceman/zqk/pkg/infrastructure/crypto"
	"github.com/lanceman/zqk/pkg/metrics"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

// FissionController manages the autonomous division of a kernel node.
type FissionController struct {
	store  storage.ObjectStorageProvider
	spine  infrastructure.SpinalSpine
	signer crypto.Signer

	mu              sync.Mutex
	lastFissionTime time.Time
	isDividing      bool
}

// NewFissionController creates a new FissionController.
func NewFissionController(store storage.ObjectStorageProvider, spine infrastructure.SpinalSpine, signer crypto.Signer) *FissionController {
	return &FissionController{
		store:  store,
		spine:  spine,
		signer: signer,
	}
}

// Monitor checks resource availability and triggers fission if saturation thresholds are met.
func (c *FissionController) Monitor(ctx context.Context) error {
	res := metrics.GetResourceAvailability()

	// Autonomous Governance: Trigger fission if saturation > 85% and system integrity (PCS) > 80
	vitality, err := c.store.Read(ctx, nil, "system_vitality")
	if err != nil {
		return nil // Waiting for vitality signals
	}

	pcs, _ := vitality[objects.FieldKeyProjectConfidenceScore].(int)

	computeSaturation := float64(res.UsedGoroutines) / float64(res.TotalGoroutines)

	if computeSaturation > 0.85 && pcs > 80 {
		return c.Trigger(ctx)
	}

	return nil
}

// Trigger initiates the fission protocol.
func (c *FissionController) Trigger(ctx context.Context) error {
	c.mu.Lock()
	if c.isDividing {
		c.mu.Unlock()
		return nil
	}
	c.isDividing = true
	c.mu.Unlock()

	logging.FluentEvent(logging.GetLogger()).Info("[FISSION] Autonomous resource saturation detected. Initiating Cellular Fission...").Log()

	// 1. Create State Snapshot
	snapshotID := fmt.Sprintf("SNP-%d", time.Now().Unix())

	event := infrastructure.Event{
		ObjectID: snapshotID,
		Kind:     "fission_event",
		Op:       "initiated",
		Payload: map[string]any{
			objects.FieldKeyParentNodeID: "kernel-001",
			objects.FieldKeySnapshotID:   snapshotID,
			objects.FieldKeyReason:       "autonomous_saturation_fission",
		},
	}

	// 2. Sign Pulse
	sigStatus := "unsigned"
	if c.signer != nil {
		sig, sigErr := c.signer.Sign([]byte(fmt.Sprintf("fission:%s", snapshotID)))
		if sigErr == nil {
			sigStatus = "signed:" + sig[:8]
		}
	}

	// 3. Publish to Spine
	if err := c.spine.Publish(ctx, event); err != nil {
		return err
	}

	logging.FluentEvent(logging.GetLogger()).Info(fmt.Sprintf("⚡ [HIVE PULSE] [fission] [%s] kernel-001: Initiated autonomous cellular division to node %s\n", sigStatus, snapshotID)).Log()
	logging.FluentEvent(logging.GetLogger()).Info(fmt.Sprintf("[FISSION] Fission event %s published. Waiting for child peer registration...\n", snapshotID)).Log()

	c.mu.Lock()
	c.lastFissionTime = time.Now()
	c.isDividing = false
	c.mu.Unlock()

	return nil
}
