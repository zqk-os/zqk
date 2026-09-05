package policyinterrupt

import (
	"bufio"
	"encoding/json"
	"sort"
	"strings"
	"time"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/walutil"
)

const (
	policyInterruptWALFileName      = "policy_interrupts.wal"
	policyInterruptAckWALFileName   = "policy_interrupt_acks.wal"
	policyInterruptWALCheckpointExt = ".checkpoint"
	defaultPolicyProfile            = "pol" + "icy" // logging profile id; not necessarily KindPolicy
	tmpFileSuffix                   = ".tmp"
	walFilePerm                     = 0o600

	// maxWALLineSize limits line length when reading (single JSON object per line).
	maxWALLineSize  = 64 * 1024
	seqInitialValue = 1
	ackKeySeparator = "::"
	emptyValue      = ""
)

type Severity string

const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

// InterruptRecord is one durable record in the policy interrupt WAL (append-only).
type InterruptRecord struct {
	Seq int64     `json:"seq"`
	Ts  time.Time `json:"ts"`

	// Profile is the queue/profile name. For v1 this is always "policy".
	Profile string `json:"profile"`

	Severity    Severity `json:"severity"`
	AckRequired bool     `json:"ack_required"`

	// DedupeKey identifies the logical interrupt instance. The gate checks ack by (profile, dedupe_key).
	DedupeKey string `json:"dedupe_key"`

	// Phase 14: Neurological Governance Fields
	Confidence     float64 `json:"confidence,omitempty"`      // Confidence interval [0.0, 1.0] of the agent's logic.
	IsStreaming    bool    `json:"is_streaming,omitempty"`    // If true, this is a non-blocking stream; humans can steer.
	DecisionBranch string  `json:"decision_branch,omitempty"` // The isolated context tree identifier this interrupt relates to.

	PolicyID         string `json:"policy_id,omitempty"`
	OriginOperation  string `json:"origin_operation,omitempty"`
	OriginActorID    string `json:"origin_actor_id,omitempty"`
	Message          string `json:"message,omitempty"`
	SuggestedAction  string `json:"suggested_action,omitempty"`
	ExpiresAtRFC3339 string `json:"expires_at,omitempty"`
}

// AckRecord is one durable record in the policy interrupt ack WAL (append-only).
type AckRecord struct {
	Seq int64     `json:"seq"`
	Ts  time.Time `json:"ts"`

	Profile   string `json:"profile"`
	DedupeKey string `json:"dedupe_key"`
	AckedBy   string `json:"acked_by"`

	// Phase 14: Neurological Governance Fields
	SteeringAction string `json:"steering_action,omitempty"` // Real-time course correction passed back to the agent

	Reason           string `json:"reason,omitempty"`
	ExpiresAtRFC3339 string `json:"expires_at,omitempty"`
}

type checkpoint struct {
	LastReplayedSeq  int64 `json:"last_replayed_seq"`
	LastCompactedSeq int64 `json:"last_compacted_seq"`
}

func newWAL[T any](projectRoot, filename string) (*walutil.JSONLineWAL[T], error) {
	return walutil.OpenJSONLineWAL[T](projectRoot, filename, policyInterruptWALCheckpointExt, maxWALLineSize)
}

func readCheckpoint(path string) checkpoint {
	var ck checkpoint
	_ = walutil.ReadJSONFile(path, &ck)
	return ck
}

func writeCheckpointAtomic(path string, ck checkpoint) error {
	return walutil.WriteJSONFileAtomic(path, ck, walFilePerm)
}

// ReplayFrom reads records from the WAL starting at seqAfter (exclusive).
func ReplayFrom[T any](walPath string, seqAfter int64, parse func([]byte) (*T, error), fn func(*T) error) error {
	return walutil.ReplayFrom[T](
		walPath,
		maxWALLineSize,
		seqAfter,
		parse,
		func(rec *T) int64 { return walutil.ExtractSeqFromAnyJSON(rec) },
		fn,
	)
}

func parseInterrupt(line []byte) (*InterruptRecord, error) {
	var rec InterruptRecord
	if err := json.Unmarshal(line, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

func parseAck(line []byte) (*AckRecord, error) {
	var rec AckRecord
	if err := json.Unmarshal(line, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

// AckState is the latest ack per (profile, dedupe_key).
type AckState struct {
	AckedAt   time.Time
	AckedBy   string
	ExpiresAt time.Time
	HasExpiry bool
}

func ackKey(profile, dedupeKey string) string {
	return strings.TrimSpace(profile) + ackKeySeparator + strings.TrimSpace(dedupeKey)
}

// LoadAcksIncremental loads ack state by replaying the ack WAL from the checkpoint.
// It returns the in-memory map plus the updated checkpoint it wrote.
func LoadAcksIncremental(projectRoot string) (map[string]AckState, error) {
	aw, err := newWAL[AckRecord](projectRoot, policyInterruptAckWALFileName)
	if err != nil {
		return nil, err
	}
	defer aw.Close()

	ck := readCheckpoint(aw.CheckpointPath())
	state := make(map[string]AckState)
	maxSeq := ck.LastReplayedSeq
	now := time.Now().UTC()
	err = ReplayFrom[AckRecord](aw.Path(), 0, parseAck, func(rec *AckRecord) error {
		if rec.Seq > maxSeq {
			maxSeq = rec.Seq
		}
		k := ackKey(rec.Profile, rec.DedupeKey)
		st := AckState{AckedAt: rec.Ts, AckedBy: rec.AckedBy}
		if rec.ExpiresAtRFC3339 != emptyValue {
			if t, e := time.Parse(time.RFC3339, rec.ExpiresAtRFC3339); e == nil {
				st.ExpiresAt = t
				st.HasExpiry = true
				if now.After(t) {
					delete(state, k)
					return nil
				}
			}
		}
		state[k] = st
		return nil
	})
	if err != nil {
		return nil, err
	}
	if maxSeq > ck.LastReplayedSeq {
		ck.LastReplayedSeq = maxSeq
		if ck.LastCompactedSeq > ck.LastReplayedSeq {
			ck.LastReplayedSeq = ck.LastCompactedSeq
		}
		_ = writeCheckpointAtomic(aw.CheckpointPath(), ck)
	}
	return state, nil
}

// CompactAckWAL compacts the ack WAL by rewriting it to one record per live (profile,dedupe_key).
// Expired records are dropped.
func CompactAckWAL(projectRoot string) error {
	aw, err := newWAL[AckRecord](projectRoot, policyInterruptAckWALFileName)
	if err != nil {
		return err
	}
	defer aw.Close()

	ck := readCheckpoint(aw.CheckpointPath())
	latest := make(map[string]AckRecord)
	var maxSeq int64
	now := time.Now().UTC()
	_ = ReplayFrom[AckRecord](aw.Path(), 0, parseAck, func(rec *AckRecord) error {
		if rec.Seq > maxSeq {
			maxSeq = rec.Seq
		}
		if rec.ExpiresAtRFC3339 != emptyValue {
			if t, e := time.Parse(time.RFC3339, rec.ExpiresAtRFC3339); e == nil && now.After(t) {
				return nil
			}
		}
		k := ackKey(rec.Profile, rec.DedupeKey)
		if prev, ok := latest[k]; !ok || rec.Seq > prev.Seq {
			latest[k] = *rec
		}
		return nil
	})

	// Rewrite WAL atomically
	tmp := aw.Path() + tmpFileSuffix
	f, err := fileutil.OpenFile(tmp, fileutil.O_CREATE|fileutil.O_WRONLY|fileutil.O_TRUNC, walFilePerm)
	if err != nil {
		return err
	}
	bw := bufio.NewWriter(f)
	keys := make([]string, 0, len(latest))
	for k := range latest {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	// Important: renumber seq in the compacted WAL so checkpoint-based replay works.
	// The checkpoint tracks the last replayed seq, so if we preserve large historical seqs and
	// also advance the checkpoint to that historical max, a subsequent replay would skip all records.
	var newSeq int64 = seqInitialValue
	for _, k := range keys {
		rec := latest[k]
		rec.Seq = newSeq
		newSeq++
		line, err := json.Marshal(rec)
		if err != nil {
			continue
		}
		if _, err := bw.Write(line); err != nil {
			_ = f.Close()
			return err
		}
		_ = bw.WriteByte('\n')
	}
	if err := bw.Flush(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := fileutil.Rename(tmp, aw.Path()); err != nil {
		return err
	}

	if maxSeq > 0 {
		ck.LastCompactedSeq = maxSeq
		// Reset replay checkpoint to 0 so next LoadAcksIncremental replays the compacted WAL.
		// LoadAcksIncremental will then advance LastReplayedSeq to the new (small) max seq.
		ck.LastReplayedSeq = 0
		_ = writeCheckpointAtomic(aw.CheckpointPath(), ck)
	}
	return nil
}

// AppendInterrupt appends a policy interrupt record and syncs it to disk.
func AppendInterrupt(projectRoot string, rec InterruptRecord) error {
	w, err := newWAL[InterruptRecord](projectRoot, policyInterruptWALFileName)
	if err != nil {
		return err
	}
	defer w.Close()
	if err := w.Append(func(seq int64, ts time.Time) InterruptRecord {
		rec.Seq = seq
		rec.Ts = ts
		if rec.Profile == emptyValue {
			rec.Profile = defaultPolicyProfile
		}
		return rec
	}, maxWALLineSize); err != nil {
		return err
	}
	return w.Sync()
}

// AppendAck appends an ack record and syncs it to disk.
func AppendAck(projectRoot string, rec AckRecord) error {
	w, err := newWAL[AckRecord](projectRoot, policyInterruptAckWALFileName)
	if err != nil {
		return err
	}
	defer w.Close()
	if err := w.Append(func(seq int64, ts time.Time) AckRecord {
		rec.Seq = seq
		rec.Ts = ts
		if rec.Profile == emptyValue {
			rec.Profile = defaultPolicyProfile
		}
		return rec
	}, maxWALLineSize); err != nil {
		return err
	}
	return w.Sync()
}

// LoadLatestCriticalUnacked returns the most recent critical, ack_required interrupt that is not acked.
func LoadLatestCriticalUnacked(projectRoot string, acks map[string]AckState) (*InterruptRecord, error) {
	iw, err := newWAL[InterruptRecord](projectRoot, policyInterruptWALFileName)
	if err != nil {
		return nil, err
	}
	defer iw.Close()

	var latest *InterruptRecord
	now := time.Now().UTC()
	err = ReplayFrom[InterruptRecord](iw.Path(), 0, parseInterrupt, func(rec *InterruptRecord) error {
		if rec.Profile == emptyValue {
			rec.Profile = defaultPolicyProfile
		}
		if rec.Severity != SeverityCritical || !rec.AckRequired || rec.DedupeKey == emptyValue {
			return nil
		}
		if rec.ExpiresAtRFC3339 != emptyValue {
			if t, e := time.Parse(time.RFC3339, rec.ExpiresAtRFC3339); e == nil && now.After(t) {
				return nil
			}
		}
		if _, ok := acks[ackKey(rec.Profile, rec.DedupeKey)]; ok {
			return nil
		}
		if latest == nil || rec.Seq > latest.Seq {
			cp := *rec
			latest = &cp
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return latest, nil
}

// LoadCriticalUnacked returns critical, ack-required interrupts that are not yet acknowledged.
// Results are sorted by sequence descending (most recent first). If limit > 0, returns up to limit items.
func LoadCriticalUnacked(projectRoot string, acks map[string]AckState, limit int) ([]InterruptRecord, error) {
	iw, err := newWAL[InterruptRecord](projectRoot, policyInterruptWALFileName)
	if err != nil {
		return nil, err
	}
	defer iw.Close()

	now := time.Now().UTC()
	out := make([]InterruptRecord, 0, 16)
	err = ReplayFrom[InterruptRecord](iw.Path(), 0, parseInterrupt, func(rec *InterruptRecord) error {
		if rec.Profile == emptyValue {
			rec.Profile = defaultPolicyProfile
		}
		if rec.Severity != SeverityCritical || !rec.AckRequired || rec.DedupeKey == emptyValue {
			return nil
		}
		if rec.ExpiresAtRFC3339 != emptyValue {
			if t, e := time.Parse(time.RFC3339, rec.ExpiresAtRFC3339); e == nil && now.After(t) {
				return nil
			}
		}
		if _, ok := acks[ackKey(rec.Profile, rec.DedupeKey)]; ok {
			return nil
		}
		out = append(out, *rec)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seq > out[j].Seq })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
