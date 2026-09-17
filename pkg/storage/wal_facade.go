package storage

// Facade boundary for upcoming decomposition. TRACK: BLI-TRACK-STORAGE-SPLIT-001

import (
	"time"

	"github.com/lanceman/zqk/pkg/storage/wal"
)

// WAL aliases forwarding to pkg/storage/wal
type WALRecord = wal.WALRecord
type ObjectWAL = wal.ObjectWAL

// Legacy and helper forwarders
const (
	ObjectWALFileName        = wal.ObjectWALFileName
	ObjectWALCheckpointExt   = wal.ObjectWALCheckpointExt
	LegacyObjectWALFileName  = wal.LegacyObjectWALFileName
	LegacyObjectWALFileName2 = wal.LegacyObjectWALFileName2
	MaxWALLineSize           = wal.MaxWALLineSize
	maxWALLineSize           = wal.MaxWALLineSize
	objectWALFileName        = wal.ObjectWALFileName
	objectWALCheckpointExt   = wal.ObjectWALCheckpointExt
	legacyObjectWALFileName  = wal.LegacyObjectWALFileName
	legacyObjectWALFileName2 = wal.LegacyObjectWALFileName2
)

func NewObjectWAL(projectRoot string) (*ObjectWAL, error) {
	return wal.NewObjectWAL(projectRoot)
}

func GetWALPath(projectRoot string) string {
	return wal.GetWALPath(projectRoot)
}

func ReadAppliedSeq(projectRoot string) (int64, error) {
	return wal.ReadAppliedSeq(projectRoot)
}

func WriteAppliedSeq(projectRoot string, seq int64) error {
	return wal.WriteAppliedSeq(projectRoot, seq)
}

func ReplayWALChunk(projectRoot string, appliedSeq int64, limit int, fn func(rec *WALRecord) error) (replayed int, lastSeq int64, err error) {
	return wal.ReplayWALChunk(projectRoot, appliedSeq, limit, fn)
}

func ReplayWAL(projectRoot string, appliedSeq int64, fn func(rec *WALRecord) error) error {
	return wal.ReplayWAL(projectRoot, appliedSeq, fn)
}

func parseWALLine(line []byte) ([]*wal.WALRecord, error) {
	return wal.ParseWALLine(line)
}

func getMaxSeqFromLine(line []byte) int64 {
	return wal.GetMaxSeqFromLine(line)
}

func WaitForWALProcessingEventDriven(projectRoot string, timeout time.Duration) error {
	return wal.WaitForWALProcessingEventDriven(projectRoot, timeout)
}

func ReadLastSeqFromTail(projectRoot string) (int64, error) {
	return wal.ReadLastSeqFromTail(projectRoot)
}

func CompactWAL(projectRoot string) error {
	return wal.CompactWAL(projectRoot)
}

func AcquireObjectWAL(projectRoot string) (*wal.ObjectWAL, error) {
	return wal.AcquireObjectWAL(projectRoot)
}

func ReleaseObjectWAL(projectRoot string, w *wal.ObjectWAL) error {
	return wal.ReleaseObjectWAL(projectRoot, w)
}

func tryClaimWriteBehindOwner(projectRoot string, owner any) bool {
	return wal.TryClaimWriteBehindOwner(projectRoot, owner)
}

func releaseWriteBehindOwner(projectRoot string, owner any) {
	wal.ReleaseWriteBehindOwner(projectRoot, owner)
}

func logWriteBehindOwnerSkipped(projectRoot string) {
	wal.LogWriteBehindOwnerSkipped(projectRoot)
}

func objectWALRefCountForTest(projectRoot string) int {
	return wal.ObjectWALRefCountForTest(projectRoot)
}
