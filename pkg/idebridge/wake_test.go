package idebridge

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/brand"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestQueueWakeAttn(t *testing.T) {
	root := t.TempDir()
	if !QueueWakeAttn(root, "ATTN peer: hello") {
		t.Fatal("expected queue ok")
	}
	data, err := fileutil.ReadFile(ControlJSONLPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), CmdWakeAttn) || !strings.Contains(string(data), "ATTN peer") {
		t.Fatalf("unexpected jsonl: %s", data)
	}
}

func TestQueueWakeAttn_disabled(t *testing.T) {
	key := brand.EnvVar("IDE_BRIDGE_WAKE")
	t.Setenv(key, "0")
	if QueueWakeAttn(t.TempDir(), "x") {
		t.Fatal("expected disabled")
	}
}

func TestTruncateRunes(t *testing.T) {
	if got := truncateRunes("abcd", 10); got != "abcd" {
		t.Fatalf("got %q", got)
	}
	if got := truncateRunes("abcdefghij", 4); got != "abcd…" {
		t.Fatalf("got %q", got)
	}
	_ = filepath.Separator
}
