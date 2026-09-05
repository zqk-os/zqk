package walutil

import (
	"bufio"
	"errors"
	"strings"
	"testing"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
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
