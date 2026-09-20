// Package clusterstatus is the cluster status plane (API-gateway shaped) for
// permeating job/task phase/progress/error across roots and nodes.
// See docs/architecture/SCHEDULER_HOST_SERVICE_AND_CLUSTER_STATUS.md.
package clusterstatus

import (
	"encoding/json"
	"path/filepath"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	PhaseRunning   = "running"
	PhaseSucceeded = "succeeded"
	PhaseFailed    = "failed"
	PhaseDegraded  = "degraded"
	PhaseStale     = "stale"

	DefaultTTL = 2 * time.Minute
	logRelPath = "logs/cluster_status/events.jsonl"
)

// Event is a normalized status plane record.
type Event struct {
	NodeID    string    `json:"node_id"`
	RootID    string    `json:"root_id"`
	JobID     string    `json:"job_id,omitempty"`
	TaskID    string    `json:"task_id,omitempty"`
	Phase     string    `json:"phase"`
	Progress  float64   `json:"progress,omitempty"`
	Error     string    `json:"error,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Watch matches a dependent's depends_on interest.
type Watch struct {
	RootID string
	JobID  string
	TaskID string
	TTL    time.Duration
}

// Bus is a minimal local status gateway (file-backed emit + in-memory latest).
type Bus struct {
	mu     sync.RWMutex
	latest map[string]Event // key: rootID|jobID|taskID
	path   string
	ttl    time.Duration
}

// NewBus creates a bus rooted at projectRoot (writes under .zqk/logs/cluster_status/).
func NewBus(projectRoot string, ttl time.Duration) *Bus {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	return &Bus{
		latest: make(map[string]Event),
		path:   filepath.Join(projectRoot, paths.ProjectDataDir, logRelPath),
		ttl:    ttl,
	}
}

func eventKey(e Event) string {
	return e.RootID + "|" + e.JobID + "|" + e.TaskID
}

// Emit appends an event and updates the latest map.
func (b *Bus) Emit(e Event) error {
	if e.UpdatedAt.IsZero() {
		e.UpdatedAt = time.Now().UTC()
	}
	b.mu.Lock()
	b.latest[eventKey(e)] = e
	b.mu.Unlock()
	if err := fileutil.MkdirAll(filepath.Dir(b.path), paths.DirPerm755); err != nil {
		return errfmt.Newf("cluster status log dir").Wrap(err)
	}
	f, err := fileutil.OpenFile(b.path, fileutil.O_APPEND|fileutil.O_CREATE|fileutil.O_WRONLY, paths.FilePerm644)
	if err != nil {
		return errfmt.Newf("open cluster status log").Wrap(err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	return enc.Encode(e)
}

// Latest returns the freshest event for a watch, or stale/degraded if beyond TTL.
func (b *Bus) Latest(w Watch) (Event, string, error) {
	ttl := w.TTL
	if ttl <= 0 {
		ttl = b.ttl
	}
	key := w.RootID + "|" + w.JobID + "|" + w.TaskID
	b.mu.RLock()
	e, ok := b.latest[key]
	b.mu.RUnlock()
	if !ok {
		return Event{RootID: w.RootID, JobID: w.JobID, TaskID: w.TaskID, Phase: PhaseStale}, PhaseStale, nil
	}
	if time.Since(e.UpdatedAt) > ttl {
		e.Phase = PhaseStale
		return e, PhaseStale, nil
	}
	return e, e.Phase, nil
}

// DependentDecision is fail-closed guidance for orchestration.
type DependentDecision struct {
	AllowForward bool
	Reason       string
	Phase        string
}

// Decide returns whether a dependent may advance given a watch.
func (b *Bus) Decide(w Watch) DependentDecision {
	e, phase, _ := b.Latest(w)
	switch phase {
	case PhaseSucceeded:
		return DependentDecision{AllowForward: true, Reason: "peer succeeded", Phase: phase}
	case PhaseRunning:
		return DependentDecision{AllowForward: false, Reason: "peer still running", Phase: phase}
	case PhaseFailed:
		return DependentDecision{AllowForward: false, Reason: "peer failed: " + e.Error, Phase: phase}
	case PhaseStale, PhaseDegraded:
		return DependentDecision{AllowForward: false, Reason: "peer status " + phase + " (fail closed)", Phase: phase}
	default:
		return DependentDecision{AllowForward: false, Reason: "unknown peer phase", Phase: phase}
	}
}
