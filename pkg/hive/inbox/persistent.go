package inbox

import (
	"bufio"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type Operation string

const (
	OpSubmit  Operation = "submit"
	OpApprove Operation = "approve"
	OpReject  Operation = "reject"
)

// TDEEvent represents a single change to the inbox state.
type TDEEvent struct {
	Op         Operation   `json:"op"`
	Timestamp  time.Time   `json:"timestamp"`
	Envelope   TDEEnvelope `json:"envelope"`         // Used for Submit
	EnvelopeID string      `json:"envelope_id"`      // Used for Approve/Reject
	Reason     string      `json:"reason,omitempty"` // Used for Reject
}

// PersistentInbox wraps a memoryInbox and writes an append-only JSONL log.
type PersistentInbox struct {
	mu       sync.Mutex // guards the file writer
	inner    Inbox
	logFile  *fileutil.File
	stateDir string
}

// NewPersistentInbox creates a new inbox backed by an append-only log in stateDir.
func NewPersistentInbox(stateDir string) (*PersistentInbox, error) {
	if err := fileutil.EnsureDir(stateDir); err != nil {
		return nil, fmt.Errorf("failed to create state dir: %w", err)
	}

	logPath := filepath.Join(stateDir, "inbox_tde.jsonl")
	file, err := fileutil.OpenFile(logPath, fileutil.O_CREATE|fileutil.O_RDWR|fileutil.O_APPEND, paths.FilePerm600)
	if err != nil {
		return nil, fmt.Errorf("failed to open inbox log: %w", err)
	}

	inner := NewMemoryInbox()

	// Replay existing events
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var ev TDEEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			// Skip corrupted lines in the WAL to maintain resilience
			continue
		}

		switch ev.Op {
		case OpSubmit:
			_ = inner.Submit(ev.Envelope)
		case OpApprove:
			_ = inner.Approve(ev.EnvelopeID)
		case OpReject:
			_ = inner.Reject(ev.EnvelopeID, ev.Reason)
		}
	}

	return &PersistentInbox{
		inner:    inner,
		logFile:  file,
		stateDir: stateDir,
	}, nil
}

func (p *PersistentInbox) writeEvent(ev TDEEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	_, err = p.logFile.Write(data)
	if err == nil {
		// Sync to disk to ensure durability across reboots
		_ = p.logFile.Sync()
	}
	return err
}

func (p *PersistentInbox) Submit(env TDEEnvelope) error {
	err := p.inner.Submit(env)
	if err != nil {
		return err
	}

	ev := TDEEvent{
		Op:        OpSubmit,
		Timestamp: time.Now(),
		Envelope:  env,
	}
	return p.writeEvent(ev)
}

func (p *PersistentInbox) Get(id string) (TDEEnvelope, error) {
	return p.inner.Get(id)
}

func (p *PersistentInbox) ListPending() []TDEEnvelope {
	return p.inner.ListPending()
}

func (p *PersistentInbox) Approve(id string) error {
	err := p.inner.Approve(id)
	if err != nil {
		return err
	}

	ev := TDEEvent{
		Op:         OpApprove,
		Timestamp:  time.Now(),
		EnvelopeID: id,
	}
	return p.writeEvent(ev)
}

func (p *PersistentInbox) Reject(id string, reason string) error {
	err := p.inner.Reject(id, reason)
	if err != nil {
		return err
	}

	ev := TDEEvent{
		Op:         OpReject,
		Timestamp:  time.Now(),
		EnvelopeID: id,
		Reason:     reason,
	}
	return p.writeEvent(ev)
}

// Close gracefully closes the log file.
func (p *PersistentInbox) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.logFile.Close()
}
