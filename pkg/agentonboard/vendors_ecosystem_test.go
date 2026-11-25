package agentonboard

import (
	"path/filepath"
	"strings"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestKnownVendors_IncludesRooCodeAndZed(t *testing.T) {
	byID := make(map[VendorID]Vendor)
	for _, v := range KnownVendors() {
		byID[v.ID] = v
	}

	roo, ok := byID[VendorRooCode]
	if !ok {
		t.Fatalf("expected VendorRooCode in KnownVendors")
	}
	if !strings.Contains(BootPayloadFor(roo), "## Vendor: Roo Code") || !strings.Contains(BootPayloadFor(roo), ".roorules") {
		t.Errorf("unexpected Roo Code boot payload:\n%s", BootPayloadFor(roo))
	}

	zed, ok := byID[VendorZed]
	if !ok {
		t.Fatalf("expected VendorZed in KnownVendors")
	}
	if !strings.Contains(BootPayloadFor(zed), "## Vendor: Zed") || !strings.Contains(BootPayloadFor(zed), ".rules") {
		t.Errorf("unexpected Zed boot payload:\n%s", BootPayloadFor(zed))
	}

	cont, ok := byID[VendorContinue]
	if !ok {
		t.Fatalf("expected VendorContinue in KnownVendors")
	}
	if !strings.Contains(BootPayloadFor(cont), "## Vendor: Continue") || !strings.Contains(BootPayloadFor(cont), ".continuerules") {
		t.Errorf("unexpected Continue boot payload:\n%s", BootPayloadFor(cont))
	}

	aider, ok := byID[VendorAider]
	if !ok {
		t.Fatalf("expected VendorAider in KnownVendors")
	}
	if !strings.Contains(BootPayloadFor(aider), "## Vendor: Aider") || !strings.Contains(BootPayloadFor(aider), "CONVENTIONS.md") {
		t.Errorf("unexpected Aider boot payload:\n%s", BootPayloadFor(aider))
	}
}

func TestDetectVendors_RooCodeAndZedMarkers(t *testing.T) {
	dir := t.TempDir()
	if err := fileutil.WriteStandardFile(filepath.Join(dir, ".roorules"), []byte("# roo\n")); err != nil {
		t.Fatalf("write .roorules: %v", err)
	}
	if err := fileutil.EnsureDir(filepath.Join(dir, ".zed")); err != nil {
		t.Fatalf("mkdir .zed: %v", err)
	}
	if err := fileutil.EnsureDir(filepath.Join(dir, ".continue")); err != nil {
		t.Fatalf("mkdir .continue: %v", err)
	}
	if err := fileutil.WriteStandardFile(filepath.Join(dir, ".aider.conf.yml"), []byte("read: CONVENTIONS.md\n")); err != nil {
		t.Fatalf("write .aider.conf.yml: %v", err)
	}

	detected := DetectVendors(dir)
	found := make(map[VendorID]bool, len(detected))
	for _, v := range detected {
		found[v.ID] = true
	}
	if !found[VendorRooCode] {
		t.Errorf("expected VendorRooCode to be detected from .roorules")
	}
	if !found[VendorZed] {
		t.Errorf("expected VendorZed to be detected from .zed/")
	}
	if !found[VendorContinue] {
		t.Errorf("expected VendorContinue to be detected from .continue/")
	}
	if !found[VendorAider] {
		t.Errorf("expected VendorAider to be detected from .aider.conf.yml")
	}
}
