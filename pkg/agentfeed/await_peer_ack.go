package agentfeed

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// Peer-ack await registry: register on steer, fire wake callback on feed ack
// (same “register then notify” shape as agent next --on-validation-failure wake).

const (
	AwaitActionWake      = "wake"
	AwaitStatusOpen      = "open"
	AwaitStatusCompleted = "completed"
	AwaitStatusCancelled = "cancelled"
	AwaitStatusExpired   = "expired"
)

// PeerAckAwait is one registered “ring me when peer_acks this event_id” callback.
type PeerAckAwait struct {
	ID          string `json:"id"`
	EventID     string `json:"event_id"`
	FromAgentID string `json:"from_agent_id"`
	ToAgentID   string `json:"to_agent_id,omitempty"`
	Action      string `json:"action"`
	WakeMessage string `json:"wake_message,omitempty"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
	CompletedAt string `json:"completed_at,omitempty"`
}

// PeerAckAwaitInput is the register request.
type PeerAckAwaitInput struct {
	EventID     string
	FromAgentID string
	ToAgentID   string
	Action      string
	WakeMessage string
}

type peerAckAwaitFile struct {
	Awaits []PeerAckAwait `json:"awaits"`
}

var (
	peerAckAwaitMu sync.Mutex
	// PeerAckAwaitFire invokes the registered action (default: PeerWakeAdapter).
	// Tests replace this.
	PeerAckAwaitFire = defaultPeerAckAwaitFire
)

func peerAckAwaitStorePath(projectRoot string) string {
	return paths.PeerAckAwaitsPath(projectRoot)
}

func loadLockedPeerAckAwaitFile(projectRoot string) (peerAckAwaitFile, func(), error) {
	peerAckAwaitMu.Lock()
	f, err := loadPeerAckAwaitFile(projectRoot)
	if err != nil {
		peerAckAwaitMu.Unlock()
		return peerAckAwaitFile{}, nil, err
	}
	return f, peerAckAwaitMu.Unlock, nil
}

// RegisterPeerAckAwait records an open await for event_id (caller will be woken on peer_ack).
func RegisterPeerAckAwait(projectRoot string, in PeerAckAwaitInput) (PeerAckAwait, error) {
	eventID := strings.TrimSpace(in.EventID)
	from := strings.TrimSpace(in.FromAgentID)
	if eventID == "" {
		return PeerAckAwait{}, errfmt.Errorf("event_id is required to register peer-ack await")
	}
	if from == "" {
		return PeerAckAwait{}, errfmt.Errorf("from_agent_id is required to register peer-ack await")
	}
	action := strings.TrimSpace(in.Action)
	if action == "" {
		action = AwaitActionWake
	}
	msg := strings.TrimSpace(in.WakeMessage)
	if msg == "" {
		msg = PeerAckPasteStub(eventID)
	}
	a := PeerAckAwait{
		ID:          "AWAIT-" + strings.TrimPrefix(NewEventID(), "AFE-"),
		EventID:     eventID,
		FromAgentID: from,
		ToAgentID:   strings.TrimSpace(in.ToAgentID),
		Action:      action,
		WakeMessage: msg,
		Status:      AwaitStatusOpen,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}

	f, unlock, err := loadLockedPeerAckAwaitFile(projectRoot)
	if err != nil {
		return PeerAckAwait{}, err
	}
	defer unlock()
	f.Awaits = append(f.Awaits, a)
	if err := savePeerAckAwaitFile(projectRoot, f); err != nil {
		return PeerAckAwait{}, err
	}
	return a, nil
}

// CompletePeerAckAwaits marks open awaits for eventID completed and fires callbacks.
// ackingAgentID must match await.ToAgentID when set (seat-bound; blocks impersonation).
func CompletePeerAckAwaits(projectRoot, eventID, ackingAgentID string) ([]PeerAckAwait, error) {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return nil, errfmt.Errorf("event_id is required to complete peer-ack awaits")
	}
	acking := strings.TrimSpace(ackingAgentID)
	peerAckAwaitMu.Lock()
	f, err := loadPeerAckAwaitFile(projectRoot)
	if err != nil {
		peerAckAwaitMu.Unlock()
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var done []PeerAckAwait
	for i := range f.Awaits {
		a := &f.Awaits[i]
		if a.Status != AwaitStatusOpen || a.EventID != eventID {
			continue
		}
		if to := strings.TrimSpace(a.ToAgentID); to != "" && acking != "" {
			if !seatMatchesAgent(Seat{AgentID: to}, acking) {
				continue
			}
		}
		a.Status = AwaitStatusCompleted
		a.CompletedAt = now
		done = append(done, *a)
	}
	if len(done) == 0 {
		peerAckAwaitMu.Unlock()
		return nil, nil
	}
	if err := savePeerAckAwaitFile(projectRoot, f); err != nil {
		peerAckAwaitMu.Unlock()
		return nil, err
	}
	peerAckAwaitMu.Unlock()

	for _, a := range done {
		if PeerAckAwaitFire == nil {
			continue
		}
		if err := PeerAckAwaitFire(a); err != nil {
			return done, errfmt.Newf("peer-ack await fired with error for %s", a.ID).Wrap(err)
		}
	}
	return done, nil
}

func withLockedPeerAckAwaitFile(projectRoot string, fn func(f *peerAckAwaitFile) error) error {
	f, unlock, err := loadLockedPeerAckAwaitFile(projectRoot)
	if err != nil {
		return err
	}
	defer unlock()
	return fn(&f)
}

// ListOpenPeerAckAwaits returns open awaits owned by fromAgentID (empty agent = all open).
func ListOpenPeerAckAwaits(projectRoot, fromAgentID string) ([]PeerAckAwait, error) {
	from := strings.TrimSpace(fromAgentID)
	var out []PeerAckAwait
	err := withLockedPeerAckAwaitFile(projectRoot, func(f *peerAckAwaitFile) error {
		for _, a := range f.Awaits {
			if a.Status != AwaitStatusOpen {
				continue
			}
			if from != "" && !strings.EqualFold(a.FromAgentID, from) {
				continue
			}
			out = append(out, a)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ExpirePeerAckAwaits marks open awaits expired if older than maxAge.
func ExpirePeerAckAwaits(projectRoot string, maxAge time.Duration) ([]PeerAckAwait, error) {
	now := time.Now().UTC()
	var expired []PeerAckAwait
	changed := false
	err := withLockedPeerAckAwaitFile(projectRoot, func(f *peerAckAwaitFile) error {
		for i := range f.Awaits {
			a := &f.Awaits[i]
			if a.Status != AwaitStatusOpen {
				continue
			}
			t, err := time.Parse(time.RFC3339, a.CreatedAt)
			if err != nil {
				continue
			}
			if now.Sub(t) > maxAge {
				a.Status = AwaitStatusExpired
				a.CompletedAt = now.Format(time.RFC3339)
				expired = append(expired, *a)
				changed = true
			}
		}
		if !changed {
			return nil
		}
		return savePeerAckAwaitFile(projectRoot, *f)
	})
	if err != nil {
		return nil, err
	}
	if !changed {
		return nil, nil
	}
	return expired, nil
}

func loadPeerAckAwaitFile(projectRoot string) (peerAckAwaitFile, error) {
	path := peerAckAwaitStorePath(projectRoot)
	b, err := fileutil.ReadFile(path)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return peerAckAwaitFile{}, nil
		}
		return peerAckAwaitFile{}, errfmt.Errorf("read peer_ack awaits: %w", err)
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return peerAckAwaitFile{}, nil
	}
	var f peerAckAwaitFile
	if err := json.Unmarshal(b, &f); err != nil {
		return peerAckAwaitFile{}, errfmt.Errorf("parse peer_ack awaits: %w", err)
	}
	return f, nil
}

func savePeerAckAwaitFile(projectRoot string, f peerAckAwaitFile) error {
	path := peerAckAwaitStorePath(projectRoot)
	if err := fileutil.EnsureDir(filepath.Dir(path)); err != nil {
		return errfmt.Errorf("create mesh state dir: %w", err)
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return errfmt.Errorf("marshal peer_ack awaits: %w", err)
	}
	if err := fileutil.WriteStandardFile(path, append(b, '\n')); err != nil {
		return errfmt.Errorf("write peer_ack awaits: %w", err)
	}
	return nil
}

func defaultPeerAckAwaitFire(a PeerAckAwait) error {
	if a.Action != AwaitActionWake && a.Action != "" {
		return nil // unknown actions: recorded only
	}
	root := "" // resolve from cwd when firing from CLI
	if testRoot := zqkenv.TestRoot().Get(); testRoot != "" {
		root = testRoot
	} else if wd, err := fileutil.Getwd(); err == nil {
		root = wd
	}
	if near := paths.FindNearestProjectRoot(root); near != "" {
		root = near
	}
	msg := strings.TrimSpace(a.WakeMessage)
	if msg == "" {
		msg = PeerAckPasteStub(a.EventID)
	}
	to := strings.TrimSpace(a.FromAgentID)
	if to == "" {
		to = "peer-tpm-02"
	}
	res := WakePeerOpts(context.Background(), WakePeerOptions{
		ProjectRoot:  root,
		Message:      msg,
		ToAgentID:    to,
		DeliveryMode: "notify",
	})
	if res.Skipped == "wake_script_missing" {
		_, _ = AppendEvent(AppendEventInput{
			ProjectRoot:      root,
			Message:          msg,
			AgentID:          "peer-ack-callback",
			Sender:           FeedSenderMeshStatus,
			EventType:        FeedEventTypeMeshStatus,
			SelfACK:          false,
			SkipEnabledCheck: true,
		})
		return nil
	}
	if res.Error != "" {
		return errfmt.Errorf("peer wake adapter: %s", res.Error)
	}
	return nil
}
