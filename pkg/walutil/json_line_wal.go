package walutil

import (
	"bufio"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	emptyValue            = ""
	walDirPerm            = 0o755
	walFilePerm           = 0o600
	defaultScannerBufCap  = 4096
	defaultScannerMaxSize = 64 * 1024
	errWALEmptyProject    = "WAL requires non-empty project root"
	errWALEmptyFilename   = "WAL requires non-empty filename"
	errWALEmptyCheckpoint = "WAL requires non-empty checkpoint extension"
	errCreateWALDirFmt    = "create WAL dir: %w"
	errOpenWALFileFmt     = "open WAL file: %w"
	errMarshalWALFmt      = "marshal WAL record: %w"
	errWALLineTooLargeFmt = "WAL line too large: %d"
	jsonFieldSeq          = "seq"
)

// JSONLineWAL is an append-only WAL storing one JSON object per line.
// Thread-safe: Append and Sync may be called concurrently; ReplayFrom is for a single reader.
type JSONLineWAL[T any] struct {
	mu      sync.Mutex
	file    *fileutil.File
	bw      *bufio.Writer
	path    string
	ckPath  string
	nextSeq int64
}

// OpenJSONLineWAL creates or opens a WAL under projectRoot/.zqk/wal/<filename>.
// The checkpoint path is <walPath><checkpointExt>.
func OpenJSONLineWAL[T any](projectRoot, filename, checkpointExt string, maxLineSize int) (*JSONLineWAL[T], error) {
	return OpenJSONLineWALWithNextSeqFunc[T](projectRoot, filename, checkpointExt, maxLineSize, func(walPath, _ string) (int64, error) {
		lastSeq, err := ReadLastSeqFromJSONLines(walPath, maxLineSize)
		if err != nil {
			return 1, err
		}
		return lastSeq + 1, nil
	})
}

// OpenJSONLineWALWithNextSeqFunc is like OpenJSONLineWAL but allows callers to customize nextSeq.
// This is useful when compaction can leave the WAL file empty while the checkpoint advances beyond
// the last on-disk sequence number.
func OpenJSONLineWALWithNextSeqFunc[T any](
	projectRoot, filename, checkpointExt string,
	maxLineSize int,
	nextSeqFn func(walPath, ckPath string) (int64, error),
) (*JSONLineWAL[T], error) {
	if projectRoot == emptyValue {
		return nil, errors.New(errWALEmptyProject)
	}
	if filename == emptyValue {
		return nil, errors.New(errWALEmptyFilename)
	}
	if checkpointExt == emptyValue {
		return nil, errors.New(errWALEmptyCheckpoint)
	}
	dir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.WalDir)
	if err := fileutil.MkdirAll(dir, walDirPerm); err != nil {
		return nil, errfmt.Errorf(errCreateWALDirFmt, err)
	}
	path := filepath.Join(dir, filename)
	ckPath := path + checkpointExt

	f, err := fileutil.OpenFile(path, fileutil.O_CREATE|fileutil.O_WRONLY|fileutil.O_APPEND, walFilePerm)
	if err != nil {
		return nil, errfmt.Errorf(errOpenWALFileFmt, err)
	}

	w := &JSONLineWAL[T]{file: f, bw: bufio.NewWriter(f), path: path, ckPath: ckPath}
	if nextSeqFn == nil {
		nextSeqFn = func(walPath, _ string) (int64, error) {
			lastSeq, err := ReadLastSeqFromJSONLines(walPath, maxLineSize)
			if err != nil {
				return 1, err
			}
			return lastSeq + 1, nil
		}
	}
	nextSeq, err := nextSeqFn(path, ckPath)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if nextSeq < 1 {
		nextSeq = 1
	}
	w.nextSeq = nextSeq
	return w, nil
}

func (w *JSONLineWAL[T]) Append(assign func(seq int64, ts time.Time) T, maxLineSize int) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	seq := w.nextSeq
	w.nextSeq++
	ts := time.Now().UTC()
	ev := assign(seq, ts)
	line, err := json.Marshal(ev)
	if err != nil {
		return errfmt.Errorf(errMarshalWALFmt, err)
	}
	if maxLineSize > 0 && len(line) > maxLineSize {
		return errfmt.Errorf(errWALLineTooLargeFmt, len(line))
	}
	if _, err := w.bw.Write(line); err != nil {
		return err
	}
	return w.bw.WriteByte('\n')
}

func (w *JSONLineWAL[T]) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.bw.Flush(); err != nil {
		return err
	}
	return w.file.Sync()
}

// CloseAfterFlush flushes bw, syncs dirty pages to physical disk, then closes f.
// Flush, sync, and close errors are joined and returned (BLI-WAL-CLOSE-FSYNC-001).
// The file descriptor is always closed even if Flush or Sync fails.
func CloseAfterFlush(bw *bufio.Writer, f *fileutil.File) error {
	var flushErr, syncErr, closeErr error
	if bw != nil {
		flushErr = bw.Flush()
	}
	if f != nil {
		if flushErr == nil {
			syncErr = f.Sync()
		}
		closeErr = f.Close()
	}
	return errors.Join(flushErr, syncErr, closeErr)
}

func (w *JSONLineWAL[T]) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := CloseAfterFlush(w.bw, w.file)
	w.file = nil
	return err
}

func (w *JSONLineWAL[T]) Path() string           { return w.path }
func (w *JSONLineWAL[T]) CheckpointPath() string { return w.ckPath }

// ReadLastSeqFromJSONLines scans the file and returns the max "seq" value found, or 0 if missing/unreadable.
func ReadLastSeqFromJSONLines(path string, maxLineSize int) (int64, error) {
	f, err := fileutil.Open(path)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	defer f.Close()
	var last int64
	sc := bufio.NewScanner(f)
	buf := make([]byte, 0, defaultScannerBufCap)
	if maxLineSize <= 0 {
		maxLineSize = defaultScannerMaxSize
	}
	sc.Buffer(buf, maxLineSize)
	for sc.Scan() {
		seq := ExtractSeqFromJSON(sc.Bytes())
		if seq > last {
			last = seq
		}
	}
	return last, sc.Err()
}

// ExtractSeqFromJSON best-effort extracts "seq" from a JSON object.
// Prefer ExtractSeqFromJSONLine for hot skip paths (no map alloc).
func ExtractSeqFromJSON(line []byte) int64 {
	var raw map[string]any
	if err := json.Unmarshal(line, &raw); err != nil {
		return 0
	}
	switch v := raw[jsonFieldSeq].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	default:
		return 0
	}
}

// ExtractSeqFromAnyJSON is a convenience helper that marshals v then extracts "seq".
// Useful when callers already have a parsed record type but don’t want to add type-specific seq accessors.
func ExtractSeqFromAnyJSON(v any) int64 {
	b, err := json.Marshal(v)
	if err != nil {
		return 0
	}
	return ExtractSeqFromJSON(b)
}
