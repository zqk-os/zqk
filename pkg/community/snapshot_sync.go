package community

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/strutil"
)

// SnapshotHeader defines the metadata and cryptographic verification checksum for an epoch snapshot.
type SnapshotHeader struct {
	ClusterID     string    `json:"cluster_id"`
	NodeID        string    `json:"node_id"`
	Epoch         uint64    `json:"epoch"`
	CreatedAt     time.Time `json:"created_at"`
	PayloadSHA256 string    `json:"payload_sha256"`
	EntityCount   int       `json:"entity_count"`
}

// Validate verifies that the header fields satisfy static floor invariants.
func (h *SnapshotHeader) Validate() error {
	if err := strutil.RequireNonBlank(h.ClusterID, "cluster ID"); err != nil {
		return err
	}
	if err := strutil.RequireNonBlank(h.NodeID, "node ID"); err != nil {
		return err
	}
	if h.Epoch == 0 {
		return errors.New("epoch must be greater than zero")
	}
	if err := strutil.RequireNonBlank(h.PayloadSHA256, "payload SHA256"); err != nil {
		return err
	}
	if h.CreatedAt.IsZero() {
		return errors.New("created_at timestamp cannot be zero")
	}
	return nil
}

// SnapshotEnvelope bundles the verified header and serialized state payload.
type SnapshotEnvelope struct {
	Header  SnapshotHeader `json:"header"`
	Payload []byte         `json:"payload"`
}

// VerifyChecksum validates that the SHA-256 hash of the payload matches Header.PayloadSHA256.
func (env *SnapshotEnvelope) VerifyChecksum() error {
	if err := env.Header.Validate(); err != nil {
		return fmt.Errorf("invalid header: %w", err)
	}
	sum := sha256.Sum256(env.Payload)
	actualHex := hex.EncodeToString(sum[:])
	if actualHex != env.Header.PayloadSHA256 {
		return fmt.Errorf("cryptographic checksum mismatch: expected %s, got %s", env.Header.PayloadSHA256, actualHex)
	}
	return nil
}

// SnapshotSyncController manages capturing, restoring, and replay-fencing distributed state snapshots.
type SnapshotSyncController struct {
	mu           sync.RWMutex
	clusterID    string
	nodeID       string
	currentEpoch uint64
	entities     map[string]string
}

// NewSnapshotSyncController instantiates a new controller.
func NewSnapshotSyncController(clusterID, nodeID string) (*SnapshotSyncController, error) {
	if err := strutil.RequireNonBlank(clusterID, "cluster ID"); err != nil {
		return nil, err
	}
	if err := strutil.RequireNonBlank(nodeID, "node ID"); err != nil {
		return nil, err
	}

	return &SnapshotSyncController{
		clusterID: clusterID,
		nodeID:    nodeID,
		entities:  make(map[string]string),
	}, nil
}

// CaptureSnapshot snapshots current in-memory entities and produces an authenticated SnapshotEnvelope.
func (c *SnapshotSyncController) CaptureSnapshot(epoch uint64, state map[string]string) (*SnapshotEnvelope, error) {
	if epoch == 0 {
		return nil, errors.New("epoch must be greater than zero")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if epoch <= c.currentEpoch {
		return nil, fmt.Errorf("epoch %d must be strictly greater than current epoch %d", epoch, c.currentEpoch)
	}

	if state == nil {
		state = make(map[string]string)
	}

	payloadBytes, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("marshal state payload: %w", err)
	}

	sum := sha256.Sum256(payloadBytes)
	checksum := hex.EncodeToString(sum[:])

	header := SnapshotHeader{
		ClusterID:     c.clusterID,
		NodeID:        c.nodeID,
		Epoch:         epoch,
		CreatedAt:     time.Now().UTC(),
		PayloadSHA256: checksum,
		EntityCount:   len(state),
	}

	c.currentEpoch = epoch
	c.entities = make(map[string]string, len(state))
	for k, v := range state {
		c.entities[k] = v
	}

	return &SnapshotEnvelope{
		Header:  header,
		Payload: payloadBytes,
	}, nil
}

// RestoreSnapshot restores the controller's state from a cryptographically verified snapshot.
func (c *SnapshotSyncController) RestoreSnapshot(env *SnapshotEnvelope) error {
	if env == nil {
		return errors.New("cannot restore from nil snapshot envelope")
	}
	if err := env.VerifyChecksum(); err != nil {
		return fmt.Errorf("snapshot verification failed: %w", err)
	}

	var restored map[string]string
	if err := json.Unmarshal(env.Payload, &restored); err != nil {
		return fmt.Errorf("unmarshal state payload: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if env.Header.Epoch < c.currentEpoch {
		return fmt.Errorf("cannot regress epoch from %d to %d", c.currentEpoch, env.Header.Epoch)
	}

	c.currentEpoch = env.Header.Epoch
	c.entities = make(map[string]string, len(restored))
	for k, v := range restored {
		c.entities[k] = v
	}

	return nil
}

// CanReplayWALEntry checks whether a WAL entry can be replayed or if it is fenced by the snapshot epoch.
func (c *SnapshotSyncController) CanReplayWALEntry(walEpoch uint64) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return walEpoch > c.currentEpoch
}

// GetEntity retrieves an entity from the current state.
func (c *SnapshotSyncController) GetEntity(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	val, ok := c.entities[key]
	return val, ok
}

// CurrentEpoch returns the current epoch.
func (c *SnapshotSyncController) CurrentEpoch() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.currentEpoch
}
