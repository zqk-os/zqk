package policyinterrupt

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/paths"
)

func TestAckWAL_AppendLoadAndCompact(t *testing.T) {
	root := t.TempDir()
	projectRoot := root

	// Append two acks for same key (second should win).
	if err := AppendAck(projectRoot, AckRecord{Profile: "policy", DedupeKey: "k1", AckedBy: "account:a"}); err != nil {
		t.Fatalf("AppendAck 1: %v", err)
	}
	if err := AppendAck(projectRoot, AckRecord{Profile: "policy", DedupeKey: "k1", AckedBy: "account:b"}); err != nil {
		t.Fatalf("AppendAck 2: %v", err)
	}

	acks, err := LoadAcksIncremental(projectRoot)
	if err != nil {
		t.Fatalf("LoadAcksIncremental: %v", err)
	}
	st, ok := acks[ackKey("policy", "k1")]
	if !ok {
		t.Fatalf("expected ack state for k1")
	}
	if st.AckedBy != "account:b" {
		t.Fatalf("expected latest acked_by account:b, got %q", st.AckedBy)
	}

	// Compact should succeed and keep state.
	if err := CompactAckWAL(projectRoot); err != nil {
		t.Fatalf("CompactAckWAL: %v", err)
	}
	acks2, err := LoadAcksIncremental(projectRoot)
	if err != nil {
		t.Fatalf("LoadAcksIncremental after compact: %v", err)
	}
	if _, ok := acks2[ackKey("policy", "k1")]; !ok {
		t.Fatalf("expected ack state after compact")
	}
}

func TestGate_LoadLatestCriticalUnacked(t *testing.T) {
	projectRoot := t.TempDir()

	dedupe := "POLICY-AGENT-002:critical"
	if err := AppendInterrupt(projectRoot, InterruptRecord{
		Profile:         "policy",
		Severity:        SeverityCritical,
		AckRequired:     true,
		DedupeKey:       dedupe,
		Message:         "Test interrupt",
		SuggestedAction: "Run ack",
	}); err != nil {
		t.Fatalf("AppendInterrupt: %v", err)
	}

	acks, err := LoadAcksIncremental(projectRoot)
	if err != nil {
		t.Fatalf("LoadAcksIncremental: %v", err)
	}
	latest, err := LoadLatestCriticalUnacked(projectRoot, acks)
	if err != nil {
		t.Fatalf("LoadLatestCriticalUnacked: %v", err)
	}
	if latest == nil || latest.DedupeKey != dedupe {
		t.Fatalf("expected latest unacked interrupt for %q", dedupe)
	}

	// Ack it; gate should return nil.
	if err := AppendAck(projectRoot, AckRecord{Profile: "policy", DedupeKey: dedupe, AckedBy: "account:x"}); err != nil {
		t.Fatalf("AppendAck: %v", err)
	}
	acks2, _ := LoadAcksIncremental(projectRoot)
	latest2, err := LoadLatestCriticalUnacked(projectRoot, acks2)
	if err != nil {
		t.Fatalf("LoadLatestCriticalUnacked 2: %v", err)
	}
	if latest2 != nil {
		t.Fatalf("expected no unacked interrupt after ack")
	}
}

func TestAckWAL_ExpiresDropped(t *testing.T) {
	projectRoot := t.TempDir()
	expired := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
	if err := AppendAck(projectRoot, AckRecord{Profile: "policy", DedupeKey: "kexp", AckedBy: "account:x", ExpiresAtRFC3339: expired}); err != nil {
		t.Fatalf("AppendAck: %v", err)
	}
	acks, err := LoadAcksIncremental(projectRoot)
	if err != nil {
		t.Fatalf("LoadAcksIncremental: %v", err)
	}
	if _, ok := acks[ackKey("policy", "kexp")]; ok {
		t.Fatalf("expected expired ack to be dropped")
	}

	// Compact should also drop it and rewrite WAL.
	if err := CompactAckWAL(projectRoot); err != nil {
		t.Fatalf("CompactAckWAL: %v", err)
	}
	// Ensure WAL exists and is readable.
	walPath := filepath.Join(projectRoot, paths.ProjectDataDir, paths.WalDir, policyInterruptAckWALFileName)
	if _, err := os.Stat(walPath); err != nil {
		t.Fatalf("expected WAL file to exist: %v", err)
	}
}

func TestLoadCriticalUnacked_SortedAndFiltered(t *testing.T) {
	projectRoot := t.TempDir()

	// Non-critical should be ignored.
	if err := AppendInterrupt(projectRoot, InterruptRecord{
		Profile:     "policy",
		Severity:    SeverityHigh,
		AckRequired: true,
		DedupeKey:   "high-1",
		Message:     "non-critical",
	}); err != nil {
		t.Fatalf("AppendInterrupt high: %v", err)
	}

	// Critical unacked #1.
	if err := AppendInterrupt(projectRoot, InterruptRecord{
		Profile:     "policy",
		Severity:    SeverityCritical,
		AckRequired: true,
		DedupeKey:   "crit-1",
		Message:     "critical one",
	}); err != nil {
		t.Fatalf("AppendInterrupt crit-1: %v", err)
	}

	// Critical unacked #2 (newer, should sort first).
	if err := AppendInterrupt(projectRoot, InterruptRecord{
		Profile:     "policy",
		Severity:    SeverityCritical,
		AckRequired: true,
		DedupeKey:   "crit-2",
		Message:     "critical two",
	}); err != nil {
		t.Fatalf("AppendInterrupt crit-2: %v", err)
	}

	// Acknowledge crit-1 so it is filtered out.
	if err := AppendAck(projectRoot, AckRecord{
		Profile:   "policy",
		DedupeKey: "crit-1",
		AckedBy:   "account:test",
	}); err != nil {
		t.Fatalf("AppendAck crit-1: %v", err)
	}

	acks, err := LoadAcksIncremental(projectRoot)
	if err != nil {
		t.Fatalf("LoadAcksIncremental: %v", err)
	}

	all, err := LoadCriticalUnacked(projectRoot, acks, 0)
	if err != nil {
		t.Fatalf("LoadCriticalUnacked all: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 critical unacked interrupt, got %d", len(all))
	}
	if all[0].DedupeKey != "crit-2" {
		t.Fatalf("expected crit-2 to remain unacked, got %q", all[0].DedupeKey)
	}

	// Add one more current critical item and verify descending seq + limit.
	if err := AppendInterrupt(projectRoot, InterruptRecord{
		Profile:     "policy",
		Severity:    SeverityCritical,
		AckRequired: true,
		DedupeKey:   "crit-3",
		Message:     "critical three",
	}); err != nil {
		t.Fatalf("AppendInterrupt crit-3: %v", err)
	}

	acks2, err := LoadAcksIncremental(projectRoot)
	if err != nil {
		t.Fatalf("LoadAcksIncremental 2: %v", err)
	}
	limited, err := LoadCriticalUnacked(projectRoot, acks2, 1)
	if err != nil {
		t.Fatalf("LoadCriticalUnacked limited: %v", err)
	}
	if len(limited) != 1 {
		t.Fatalf("expected 1 item with limit, got %d", len(limited))
	}
	if limited[0].DedupeKey != "crit-3" {
		t.Fatalf("expected newest critical item crit-3, got %q", limited[0].DedupeKey)
	}
}
