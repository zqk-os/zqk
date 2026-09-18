package walutil

import (
	"bufio"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type failWriter struct {
	err error
}

func (w *failWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func TestCloseAfterFlush_ReturnsFlushError(t *testing.T) {
	flushErr := errors.New("flush boom")
	f, err := fileutil.CreateTemp(t.TempDir(), "wal")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	bw := bufio.NewWriter(&failWriter{err: flushErr})
	if _, err := bw.WriteString("buffered"); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	err = CloseAfterFlush(bw, f)
	if err == nil || !strings.Contains(err.Error(), flushErr.Error()) {
		t.Fatalf("CloseAfterFlush = %v, want flush error %v", err, flushErr)
	}
}

func TestJSONLineWAL_CloseReturnsFlushError(t *testing.T) {
	flushErr := errors.New("flush boom")
	f, err := fileutil.CreateTemp(t.TempDir(), "wal")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	w := &JSONLineWAL[map[string]any]{
		file: f,
		bw:   bufio.NewWriter(&failWriter{err: flushErr}),
	}
	if _, err := w.bw.WriteString("buffered"); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	err = w.Close()
	if err == nil || !strings.Contains(err.Error(), flushErr.Error()) {
		t.Fatalf("Close = %v, want flush error %v", err, flushErr)
	}
}

func TestCloseAfterFlush_FlushesAndSyncsSuccessfully(t *testing.T) {
	f, err := fileutil.CreateTemp(t.TempDir(), "wal")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	bw := bufio.NewWriter(f)
	if _, err := bw.WriteString("persisted entry\n"); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	if err := CloseAfterFlush(bw, f); err != nil {
		t.Fatalf("CloseAfterFlush failed: %v", err)
	}
}

func TestOpenJSONLineWAL_CreatesDurableDirectoryEntry(t *testing.T) {
	root := t.TempDir()
	const filename = "events.jsonl"
	var synced []string
	w, err := openJSONLineWALWithNextSeqAndSyncDir[map[string]any](
		root,
		filename,
		".checkpoint",
		defaultScannerMaxSize,
		func(string, string) (int64, error) { return 1, nil },
		func(path string) error {
			synced = append(synced, path)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("OpenJSONLineWAL: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	walPath := filepath.Join(root, paths.ProjectDataDir, paths.WalDir, filename)
	if !fileutil.IsRegularFile(walPath) {
		t.Fatalf("WAL file was not created at %s", walPath)
	}
	dataDir := filepath.Join(root, paths.ProjectDataDir)
	walDir := filepath.Join(dataDir, paths.WalDir)
	wantSynced := []string{dataDir, root, walDir}
	if !reflect.DeepEqual(synced, wantSynced) {
		t.Fatalf("synced directories = %v, want %v", synced, wantSynced)
	}
}

func TestOpenJSONLineWAL_PropagatesDirectorySyncErrors(t *testing.T) {
	syncErr := errors.New("sync directory")
	for failAt, name := range []string{"wal-directory-entry", "data-directory-entry", "wal-file-entry"} {
		t.Run(name, func(t *testing.T) {
			syncCall := 0
			_, err := openJSONLineWALWithNextSeqAndSyncDir[map[string]any](
				t.TempDir(),
				"events.jsonl",
				".checkpoint",
				defaultScannerMaxSize,
				func(string, string) (int64, error) { return 1, nil },
				func(string) error {
					current := syncCall
					syncCall++
					if current == failAt {
						return syncErr
					}
					return nil
				},
			)
			if err == nil || !errors.Is(err, syncErr) {
				t.Fatalf("OpenJSONLineWAL error = %v, want %v", err, syncErr)
			}
		})
	}
}
