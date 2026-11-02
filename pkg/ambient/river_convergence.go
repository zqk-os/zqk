package ambient

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// Named constants enforcing bounded resource allocation and deterministic convergence.
const (
	DefaultIntentQueueCapacity = 2048
	DefaultRiverCapacity       = 512
	DefaultConvergenceWindow   = 50 * time.Millisecond
	DefaultMaxActiveStreams    = 128
	DefaultMinConsensusDrops   = 3
	DefaultAlignmentThreshold  = 0.70
)

var (
	ErrNilDrop               = errors.New("intent drop cannot be nil")
	ErrEmptyMissionID        = errors.New("mission id cannot be empty")
	ErrEngineClosed          = errors.New("river convergence engine is closed")
	ErrStreamCapacityReached = errors.New("maximum active intent streams reached")
	ErrDimensionMismatch     = errors.New("intent vector dimension mismatch")
)

// IntentDrop represents an atomic raindrop of user/agent intent originating from a seed.
type IntentDrop struct {
	ID           string            `json:"id"`
	Source       string            `json:"source"`
	MissionID    string            `json:"mission_id"`
	IntentVector []float64         `json:"intent_vector"`
	Timestamp    time.Time         `json:"timestamp"`
	Weight       float64           `json:"weight"`
	Tags         map[string]string `json:"tags,omitempty"`
}

// IntentRiver represents a pooled stream of intention that has merged into a cohesive directional river.
type IntentRiver struct {
	ID             string    `json:"id"`
	RootVisionID   string    `json:"root_vision_id"`
	ActiveMissions []string  `json:"active_missions"`
	PooledVector   []float64 `json:"pooled_vector"`
	AlignmentScore float64   `json:"alignment_score"`
	TotalWeight    float64   `json:"total_weight"`
	DropCount      int       `json:"drop_count"`
	FormedAt       time.Time `json:"formed_at"`
	LastConverged  time.Time `json:"last_converged"`
	Status         string    `json:"status"` // "forming", "surging", "diverging", "quiescent"
}

// ConvergenceMetrics captures operational and test-drive statistics.
type ConvergenceMetrics struct {
	TotalDropsIngested int64     `json:"total_drops_ingested"`
	TotalRiversFormed  int64     `json:"total_rivers_formed"`
	DivergenceWarnings int64     `json:"divergence_warnings"`
	AverageAlignment   float64   `json:"average_alignment"`
	ActiveRiversCount  int       `json:"active_rivers_count"`
	LastConvergenceAt  time.Time `json:"last_convergence_at"`
}

// DivergenceHandler defines an optional hook called when a stream diverges from root vision.
type DivergenceHandler func(riverID, missionID string, alignmentScore float64)

// RiverConfig provides configurable parameters for river convergence.
type RiverConfig struct {
	RootVisionID       string
	RootVisionVector   []float64
	QueueCapacity      int
	RiverCapacity      int
	ConvergenceWindow  time.Duration
	MinConsensusDrops  int
	AlignmentThreshold float64
	OnDivergence       DivergenceHandler
}

// DefaultRiverConfig returns a production-ready default configuration.
func DefaultRiverConfig(rootVisionID string, rootVisionVector []float64) RiverConfig {
	return RiverConfig{
		RootVisionID:       rootVisionID,
		RootVisionVector:   rootVisionVector,
		QueueCapacity:      DefaultIntentQueueCapacity,
		RiverCapacity:      DefaultRiverCapacity,
		ConvergenceWindow:  DefaultConvergenceWindow,
		MinConsensusDrops:  DefaultMinConsensusDrops,
		AlignmentThreshold: DefaultAlignmentThreshold,
	}
}

// RiverConvergenceEngine coordinates discrete raindrops and streams into pooled rivers of intention.
type RiverConvergenceEngine struct {
	cfg    RiverConfig
	inbox  chan IntentDrop
	rivers map[string]*IntentRiver
	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	closed atomic.Bool

	// Metrics
	dropsIngested        atomic.Int64
	riversFormed         atomic.Int64
	divergenceWarnings   atomic.Int64
	totalAlignmentSum    float64
	alignmentEvaluations int64
}

// NewRiverConvergenceEngine initializes and starts an Intent River Convergence Engine holon.
func NewRiverConvergenceEngine(ctx context.Context, cfg RiverConfig) (*RiverConvergenceEngine, error) {
	if cfg.RootVisionID == "" {
		return nil, errors.New("root vision id is required")
	}
	if len(cfg.RootVisionVector) == 0 {
		return nil, errors.New("root vision vector cannot be empty")
	}
	if cfg.QueueCapacity <= 0 {
		cfg.QueueCapacity = DefaultIntentQueueCapacity
	}
	if cfg.RiverCapacity <= 0 {
		cfg.RiverCapacity = DefaultRiverCapacity
	}
	if cfg.ConvergenceWindow <= 0 {
		cfg.ConvergenceWindow = DefaultConvergenceWindow
	}
	if cfg.MinConsensusDrops <= 0 {
		cfg.MinConsensusDrops = DefaultMinConsensusDrops
	}
	if cfg.AlignmentThreshold <= 0 {
		cfg.AlignmentThreshold = DefaultAlignmentThreshold
	}

	engineCtx, cancel := context.WithCancel(ctx)
	engine := &RiverConvergenceEngine{
		cfg:    cfg,
		inbox:  make(chan IntentDrop, cfg.QueueCapacity),
		rivers: make(map[string]*IntentRiver),
		ctx:    engineCtx,
		cancel: cancel,
	}

	engine.wg.Add(1)
	goroutinelabels.NewGoroutine("ambient_river_processor", "process intent drops and pool rivers").StartSimple(func() {
		defer engine.wg.Done()
		engine.runConvergenceLoop()
	})

	return engine, nil
}

// IngestDrop sends a single raindrop of intent into the convergence engine.
func (e *RiverConvergenceEngine) IngestDrop(drop IntentDrop) error {
	if e.closed.Load() {
		return ErrEngineClosed
	}
	if drop.MissionID == "" {
		return ErrEmptyMissionID
	}
	if len(drop.IntentVector) != len(e.cfg.RootVisionVector) {
		return ErrDimensionMismatch
	}
	if drop.Weight <= 0 {
		drop.Weight = 1.0
	}
	if drop.Timestamp.IsZero() {
		drop.Timestamp = time.Now()
	}

	select {
	case e.inbox <- drop:
		e.dropsIngested.Add(1)
		return nil
	case <-e.ctx.Done():
		return ErrEngineClosed
	}
}

// Close gracefully terminates the convergence engine and waits for worker goroutines to flush.
func (e *RiverConvergenceEngine) Close() error {
	if e.closed.Swap(true) {
		return nil
	}
	e.cancel()
	e.wg.Wait()
	close(e.inbox)
	return nil
}

// GetRiver retrieves an active Intent River by ID.
func (e *RiverConvergenceEngine) GetRiver(riverID string) (*IntentRiver, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	river, ok := e.rivers[riverID]
	if !ok {
		return nil, false
	}
	// Return defensive copy
	copyRiver := *river
	copyRiver.PooledVector = append([]float64(nil), river.PooledVector...)
	copyRiver.ActiveMissions = append([]string(nil), river.ActiveMissions...)
	return &copyRiver, true
}

// ListRivers returns a snapshot of all active rivers.
func (e *RiverConvergenceEngine) ListRivers() []*IntentRiver {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make([]*IntentRiver, 0, len(e.rivers))
	for _, r := range e.rivers {
		copyRiver := *r
		copyRiver.PooledVector = append([]float64(nil), r.PooledVector...)
		copyRiver.ActiveMissions = append([]string(nil), r.ActiveMissions...)
		result = append(result, &copyRiver)
	}
	return result
}

// Metrics returns real-time convergence statistics.
func (e *RiverConvergenceEngine) Metrics() ConvergenceMetrics {
	e.mu.RLock()
	defer e.mu.RUnlock()
	avgAlign := 0.0
	if e.alignmentEvaluations > 0 {
		avgAlign = e.totalAlignmentSum / float64(e.alignmentEvaluations)
	}
	return ConvergenceMetrics{
		TotalDropsIngested: e.dropsIngested.Load(),
		TotalRiversFormed:  e.riversFormed.Load(),
		DivergenceWarnings: e.divergenceWarnings.Load(),
		AverageAlignment:   avgAlign,
		ActiveRiversCount:  len(e.rivers),
		LastConvergenceAt:  time.Now(),
	}
}

func (e *RiverConvergenceEngine) runConvergenceLoop() {
	ticker := time.NewTicker(e.cfg.ConvergenceWindow)
	defer ticker.Stop()

	pendingDrops := make([]IntentDrop, 0, 64)

	for {
		select {
		case <-e.ctx.Done():
			// Drain remaining inbox
			for {
				select {
				case drop := <-e.inbox:
					pendingDrops = append(pendingDrops, drop)
				default:
					if len(pendingDrops) > 0 {
						e.processBatch(pendingDrops)
					}
					return
				}
			}
		case drop := <-e.inbox:
			pendingDrops = append(pendingDrops, drop)
			if len(pendingDrops) >= 64 {
				e.processBatch(pendingDrops)
				pendingDrops = pendingDrops[:0]
			}
		case <-ticker.C:
			if len(pendingDrops) > 0 {
				e.processBatch(pendingDrops)
				pendingDrops = pendingDrops[:0]
			}
		}
	}
}

func (e *RiverConvergenceEngine) processBatch(drops []IntentDrop) {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, drop := range drops {
		riverID := fmt.Sprintf("river:%s", drop.MissionID)
		river, exists := e.rivers[riverID]
		if !exists {
			river = &IntentRiver{
				ID:             riverID,
				RootVisionID:   e.cfg.RootVisionID,
				ActiveMissions: []string{drop.MissionID},
				PooledVector:   make([]float64, len(drop.IntentVector)),
				FormedAt:       drop.Timestamp,
				LastConverged:  drop.Timestamp,
				Status:         "forming",
			}
			e.rivers[riverID] = river
			e.riversFormed.Add(1)
		}

		// Accumulate weighted intent vector
		for i, v := range drop.IntentVector {
			river.PooledVector[i] = (river.PooledVector[i]*river.TotalWeight + v*drop.Weight) / (river.TotalWeight + drop.Weight)
		}
		river.TotalWeight += drop.Weight
		river.DropCount++
		river.LastConverged = time.Now()

		// Evaluate alignment against Root Vision using cosine similarity
		alignment := CosineSimilarity(river.PooledVector, e.cfg.RootVisionVector)
		river.AlignmentScore = alignment
		e.totalAlignmentSum += alignment
		e.alignmentEvaluations++

		if river.DropCount >= e.cfg.MinConsensusDrops {
			if alignment >= e.cfg.AlignmentThreshold {
				river.Status = "surging"
			} else {
				river.Status = "diverging"
				e.divergenceWarnings.Add(1)
				if e.cfg.OnDivergence != nil {
					e.cfg.OnDivergence(river.ID, drop.MissionID, alignment)
				}
			}
		}
	}
}

// CosineSimilarity calculates the cosine similarity between two normalized or raw floating-point vectors.
func CosineSimilarity(a, b []float64) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0.0
	}
	var dotProduct, normA, normB float64
	for i := 0; i < len(a); i++ {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0.0 || normB == 0.0 {
		return 0.0
	}
	sim := dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
	if math.IsNaN(sim) {
		return 0.0
	}
	return sim
}
