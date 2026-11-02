package ambient

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// Intent stream configuration constants (zero magic numbers).
const (
	DefaultStreamCapacity   = 1000
	DefaultMinWeightFloor   = 0.01
	MaxRaindropContentBytes = 64 * 1024
	StreamStatusPooling     = "pooling"
	StreamStatusMerged      = "merged"
	StreamStatusDischarged  = "discharged"
)

// IntentRaindrop represents an atomic intent signal or preference droplet.
type IntentRaindrop struct {
	ID        string    `json:"id"`
	Source    string    `json:"source"`
	Kind      string    `json:"kind"`
	Content   string    `json:"content"`
	Weight    float64   `json:"weight"`
	Timestamp time.Time `json:"timestamp"`
	Tags      []string  `json:"tags,omitempty"`
}

// IntentStream represents a pooled stream of aggregated intent droplets.
type IntentStream struct {
	ID                string           `json:"id"`
	Name              string           `json:"name"`
	Raindrops         []IntentRaindrop `json:"raindrops"`
	TotalWeight       float64          `json:"total_weight"`
	Status            string           `json:"status"`
	CreatedAt         time.Time        `json:"created_at"`
	MergedAt          time.Time        `json:"merged_at,omitempty"`
	PreferenceSummary string           `json:"preference_summary,omitempty"`
}

// StreamHydrator is a middle-brain holon that pools intent raindrops into coherent preference streams.
type StreamHydrator struct {
	mu          sync.RWMutex
	capacity    int
	minWeight   float64
	streams     map[string]*IntentStream
	raindropIDs map[string]struct{}
}

// NewStreamHydrator constructs a new StreamHydrator with capacity bounds.
func NewStreamHydrator(capacity int, minWeight float64) *StreamHydrator {
	if capacity <= 0 {
		capacity = DefaultStreamCapacity
	}
	if minWeight <= 0 {
		minWeight = DefaultMinWeightFloor
	}
	return &StreamHydrator{
		capacity:    capacity,
		minWeight:   minWeight,
		streams:     make(map[string]*IntentStream),
		raindropIDs: make(map[string]struct{}),
	}
}

// PoolRaindrop ingests an IntentRaindrop into the designated stream, validating boundaries and deduplicating.
func (h *StreamHydrator) PoolRaindrop(ctx context.Context, streamID string, drop IntentRaindrop) error {
	if h == nil {
		return errfmt.Errorf("stream hydrator: nil receiver")
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if strings.TrimSpace(streamID) == "" {
		return errfmt.Errorf("stream hydrator: streamID cannot be empty")
	}
	if strings.TrimSpace(drop.ID) == "" {
		return errfmt.Errorf("stream hydrator: raindrop ID cannot be empty")
	}
	if len(drop.Content) > MaxRaindropContentBytes {
		return errfmt.Errorf("stream hydrator: raindrop content exceeds maximum size of %d bytes", MaxRaindropContentBytes)
	}
	if drop.Weight < h.minWeight {
		return errfmt.Errorf("stream hydrator: raindrop weight %f below minimum threshold %f", drop.Weight, h.minWeight)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.streams == nil || h.raindropIDs == nil {
		return errfmt.Errorf("stream hydrator: uninitialized internal store")
	}

	if _, exists := h.raindropIDs[drop.ID]; exists {
		return errfmt.Errorf("stream hydrator: duplicate raindrop ID %q rejected", drop.ID)
	}

	st, exists := h.streams[streamID]
	if !exists {
		if len(h.streams) >= h.capacity {
			return errfmt.Errorf("stream hydrator: capacity limit reached (%d streams)", h.capacity)
		}
		st = &IntentStream{
			ID:        streamID,
			Name:      streamID,
			Status:    StreamStatusPooling,
			CreatedAt: time.Now().UTC(),
		}
		h.streams[streamID] = st
	}

	if st.Status != StreamStatusPooling {
		return errfmt.Errorf("stream hydrator: stream %q is %s and not accepting droplets", streamID, st.Status)
	}

	if drop.Timestamp.IsZero() {
		drop.Timestamp = time.Now().UTC()
	}

	st.Raindrops = append(st.Raindrops, drop)
	st.TotalWeight += drop.Weight
	h.raindropIDs[drop.ID] = struct{}{}
	return nil
}

func copyStreamSnapshot(st *IntentStream) *IntentStream {
	if st == nil {
		return nil
	}
	copied := *st
	copied.Raindrops = make([]IntentRaindrop, len(st.Raindrops))
	copy(copied.Raindrops, st.Raindrops)
	return &copied
}

// GetStream retrieves an intent stream snapshot by ID.
func (h *StreamHydrator) GetStream(streamID string) (*IntentStream, bool) {
	if h == nil {
		return nil, false
	}
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.streams == nil {
		return nil, false
	}
	st, exists := h.streams[streamID]
	if !exists {
		return nil, false
	}
	return copyStreamSnapshot(st), true
}

// ListStreams returns a snapshot of all tracked intent streams.
func (h *StreamHydrator) ListStreams() []*IntentStream {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.streams == nil {
		return nil
	}
	out := make([]*IntentStream, 0, len(h.streams))
	for _, st := range h.streams {
		out = append(out, copyStreamSnapshot(st))
	}
	return out
}

// MergeStream merges all pooled raindrops in a stream into a unified preference summary and flips status to merged.
func (h *StreamHydrator) MergeStream(streamID string) (*IntentStream, error) {
	if h == nil {
		return nil, errfmt.Errorf("stream hydrator: nil receiver")
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.streams == nil {
		return nil, errfmt.Errorf("stream hydrator: uninitialized stream store")
	}
	st, exists := h.streams[streamID]
	if !exists {
		return nil, errfmt.Errorf("stream hydrator: stream %q not found", streamID)
	}
	if len(st.Raindrops) == 0 {
		return nil, errfmt.Errorf("stream hydrator: stream %q has zero raindrops to merge", streamID)
	}

	var contents []string
	for _, drop := range st.Raindrops {
		contents = append(contents, drop.Content)
	}

	st.PreferenceSummary = strings.Join(contents, " -> ")
	st.Status = StreamStatusMerged
	st.MergedAt = time.Now().UTC()

	return copyStreamSnapshot(st), nil
}
