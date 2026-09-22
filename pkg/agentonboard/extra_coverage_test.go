// BLI-STARTER-COMMUNITY-029 / PRI-STARTER-COMMUNITY-029 coverage elevation
package agentonboard

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestRun_EmptyRootAndMissingSeat(t *testing.T) {
	t.Parallel()
	if _, err := Run(Options{}); err == nil {
		t.Fatal("empty root")
	}
	root := t.TempDir()
	res, err := Run(Options{ProjectRoot: root, SessionOK: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != ResultBlocked || res.Stages[StageSeat].Status != StageFailed {
		t.Fatalf("missing seat: %+v", res)
	}
}

func TestRun_SkipSeatAndPrime(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	res, err := Run(Options{ProjectRoot: root, SessionOK: true, SkipSeat: true, SkipPrime: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != ResultSuccess {
		t.Fatalf("status=%s stages=%+v", res.Status, res.Stages)
	}
}

func TestRun_SeatError(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := fileutil.MkdirAll(filepath.Join(root, ".claude"), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	res, err := Run(Options{
		ProjectRoot: root,
		SessionOK:   true,
		Seat: func(string, logging.Logger) (int, error) {
			return 0, errors.New("seat boom")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != ResultBlocked {
		t.Fatalf("status=%s", res.Status)
	}
}

func TestVendorHelpers(t *testing.T) {
	t.Parallel()
	if InferVector([]DetectedVendor{{ID: VendorAgentsMD}}) != "B" {
		t.Fatal("agents_md only is vector B")
	}
	filt := NormalizeVendorFilter(" IDE , cline, ")
	if _, ok := filt[VendorIDE]; !ok {
		t.Fatalf("filter %+v", filt)
	}
	if NormalizeVendorFilter("  ") != nil {
		t.Fatal("blank filter")
	}
	all := KnownVendors()
	got := FilterVendors(all, map[VendorID]struct{}{VendorCline: {}})
	if len(got) != 1 || got[0].ID != VendorCline {
		t.Fatalf("filter vendors %+v", got)
	}
	if n := FilterVendors(all, nil); len(n) != len(all) {
		t.Fatal("nil allow")
	}
	if len(VendorsToPrime(nil, true)) != len(all) {
		t.Fatal("all vendors")
	}
	if ids := vendorIDs(all); len(ids) != len(all) {
		t.Fatal("ids")
	}
	if fp := WorkspaceFingerprint("/tmp/x", "A", nil); fp == "" {
		t.Fatal("fingerprint")
	}
}

func TestBootPayloadForEveryVendor(t *testing.T) {
	t.Parallel()
	for _, v := range KnownVendors() {
		if s := BootPayloadFor(v); s == "" {
			t.Fatalf("empty payload %s", v.ID)
		}
	}
	unknown := Vendor{ID: "other", DisplayName: "Other"}
	if s := BootPayloadFor(unknown); s == "" {
		t.Fatal("default addendum")
	}
}

func TestWriteVendorPacksDryRun(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	written, err := WriteVendorPacks(root, KnownVendors()[:2], true, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 2 {
		t.Fatalf("written=%v", written)
	}
	abs := filepath.Join(root, filepath.FromSlash(written[0]))
	if pathExists(abs) {
		t.Fatal("dry-run must not write")
	}
}

func TestHeadlessVendorsOnly(t *testing.T) {
	t.Parallel()
	got := headlessVendorsOnly(KnownVendors())
	if len(got) != 1 || got[0].ID != VendorAgentsMD {
		t.Fatalf("%+v", got)
	}
	empty := headlessVendorsOnly(nil)
	if len(empty) != 1 || empty[0].ID != VendorAgentsMD {
		t.Fatalf("empty %+v", empty)
	}
}

func TestEnsureAgentsMD(t *testing.T) {
	t.Parallel()
	cline := FilterVendors(KnownVendors(), map[VendorID]struct{}{VendorCline: {}})
	got := ensureAgentsMD(cline)
	found := false
	for _, v := range got {
		if v.ID == VendorAgentsMD {
			found = true
		}
	}
	if !found {
		t.Fatal("must prepend agents_md")
	}
}

func TestSmokeAndNextSteps(t *testing.T) {
	t.Parallel()
	ok, detail := smoke(t.TempDir(), true, false, []string{"x"}, nil)
	if !ok || detail["dry_run"] != true {
		t.Fatalf("%v %v", ok, detail)
	}
	ok, _ = smoke(t.TempDir(), false, true, nil, nil)
	if !ok {
		t.Fatal("skip prime")
	}
	_ = nextSteps(&Result{Status: ResultFailed, Vector: "B"})
	_ = nextSteps(&Result{Status: ResultSuccess, Vector: "A"})
}
