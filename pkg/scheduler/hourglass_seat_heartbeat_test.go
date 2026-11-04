package scheduler

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/agentclaim"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type seatWakeStub struct {
	to  string
	msg string
	n   int
}

func (s *seatWakeStub) Wake(_ context.Context, req agentfeed.PeerWakeRequest) (agentfeed.PeerWakeAdapterResult, error) {
	s.n++
	s.to = req.ToAgentID
	s.msg = req.Message
	return agentfeed.PeerWakeAdapterResult{Live: true, Transport: "test"}, nil
}

func TestHandleMissedSeatHeartbeat_wakesAndRearms(t *testing.T) {
	// Not parallel: SetPeerWakeAdapter is process-wide.
	root := t.TempDir()
	seat := "cursor-composer"
	if err := writeExpiredSeatTimer(root, seat); err != nil {
		t.Fatalf("write expired: %v", err)
	}

	stub := &seatWakeStub{}
	prev := agentfeed.CurrentPeerWakeAdapter()
	agentfeed.SetPeerWakeAdapter(stub)
	t.Cleanup(func() { agentfeed.SetPeerWakeAdapter(prev) })

	s := &Scheduler{projectRoot: root}
	s.handleMissedSeatHeartbeat(context.Background(), agentclaim.SeatHeartbeatPath(root, seat))

	if stub.n != 1 {
		t.Fatalf("wake calls = %d, want 1", stub.n)
	}
	if stub.to != seat {
		t.Fatalf("woke ToAgentID=%q, want %q", stub.to, seat)
	}
	if stub.msg == "" {
		t.Fatal("expected ATTN wake message")
	}
	after, err := agentclaim.LoadSeatHeartbeat(root, seat)
	if err != nil || after == nil {
		t.Fatalf("expected rearmed timer: %v", err)
	}
	if after.Misses < 1 {
		t.Fatalf("misses = %d, want >= 1", after.Misses)
	}
	exp, err := time.Parse(time.RFC3339, after.ExpiresAt)
	if err != nil {
		t.Fatalf("parse expires: %v", err)
	}
	if !exp.After(time.Now()) {
		t.Fatal("rearm should push expires_at into the future")
	}
}

func TestCheckHourglassTimers_seatHeartbeatRoutesToWake(t *testing.T) {
	// Not parallel: SetPeerWakeAdapter is process-wide.
	root := t.TempDir()
	seat := "peer-agy-1"
	if err := writeExpiredSeatTimer(root, seat); err != nil {
		t.Fatalf("write: %v", err)
	}

	stub := &seatWakeStub{}
	prev := agentfeed.CurrentPeerWakeAdapter()
	agentfeed.SetPeerWakeAdapter(stub)
	t.Cleanup(func() { agentfeed.SetPeerWakeAdapter(prev) })

	s := &Scheduler{projectRoot: root}
	s.checkHourglassTimers(context.Background())
	if stub.n != 1 {
		t.Fatalf("wake calls = %d, want 1 (seat miss must not fall through to SIGKILL)", stub.n)
	}
	if stub.to != seat {
		t.Fatalf("ToAgentID=%q", stub.to)
	}
}

func TestActionForExpiredTimer_seatHeartbeatNeverKills(t *testing.T) {
	t.Parallel()
	if got := actionForExpiredTimer(agentclaim.TimerTypeSeatHeartbeat); got != actionWakeOrchestrator {
		t.Fatalf("got %v", got)
	}
}

func writeExpiredSeatTimer(root, seat string) error {
	timer := &agentclaim.CheckinTimer{
		TaskID:         seat,
		Kind:           agentclaim.KindAgentSeat,
		Type:           agentclaim.TimerTypeSeatHeartbeat,
		ExpiresAt:      time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339),
		ClaimedBy:      seat,
		CadenceSeconds: 60,
		ArmedAt:        time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339),
		LastCheckinAt:  time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339),
	}
	path := agentclaim.SeatHeartbeatPath(root, seat)
	if err := fileutil.MkdirAll(filepath.Dir(path), paths.DirPerm755); err != nil {
		return err
	}
	data, err := json.Marshal(timer)
	if err != nil {
		return err
	}
	return fileutil.WriteFile(path, data, paths.FilePerm644)
}
