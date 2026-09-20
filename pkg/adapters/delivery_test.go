package adapters

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/adapters/antigravity"
	"github.com/zqk-os/zqk/pkg/adapters/macos"
)

func TestResolve_AntigravityBrainTranscript(t *testing.T) {
	path := filepath.Join("brain", "conv", ".system_generated", "logs", "transcript.jsonl")
	got := Resolve(path)
	if got.Vendor() != antigravity.VendorID {
		t.Fatalf("Vendor=%q want %q", got.Vendor(), antigravity.VendorID)
	}
	if _, ok := got.(*antigravity.InboxAdapter); !ok {
		t.Fatalf("type %T", got)
	}
}

func TestResolve_DefaultsToMacOSClipboardPaste(t *testing.T) {
	got := Resolve("")
	if got.Vendor() != macos.VendorID {
		t.Fatalf("Vendor=%q want %q", got.Vendor(), macos.VendorID)
	}
	if _, ok := got.(*macos.ClipboardPasteAdapter); !ok {
		t.Fatalf("type %T", got)
	}
}

func TestResolve_DoesNotSniffSubstringFalsePositive(t *testing.T) {
	path := filepath.Join("not.system_generated", "logs", "transcript.jsonl")
	got := Resolve(path)
	if got.Vendor() != macos.VendorID {
		t.Fatalf("false-positive vendor sniff: Vendor=%q path=%q", got.Vendor(), path)
	}
}
