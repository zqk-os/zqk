package walutil

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strconv"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// ReplayCursor is a durable (or in-memory) resume point for JSON-line WAL readers.
// Seq is exclusive (process records with seq > Seq). Offset is the byte position
// immediately after the last fully consumed line (0 = start of file).
type ReplayCursor struct {
	Seq    int64 `json:"seq"`
	Offset int64 `json:"offset"`
}

// ReplayStats summarizes one ReplayFromCursor pass.
type ReplayStats struct {
	Cursor    ReplayCursor
	Delivered int // records passed to fn (seq > cursor.Seq)
	Scanned   int // lines read this pass
}

// ReplayFrom reads records from the WAL starting at seqAfter (exclusive).
// Prefer ReplayFromCursor when the caller can retain a byte offset across polls —
// otherwise each call re-scans from byte 0 (CPU cost grows with WAL size).
func ReplayFrom[T any](walPath string, maxLineSize int, seqAfter int64, parse func([]byte) (*T, error), extractSeq func(*T) int64, fn func(*T) error) error {
	_, err := ReplayFromCursor[T](walPath, maxLineSize, ReplayCursor{Seq: seqAfter}, parse, extractSeq, fn)
	return err
}

// ReplayFromCursor seeks to cursor.Offset (when > 0), then reads new lines.
// On a truncated/rewritten WAL (seek past EOF or offset not at a line boundary after
// compaction), falls back to a full scan from offset 0 filtered by cursor.Seq.
// Returned Cursor.Offset is always at a line boundary (or EOF).
func ReplayFromCursor[T any](
	walPath string,
	maxLineSize int,
	cursor ReplayCursor,
	parse func([]byte) (*T, error),
	extractSeq func(*T) int64,
	fn func(*T) error,
) (ReplayStats, error) {
	stats := ReplayStats{Cursor: cursor}
	info, err := fileutil.Stat(walPath)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return stats, nil
		}
		return stats, err
	}
	size := info.Size()
	// Stamp skip: at EOF there are no new bytes. Stat is cheaper than Open (CRIT-1790151410719520000-18e0643b).
	if size == cursor.Offset {
		return stats, nil
	}

	f, err := fileutil.Open(walPath)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return stats, nil
		}
		return stats, err
	}
	defer f.Close()

	start := cursor.Offset
	if start < 0 || start > size {
		start = 0
	}
	if start > 0 {
		if _, err := f.Seek(start, io.SeekStart); err != nil {
			start = 0
			if _, err := f.Seek(0, io.SeekStart); err != nil {
				return stats, err
			}
		}
	}

	return replayLines[T](f, start, maxLineSize, cursor.Seq, parse, extractSeq, fn)
}

func replayLines[T any](
	f *fileutil.File,
	startOffset int64,
	maxLineSize int,
	seqAfter int64,
	parse func([]byte) (*T, error),
	extractSeq func(*T) int64,
	fn func(*T) error,
) (ReplayStats, error) {
	stats := ReplayStats{Cursor: ReplayCursor{Seq: seqAfter, Offset: startOffset}}
	if maxLineSize <= 0 {
		maxLineSize = defaultScannerMaxSize
	}
	r := bufio.NewReaderSize(f, defaultScannerBufCap)
	offset := startOffset
	for {
		line, err := r.ReadBytes('\n')
		n := len(line)
		if n > 0 {
			offset += int64(n)
			stats.Scanned++
			payload := line
			if payload[len(payload)-1] == '\n' {
				payload = payload[:len(payload)-1]
			}
			if len(payload) > 0 && payload[len(payload)-1] == '\r' {
				payload = payload[:len(payload)-1]
			}
			if maxLineSize > 0 && len(payload) > maxLineSize {
				stats.Cursor.Offset = offset
				continue
			}
			// Cheap skip for already-applied records (avoid full unmarshal).
			if seq := ExtractSeqFromJSONLine(payload); seq > 0 && seq <= seqAfter {
				stats.Cursor.Seq = seq
				stats.Cursor.Offset = offset
				continue
			}
			rec, parseErr := parse(payload)
			if parseErr != nil || rec == nil {
				stats.Cursor.Offset = offset
				continue
			}
			seq := extractSeq(rec)
			if seq <= seqAfter {
				if seq > stats.Cursor.Seq {
					stats.Cursor.Seq = seq
				}
				stats.Cursor.Offset = offset
				continue
			}
			if fnErr := fn(rec); fnErr != nil {
				return stats, fnErr
			}
			stats.Delivered++
			stats.Cursor.Seq = seq
			stats.Cursor.Offset = offset
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return stats, err
		}
	}
	return stats, nil
}

// ExtractSeqFromJSONLine extracts "seq" without allocating a map when possible.
func ExtractSeqFromJSONLine(line []byte) int64 {
	const key = `"seq"`
	idx := bytes.Index(line, []byte(key))
	if idx < 0 {
		return ExtractSeqFromJSON(line)
	}
	rest := line[idx+len(key):]
	rest = bytes.TrimLeft(rest, " \t\r\n:")
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	if end == 0 {
		return ExtractSeqFromJSON(line)
	}
	v, err := strconv.ParseInt(string(rest[:end]), 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// FormatReplayCursorCheckpoint encodes seq+offset for durable IDE/checkpoint files.
// Plain integer seq remains accepted by ParseReplayCursorCheckpoint for back-compat.
func FormatReplayCursorCheckpoint(c ReplayCursor) []byte {
	b, err := json.Marshal(c)
	if err != nil {
		return []byte(strconv.FormatInt(c.Seq, 10))
	}
	return b
}

// ParseReplayCursorCheckpoint accepts JSON {"seq":N,"offset":M} or a plain integer seq.
func ParseReplayCursorCheckpoint(data []byte) (ReplayCursor, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return ReplayCursor{}, nil
	}
	if data[0] == '{' {
		var c ReplayCursor
		if err := json.Unmarshal(data, &c); err != nil {
			return ReplayCursor{}, err
		}
		return c, nil
	}
	seq, err := strconv.ParseInt(string(data), 10, 64)
	if err != nil {
		return ReplayCursor{}, err
	}
	return ReplayCursor{Seq: seq}, nil
}
