// Package storage: object WAL (write-ahead log) for Option B write-behind.
// Provides durability and crash recovery for create/update/delete before background persist.

package wal

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"sync"
	"sync/atomic"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/validation"
	"github.com/lanceman/zqk/pkg/walutil"
)

const (
	ObjectWALFileName        = "object.wal"
	ObjectWALCheckpointExt   = ".checkpoint"
	LegacyObjectWALFileName  = "object_wal"     // pre-rename; migrated on first open
	LegacyObjectWALFileName2 = "object_wal.wal" // after first consistency rename; migrated to object.wal
	// MaxWALLineSize is the maximum supported line length when reading the WAL.
	// Records can contain base64-encoded YAML (data_base64); default bufio.Scanner limit (64KB) is too small.
	MaxWALLineSize = 2 * 1024 * 1024 // 2 MiB per line

	objectWALFileName        = ObjectWALFileName
	objectWALCheckpointExt   = ObjectWALCheckpointExt
	legacyObjectWALFileName  = LegacyObjectWALFileName
	legacyObjectWALFileName2 = LegacyObjectWALFileName2
	maxWALLineSize           = MaxWALLineSize
)

// WALRecord is one log entry: create, update, or delete.
type WALRecord struct {
	Op      string `json:"op"` // "create", "update", "delete"
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Seq     int64  `json:"seq"`
	DataB64 string `json:"data_base64,omitempty"` // base64-encoded YAML for create/update
}

// ObjectWAL is an append-only write-ahead log for object write-behind.
// Thread-safe: Append and Sync may be called concurrently; Replay during init only.
type ObjectWAL struct {
	mu          sync.Mutex
	file        *fileutil.File
	bw          *bufio.Writer
	path        string
	ckPath      string
	nextSeq     int64
	projectRoot string
}

// legacyWALPathWithContent returns the path to a legacy WAL file that exists and has non-zero size,
// preferring object_wal.wal over object_wal. Empty string if none.
func legacyWALPathWithContent(dir string) string {
	for _, legacyName := range []string{legacyObjectWALFileName2, legacyObjectWALFileName} {
		legacy := filepath.Join(dir, legacyName)
		if st, err := fileutil.Stat(legacy); err == nil && st.Size() > 0 {
			return legacy
		}
	}
	return ""
}

// migrateObjectWALToCanonicalNames renames legacy WAL files to object.wal / object.wal.checkpoint.
// Handles: object_wal (original), object_wal.wal (after first consistency rename). Idempotent.
func migrateObjectWALToCanonicalNames(projectRoot string) error {
	if projectRoot == emptyValue {
		return nil
	}
	dir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.WalDir)
	canonical := filepath.Join(dir, objectWALFileName)
	ckCanonical := canonical + objectWALCheckpointExt
	if st, err := fileutil.Stat(canonical); err == nil {
		if st.Size() > 0 {
			return nil
		}
		// Empty object.wal blocks the rename below: something (e.g. recycle/touch) created the
		// canonical name before legacy could be migrated, leaving both object.wal (empty) and
		// object_wal (data). Drop the stale empty pair so legacy can become canonical.
		if legacyWALPathWithContent(dir) != emptyValue {
			logging.LogSwallowedError(fileutil.RemoveFile(canonical))
			logging.LogSwallowedError(fileutil.RemoveFile(ckCanonical))
		} else {
			return nil
		}
	}

	for _, legacyName := range []string{legacyObjectWALFileName2, legacyObjectWALFileName} {
		legacy := filepath.Join(dir, legacyName)
		if _, err := fileutil.Stat(legacy); err != nil {
			continue
		}
		if err := fileutil.Rename(legacy, canonical); err != nil {
			return errfmt.Errorf(ConstStreamMigrateObjectWalRenameStrToStrErr, legacyName, objectWALFileName, err)
		}
		ckLegacy := legacy + objectWALCheckpointExt
		if _, err := fileutil.Stat(ckLegacy); err == nil {
			var err_swallow_40 = fileutil.Rename(ckLegacy, ckCanonical)
			if err_swallow_40 != nil {
				logging.LogSwallowedError(

					// NewObjectWAL creates or opens the WAL under projectRoot/.zqk/wal/object.wal.
					// Migrates legacy object_wal or object_wal.wal to object.wal if present.
					// Caller must call Close when done. Replay (if any) should be done after construction
					// before appending new records.
					err_swallow_40)
			}
		}
		return nil
	}
	return nil
}

// sanitizeTornTailOnOpen inspects the WAL file before opening in append mode.
// If an abrupt process crash or power loss left an incomplete/torn line at EOF
// (without trailing newline), this truncates the file back to the last valid newline
// so subsequent writes do not get merged with torn bytes (CRIT-CEF-R14-RCV-CRASH-REPLAY-001).
func sanitizeTornTailOnOpen(path string) error {
	f, err := fileutil.OpenFile(path, fileutil.O_RDWR, paths.FilePerm600)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return err
	}
	size := fi.Size()
	if size == 0 {
		return nil
	}

	lastByte := make([]byte, 1)
	if _, err := f.ReadAt(lastByte, size-1); err != nil {
		return err
	}
	if lastByte[0] == '\n' {
		return nil
	}

	chunkSize := int64(64 * 1024)
	if chunkSize > size {
		chunkSize = size
	}
	buf := make([]byte, chunkSize)
	offset := size - chunkSize
	if _, err := f.ReadAt(buf, offset); err != nil {
		return err
	}

	lastNewlineIdx := bytes.LastIndexByte(buf, '\n')
	var truncatePos int64
	if lastNewlineIdx >= 0 {
		truncatePos = offset + int64(lastNewlineIdx) + 1
	} else {
		for offset > 0 {
			nextChunk := chunkSize
			if nextChunk > offset {
				nextChunk = offset
			}
			offset -= nextChunk
			buf = make([]byte, nextChunk)
			if _, err := f.ReadAt(buf, offset); err != nil {
				return err
			}
			idx := bytes.LastIndexByte(buf, '\n')
			if idx >= 0 {
				truncatePos = offset + int64(idx) + 1
				break
			}
		}
	}

	return f.Truncate(truncatePos)
}

func NewObjectWAL(projectRoot string) (*ObjectWAL, error) {
	if projectRoot == emptyValue {
		return nil, errfmt.Errorf(ConstStreamObjectWalRequiresNonEmptyProjectRoot)
	}
	dir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.WalDir)
	if err := fileutil.EnsureDir(dir); err != nil {
		return nil, errfmt.Newf(ConstStreamCreateWalDir).Wrap(err)
	}
	if err := migrateObjectWALToCanonicalNames(projectRoot); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, objectWALFileName)
	ckPath := path + objectWALCheckpointExt

	if err := sanitizeTornTailOnOpen(path); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
			Error("failed to sanitize torn WAL tail on open", err).Log()
	}

	f, err := fileutil.OpenFile(path, fileutil.O_CREATE|fileutil.O_WRONLY|fileutil.O_APPEND, paths.FilePerm600)
	if err != nil {
		return nil, errfmt.Newf("open WAL file").Wrap(err)
	}

	// Initialize nextSeq from file (count lines or read last seq). For simplicity we use
	// a checkpoint file that stores last written seq; on create we set nextSeq from
	// last line in file or 0. We'll set nextSeq from ReadLastSeq on startup.
	w := &ObjectWAL{
		file:        f,
		bw:          bufio.NewWriter(f),
		path:        path,
		ckPath:      ckPath,
		projectRoot: projectRoot,
	}
	lastSeq, _err_83635973 := w.readLastSeqFromFile()
	if _err_83635973 != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// readLastSeqFromFile scans the WAL file and returns the highest seq (0 if empty or error).
			// Supports v1, v2 single, and v2 batch lines. Call with w.mu held or during init before any Append.
			Error(ConstStreamSwallowedErrorValN,

				_err_83635973).Log()
	}
	atomic.StoreInt64(&w.nextSeq, lastSeq+1)
	return w, nil
}

func (w *ObjectWAL) readLastSeqFromFile() (int64, error) {
	f, err := fileutil.Open(w.path)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	defer f.Close()
	var last int64
	sc := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, maxWALLineSize)
	for sc.Scan() {
		line := sc.Bytes()
		seq := getMaxSeqFromLine(line)
		if seq > last {
			last = seq
		}
	}
	return last, sc.Err()
}

// Append appends a record to the WAL and assigns Seq. Uses compact v2 format (short keys, positional payload for known kinds). Does not sync; call Sync for durability.
// Rejects records with object ID longer than validation.MaxObjectIDLength or marshalled line larger than maxWALLineSize (see WAL_LONG_RECORD_SAFEGUARDS.md).
func (w *ObjectWAL) Append(rec *WALRecord) error {
	if len(rec.ID) > validation.MaxObjectIDLength {
		return errfmt.Errorf(ConstStreamWalRecordObjectIdLengthIntExceedsMaxIntKindStr, len(rec.ID), validation.MaxObjectIDLength, rec.Kind)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	seq := atomic.AddInt64(&w.nextSeq, 1) - 1
	rec.Seq = seq
	line, err := marshalCompactRecord(rec)
	if err != nil {
		return errfmt.Newf(ConstStreamMarshalWalRecord).Wrap(err)
	}
	if len(line) > maxWALLineSize {
		return errfmt.Errorf(ConstStreamWalRecordMarshalledSizeIntExceedsMaxLineSizeInt, len(line), maxWALLineSize, rec.Kind, rec.ID)
	}
	if w.bw == nil || w.file == nil {
		return errfmt.Errorf("WAL closed; reopen required before append")
	}
	if _, err := w.bw.Write(line); err != nil {
		return errfmt.Newf("write WAL").Wrap(err)
	}
	if err := w.bw.WriteByte('\n'); err != nil {
		return errfmt.Newf(ConstStreamWriteWalNewline).Wrap(err)
	}
	return nil
}

// AppendBatch appends multiple records in one WAL line (v2 batch format). Assigns sequential Seq to each record. Does not sync; call Sync for durability.
// Rejects if any record has object ID longer than validation.MaxObjectIDLength or if the marshalled batch line would exceed maxWALLineSize.
func (w *ObjectWAL) AppendBatch(recs []*WALRecord) error {
	if len(recs) == 0 {
		return nil
	}
	for _, rec := range recs {
		if len(rec.ID) > validation.MaxObjectIDLength {
			idPreview := rec.ID
			if len(idPreview) > 64 {
				idPreview = idPreview[:64] + "..."
			}
			return errfmt.Errorf(ConstStreamWalBatchRecordObjectIdLengthIntExceedsMaxIntKind, len(rec.ID), validation.MaxObjectIDLength, rec.Kind, idPreview)
		}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for i := range recs {
		recs[i].Seq = atomic.AddInt64(&w.nextSeq, 1) - 1
	}
	line, err := marshalCompactBatch(recs)
	if err != nil {
		return errfmt.Newf(ConstStreamMarshalWalBatch).Wrap(err)
	}
	if len(line) > maxWALLineSize {
		return errfmt.Errorf(ConstStreamWalBatchMarshalledSizeIntExceedsMaxLineSizeInt, len(line), maxWALLineSize, len(recs))
	}
	if w.bw == nil || w.file == nil {
		return errfmt.Errorf("WAL closed; reopen required before append")
	}
	if _, err := w.bw.Write(line); err != nil {
		return errfmt.Newf(ConstStreamWriteWalBatch).Wrap(err)
	}
	if err := w.bw.WriteByte('\n'); err != nil {
		return errfmt.Newf(ConstStreamWriteWalNewline).Wrap(err)
	}
	return nil
}

// Sync flushes the buffer and syncs the WAL file to disk.
func (w *ObjectWAL) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.bw == nil || w.file == nil {
		return errfmt.Errorf("WAL closed; reopen required before sync")
	}
	if err := w.bw.Flush(); err != nil {
		return err
	}
	return w.file.Sync()
}

// AppendWithSync appends a record to the WAL and immediately syncs to disk (fsync).
func (w *ObjectWAL) AppendWithSync(rec *WALRecord) error {
	if err := w.Append(rec); err != nil {
		return err
	}
	return w.Sync()
}

// AppendBatchWithSync appends multiple records and immediately syncs to disk (fsync).
func (w *ObjectWAL) AppendBatchWithSync(records []*WALRecord) error {
	if err := w.AppendBatch(records); err != nil {
		return err
	}
	return w.Sync()
}

// Close closes the WAL file (flushes first). Idempotent.
// Flush errors are returned (CEF E:F-REL-003 / BLI-CEF-R2-REL-WAL-CLOSE).
func (w *ObjectWAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.closeLocked()
}

func (w *ObjectWAL) closeLocked() error {
	if w.file == nil {
		return nil
	}
	err := walutil.CloseAfterFlush(w.bw, w.file)
	w.file = nil
	w.bw = nil
	return err
}

func (w *ObjectWAL) reopenLocked() error {
	f, err := fileutil.OpenFile(w.path, fileutil.O_CREATE|fileutil.O_WRONLY|fileutil.O_APPEND, paths.FilePerm600)
	if err != nil {
		return errfmt.Newf("open WAL file").Wrap(err)
	}
	w.file = f
	w.bw = bufio.NewWriter(f)
	lastSeq, seqErr := w.readLastSeqFromFile()
	if seqErr != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
			Error(ConstStreamSwallowedErrorValN, seqErr).Log()
	}
	atomic.StoreInt64(&w.nextSeq, lastSeq+1)
	return nil
}

// ReopenAfterCompact reopens the WAL path after CompactWAL renamed a new file into place.
// Prefer CompactInPlace from TryCompactWAL so Close+rename+reopen stay under w.mu.
func (w *ObjectWAL) ReopenAfterCompact() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.closeLocked(); err != nil {
		return err
	}
	return w.reopenLocked()
}

// CompactInPlace closes the write FD, runs CompactWAL (path rename), and reopens on the same
// *ObjectWAL while holding w.mu so concurrent Append/Sync cannot write to a deleted inode.
func (w *ObjectWAL) CompactInPlace(projectRoot string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.closeLocked(); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
			Error(ErrMsgSwallowedError, err).Log()
	}
	compactErr := CompactWAL(projectRoot)
	if reopenErr := w.reopenLocked(); reopenErr != nil {
		if compactErr != nil {
			return compactErr
		}
		return reopenErr
	}
	return compactErr
}

// GetCheckpointPath returns the checkpoint file path (for tests or external tools).
func (w *ObjectWAL) GetCheckpointPath() string { return w.ckPath }

// GetPath returns the WAL file path.
func (w *ObjectWAL) GetPath() string { return w.path }

// ReadAppliedSeq returns the last applied sequence number from the checkpoint file (0 if missing).
func ReadAppliedSeq(projectRoot string) (int64, error) {
	if projectRoot == emptyValue {
		return 0, nil
	}
	logging.LogSwallowedError(migrateObjectWALToCanonicalNames(projectRoot))
	path := filepath.Join(projectRoot, paths.ProjectDataDir, paths.WalDir, objectWALFileName+objectWALCheckpointExt)
	data, err := fileutil.ReadFile(path)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return 0, nil // Empty file, treat as missing
	}
	var v struct {
		AppliedSeq int64 `json:"applied_seq"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return 0, err
	}
	return v.AppliedSeq, nil
}

// WriteAppliedSeq writes the applied sequence to the checkpoint file (used by worker after apply).
func WriteAppliedSeq(projectRoot string, seq int64) error {
	if projectRoot == emptyValue {
		return nil
	}
	dir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.WalDir)
	if err := fileutil.EnsureDir(dir); err != nil {
		return err
	}
	path := filepath.Join(dir, objectWALFileName+objectWALCheckpointExt)
	data, err := json.Marshal(struct {
		AppliedSeq int64 `json:"applied_seq"`
	}{AppliedSeq: seq})
	if err != nil {
		return err
	}
	return fileutil.WriteDurableSecureFile(path, data)
}

// Replay reads the WAL from the beginning and calls fn for each record with seq > appliedSeq.
// Does not modify the WAL. Call before starting the background worker (e.g. on startup).
func ReplayWAL(projectRoot string, appliedSeq int64, fn func(rec *WALRecord) error) error {
	_, _, err := ReplayWALChunk(projectRoot, appliedSeq, 0, fn) // 0 = no limit
	return err
}

// ReplayWALChunk replays up to limit records (seq > appliedSeq) from the WAL, calling fn for each.
// If limit <= 0, replays all. Returns (replayedCount, lastSeq, err).
// Use for chunked replay: replay N, process buffer, then call again with updated appliedSeq.
func ReplayWALChunk(projectRoot string, appliedSeq int64, limit int, fn func(rec *WALRecord) error) (replayed int, lastSeq int64, err error) {
	if projectRoot == emptyValue {
		return 0, appliedSeq, nil
	}
	logging.LogSwallowedError(migrateObjectWALToCanonicalNames(projectRoot))
	path := filepath.Join(projectRoot, paths.ProjectDataDir, paths.WalDir, objectWALFileName)
	f, err := fileutil.Open(path)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return 0, appliedSeq, nil
		}
		return 0, appliedSeq, err
	}
	defer f.Close()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	sc := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, maxWALLineSize)
	for sc.Scan() {
		if limit > 0 && replayed >= limit {
			return replayed, lastSeq, nil
		}

		lineBytes := sc.Bytes()
		maxSeq := getMaxSeqFromLine(lineBytes)
		if maxSeq > 0 && maxSeq <= appliedSeq {
			continue
		}

		records, err := parseWALLine(lineBytes)
		if err != nil {
			StorageLog(logger).Warn(LogEventStorageObjectWALReplaySkipLineWarn).
				String("line", sc.Text()).
				WithError(err).
				Log()
			continue
		}
		for _, rec := range records {
			if rec.Seq <= appliedSeq {
				continue
			}
			if err := fn(rec); err != nil {
				return replayed, rec.Seq, errfmt.Errorf(ConstStreamReplayRecordSeqIntErr, rec.Seq, err)
			}
			replayed++
			lastSeq = rec.Seq
		}
	}
	if err := sc.Err(); err != nil {
		return replayed, lastSeq, err
	}
	if replayed > 0 && limit <= 0 {
		StorageLog(logger).Info(LogEventStorageObjectWALReplayCompletedInfo).
			Int(ConstStreamRecordsApplied, replayed).
			ProjectRoot(projectRoot).
			Log()
	}
	return replayed, lastSeq, nil
}

// DecodeRecordData returns the decoded YAML bytes for a create/update record.
func (r *WALRecord) DecodeRecordData() ([]byte, error) {
	if r.DataB64 == emptyValue {
		return nil, nil
	}
	return base64.StdEncoding.DecodeString(r.DataB64)
}

// GetWALPath returns the path to the object WAL file for the given project root.
func GetWALPath(projectRoot string) string {
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.WalDir, objectWALFileName)
}

// CompactWAL removes applied entries from the WAL file, keeping only unapplied entries.
// This reduces WAL file size by removing entries that have already been processed.
// Callers that hold a live ObjectWAL write FD MUST Close (or use TryCompactWAL) before
// CompactWAL: os.Rename replaces the path and leaves open FDs on the deleted inode.
func CompactWAL(projectRoot string) error {
	if projectRoot == emptyValue {
		return errfmt.Errorf(ConstStreamProjectRootRequiredForWalCompaction)
	}
	if err := migrateObjectWALToCanonicalNames(projectRoot); err != nil {
		return err
	}

	// Read checkpoint to determine which entries to keep
	appliedSeq, err := ReadAppliedSeq(projectRoot)
	if err != nil {
		return errfmt.Newf(ConstStreamFailedToReadCheckpoint).Wrap(err)
	}

	walPath := filepath.Join(projectRoot, paths.ProjectDataDir, paths.WalDir, objectWALFileName)
	walFile, err := fileutil.Open(walPath)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return nil // No WAL file, nothing to compact
		}
		return errfmt.Newf(ConstStreamFailedToOpenWalFile).Wrap(err)
	}
	defer walFile.Close()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// Read all records and filter to unapplied ones
	var unappliedRecords []WALRecord
	scanner := bufio.NewScanner(walFile)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, maxWALLineSize)
	var totalLines, totalRecords, keptCount int64
	var minSeq, maxSeq int64

	for scanner.Scan() {
		totalLines++
		records, err := parseWALLine(scanner.Bytes())
		if err != nil {
			StorageLog(logger).Warn(LogEventStorageObjectWALCompactionSkipLineWarn).
				Int("line_number", int(totalLines)).
				WithError(err).
				Log()
			continue
		}
		for _, rec := range records {
			totalRecords++
			if rec.Seq > maxSeq {
				maxSeq = rec.Seq
			}
			if minSeq == 0 || rec.Seq < minSeq {
				minSeq = rec.Seq
			}
			if rec.Seq > appliedSeq {
				unappliedRecords = append(unappliedRecords, *rec)
				keptCount++
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return errfmt.Newf(ConstStreamFailedToReadWalFile).Wrap(err)
	}

	if appliedSeq == 0 && totalRecords > 0 {
		StorageLog(logger).Warn(LogEventStorageObjectWALCompactionAppliedSeqZeroWarn).
			Int("total_records", int(totalRecords)).
			Int("wal_lines", int(totalLines)).
			Log()
	}

	// If all entries are unapplied, no compaction needed (keep only when line count matches record count and all kept)
	if keptCount == totalRecords && totalRecords > 0 {
		StorageLog(logger).Info(LogEventStorageObjectWALCompactionNoRemovableInfo).
			Int("total_records", int(totalRecords)).
			Int("applied_seq", int(appliedSeq)).
			Int("min_seq", int(minSeq)).
			Int("max_seq", int(maxSeq)).
			String("hint", ConstStreamToReduceWalSizeRunTheSchedulerSoTheWriteBehind).
			Log()
		return nil
	}

	// Write compacted WAL to temporary file first (atomic operation)
	tempPath := walPath + ".tmp"
	tempFile, err := fileutil.OpenFile(tempPath, fileutil.O_CREATE|fileutil.O_WRONLY|fileutil.O_TRUNC, paths.FilePerm600)
	if err != nil {
		return errfmt.Newf(ConstStreamFailedToCreateTempWalFile).Wrap(err)
	}
	defer tempFile.Close()

	writer := bufio.NewWriter(tempFile)
	for _, rec := range unappliedRecords {
		line, err := marshalCompactRecord(&rec)
		if err != nil {
			logging.LogSwallowedError(tempFile.Close())
			logging.LogSwallowedError(fileutil.RemoveFile(tempPath))
			return errfmt.Newf(ConstStreamFailedToMarshalWalRecord).Wrap(err)
		}
		if _, err := writer.Write(line); err != nil {
			logging.LogSwallowedError(tempFile.Close())
			logging.LogSwallowedError(fileutil.RemoveFile(tempPath))
			return errfmt.Newf(ConstStreamFailedToWriteWalRecord).Wrap(err)
		}
		if err := writer.WriteByte('\n'); err != nil {
			logging.LogSwallowedError(tempFile.Close())
			logging.LogSwallowedError(fileutil.RemoveFile(tempPath))
			return errfmt.Newf(ConstStreamFailedToWriteWalNewline).Wrap(err)
		}
	}

	if err := writer.Flush(); err != nil {
		logging.LogSwallowedError(tempFile.Close())
		logging.LogSwallowedError(fileutil.RemoveFile(tempPath))
		return errfmt.Newf(ConstStreamFailedToFlushWalFile).Wrap(err)
	}

	if err := tempFile.Sync(); err != nil {
		logging.LogSwallowedError(tempFile.Close())
		logging.LogSwallowedError(fileutil.RemoveFile(tempPath))
		return errfmt.Newf(ConstStreamFailedToSyncWalFile).Wrap(err)
	}

	if err := tempFile.Close(); err != nil {
		logging.LogSwallowedError(fileutil.RemoveFile(tempPath))
		return errfmt.Newf(ConstStreamFailedToCloseTempWalFile).Wrap(err)
	}

	// Atomically replace old WAL with compacted version
	if err := fileutil.Rename(tempPath, walPath); err != nil {
		logging.LogSwallowedError(fileutil.RemoveFile(tempPath))
		return errfmt.Newf(ConstStreamFailedToReplaceWalFile).Wrap(err)
	}
	_ = fileutil.SyncDir(filepath.Dir(walPath))

	StorageLog(logger).Info(LogEventStorageObjectWALCompactionCompletedInfo).
		Int("total_records", int(totalRecords)).
		Int(ConstStreamRemovedEntries, int(totalRecords-keptCount)).
		Int("kept_entries", int(keptCount)).
		Int("applied_seq", int(appliedSeq)).
		Log()

	return nil
}

// ReadLastSeqFromTail seeks to the end of the WAL to read the last sequence number without scanning the entire file (K:F-L-PERFORMANCE-002).
func ReadLastSeqFromTail(projectRoot string) (int64, error) {
	walPath := GetWALPath(projectRoot)
	f, err := fileutil.Open(walPath)
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()

	st, err := f.Stat()
	if err != nil || st.Size() == 0 {
		return 0, err
	}

	readSize := int64(64 * 1024)
	if st.Size() < readSize {
		readSize = st.Size()
	}
	offset := st.Size() - readSize
	if _, err := f.Seek(offset, 0); err != nil {
		return 0, err
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, maxWALLineSize), maxWALLineSize)
	var lastSeq int64
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var rec WALRecord
		if err := json.Unmarshal(line, &rec); err == nil && rec.Seq > lastSeq {
			lastSeq = rec.Seq
		}
	}
	return lastSeq, scanner.Err()
}
