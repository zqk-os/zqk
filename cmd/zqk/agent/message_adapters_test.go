package agent

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/adapters/antigravity"
	"github.com/zqk-os/zqk/pkg/adapters/macos"
)

func TestGetDeliveryAdapter_DelegatesVendorSelection(t *testing.T) {
	agy := filepath.Join("brain", "c1", ".system_generated", "logs", "transcript.jsonl")
	if GetDeliveryAdapter(agy).Vendor() != antigravity.VendorID {
		t.Fatal("expected antigravity vendor for brain transcript")
	}
	if GetDeliveryAdapter("").Vendor() != macos.VendorID {
		t.Fatal("expected macos vendor fallback")
	}
}
