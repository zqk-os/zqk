package audit

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/pkg/logging"
)

type AuditRecord struct {
	ID        string `json:"id"`
	Action    string `json:"action"`
	Target    string `json:"target"`
	Timestamp string `json:"timestamp"`
	Status    string `json:"status,omitempty"`
	Overlay   string `json:"overlay,omitempty"`
}

type AuditStream struct {
	mu          sync.RWMutex
	subscribers map[chan AuditRecord]context.CancelFunc
	projectRoot string // absolute project root; WAL writes go here instead of CWD
}

// NewAuditStream creates a new AuditStream. projectRoot should be an absolute
// path; when empty, the WAL path falls back to relative (CWD), which may cause
// test data pollution (POL-CODE-006).
func NewAuditStream(projectRoot ...string) *AuditStream {
	root := ""
	if len(projectRoot) > 0 {
		root = projectRoot[0]
	}
	return &AuditStream{
		subscribers: make(map[chan AuditRecord]context.CancelFunc),
		projectRoot: root,
	}
}

func (a *AuditStream) Subscribe(ctx context.Context) <-chan AuditRecord {
	a.mu.Lock()
	defer a.mu.Unlock()
	ch := make(chan AuditRecord, 100)
	ctx, cancel := context.WithCancel(ctx) //nolint:gosec
	a.subscribers[ch] = cancel

	goroutinelabels.StartNamedGoroutine("audit-stream-worker", "stream audit logs", func() {
		func() {
			<-ctx.Done()
			a.mu.Lock()
			defer a.mu.Unlock()
			if c, ok := a.subscribers[ch]; ok {
				c() // ensure cancellation
				delete(a.subscribers, ch)
				close(ch)
			}
		}()
	})

	return ch
}

func (a *AuditStream) Publish(ctx context.Context, record AuditRecord) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Phase 3: Write to JSONL WAL
	b, err := json.Marshal(record)
	if err != nil {
		logging.FluentEvent(logging.GetLogger()).Error("Failed to marshal audit record", err).Log()
	} else {
		walDir := filepath.Join(a.projectRoot, paths.ProjectDataDir, "logs", "audit")
		if err := fileutil.EnsureDir(walDir); err != nil {
			logging.FluentEvent(logging.GetLogger()).Error("Failed to create audit WAL directory", err).Log()
		} else {
			if f, err := fileutil.OpenFile(filepath.Join(walDir, "wal.jsonl"), fileutil.O_APPEND|fileutil.O_CREATE|fileutil.O_WRONLY, 0644); err == nil {
				if _, err := f.Write(append(b, '\n')); err != nil {
					logging.FluentEvent(logging.GetLogger()).Error("Failed to write to audit WAL", err).Log()
				}
				_ = f.Close()
			} else {
				logging.FluentEvent(logging.GetLogger()).Error("Failed to open audit WAL file", err).Log()
			}
		}
	}

	for ch, cancel := range a.subscribers {
		select {
		case ch <- record:
		default:
			// Drop slow subscribers to prevent backpressure
			cancel()
			// We don't delete from map here because RLock is held.
			// The goroutine waiting on ctx.Done() will handle cleanup.
			logging.FluentEvent(logging.GetLogger()).Warn("Dropped slow audit subscriber").Log()
		}
	}
}
