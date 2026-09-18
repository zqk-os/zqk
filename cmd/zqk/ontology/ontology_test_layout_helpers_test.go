package ontology

import (
	"bytes"
	"os"
	"testing"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func captureStdoutOntology(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = old })
	var buf bytes.Buffer
	done := make(chan struct{})
	goroutinelabels.NewGoroutine("ontology", "read stdout").StartSimple(func() {
		_, _ = buf.ReadFrom(r)
		close(done)
	})
	fn()
	_ = w.Close()
	<-done
	return buf.String()
}
