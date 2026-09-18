package idebridge

import (
	"strings"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestFormatProofOfLifeMessage(t *testing.T) {
	got := FormatProofOfLifeMessage("")
	if !strings.HasPrefix(got, ProofOfLifePrefix+":") {
		t.Fatalf("empty detail: %q", got)
	}
	got = FormatProofOfLifeMessage("ATK done")
	if got != ProofOfLifePrefix+": ATK done" {
		t.Fatalf("detail: %q", got)
	}
	got = FormatProofOfLifeMessage(ProofOfLifePrefix + ": already")
	if got != ProofOfLifePrefix+": already" {
		t.Fatalf("idempotent: %q", got)
	}
}

func TestQueueProofOfLife(t *testing.T) {
	root := t.TempDir()
	if !QueueProofOfLife(root, "hello human") {
		t.Fatal("expected queue ok")
	}
	data, err := fileutil.ReadFile(ControlJSONLPath(root))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, CmdWakeAttn) || !strings.Contains(s, ProofOfLifePrefix) || !strings.Contains(s, "hello human") {
		t.Fatalf("unexpected jsonl: %s", s)
	}
}
