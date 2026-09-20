package agentonboard

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/logging"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestDetectVendors_IDE(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := fileutil.MkdirAll(filepath.Join(root, ".ide"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := DetectVendors(root)
	if InferVector(got) != "A" {
		t.Fatalf("vector=%q want A; detected=%v", InferVector(got), got)
	}
	found := false
	for _, d := range got {
		if d.ID == VendorIDE {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected ide detection, got %#v", got)
	}
}

func TestDetectVendors_BareAgentsDirNotDetected(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := fileutil.MkdirAll(filepath.Join(root, ".agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := DetectVendors(root)
	if len(got) != 0 {
		t.Fatalf("bare .agents must not detect, got %#v", got)
	}
}

func TestDetectVendors_EmptyIsVectorB(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	got := DetectVendors(root)
	if len(got) != 0 {
		t.Fatalf("expected no detections, got %#v", got)
	}
	if InferVector(got) != "B" {
		t.Fatalf("vector=%q want B", InferVector(got))
	}
}

func TestRun_DetectOnly(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := fileutil.MkdirAll(filepath.Join(root, ".ide"), 0o755); err != nil {
		t.Fatal(err)
	}
	res, err := Run(Options{ProjectRoot: root, DetectOnly: true, SessionOK: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != ResultSuccess || res.Vector != "A" {
		t.Fatalf("status=%s vector=%s", res.Status, res.Vector)
	}
	if res.Stages[StageSeat].Status != StageSkipped {
		t.Fatalf("seat stage=%+v", res.Stages[StageSeat])
	}
}

func TestRun_DryRunDoesNotSeatOrWrite(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := fileutil.MkdirAll(filepath.Join(root, ".ide"), 0o755); err != nil {
		t.Fatal(err)
	}
	seated := false
	res, err := Run(Options{
		ProjectRoot: root,
		DryRun:      true,
		SessionOK:   true,
		Seat: func(string, logging.Logger) (int, error) {
			seated = true
			return 0, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if seated {
		t.Fatal("dry-run must not call seat")
	}
	if res.Status != ResultSuccess {
		t.Fatalf("dry-run status=%s stages=%v", res.Status, res.Stages)
	}
	if pathExists(filepath.Join(root, ".agents", "AGENTS.md")) {
		t.Fatal("dry-run must not write AGENTS.md")
	}
	if pathExists(filepath.Join(root, filepath.FromSlash(SyncReportRelPath))) {
		t.Fatal("dry-run must not write sync report")
	}
}

func TestRun_PrimeAndSmoke(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	res, err := Run(Options{
		ProjectRoot: root,
		SessionOK:   true,
		SkipSeat:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != ResultSuccess {
		t.Fatalf("status=%s stages=%v", res.Status, res.Stages)
	}
	if !pathExists(filepath.Join(root, ".agents", "AGENTS.md")) {
		t.Fatal("expected AGENTS.md")
	}
	if !pathExists(filepath.Join(root, filepath.FromSlash(SyncReportRelPath))) {
		t.Fatal("expected sync report")
	}
	if res.Vector != "B" {
		t.Fatalf("empty workspace vector=%s want B", res.Vector)
	}
	if len(res.MarketProbeOpen) == 0 {
		t.Fatal("Vector B should include market probe checklist")
	}
	pack := filepath.Join(root, filepath.FromSlash(AgentPacksRelDir), string(VendorAgentsMD), "BOOT.md")
	if !pathExists(pack) {
		t.Fatal("expected agent pack BOOT.md")
	}
	report, err := ReadSyncReport(root)
	if err != nil || report == nil || report.Fingerprint == "" {
		t.Fatalf("expected sync fingerprint, report=%v err=%v", report, err)
	}
}

func TestRun_HeadlessSkipsIDERulesEvenIfIDEPresent(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := fileutil.MkdirAll(filepath.Join(root, ".ide"), 0o755); err != nil {
		t.Fatal(err)
	}
	res, err := Run(Options{
		ProjectRoot: root,
		SessionOK:   true,
		SkipSeat:    true,
		Headless:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Vector != "B" {
		t.Fatalf("headless vector=%s want B", res.Vector)
	}
	if pathExists(filepath.Join(root, ".iderules")) {
		t.Fatal("headless must not write .iderules")
	}
	if !pathExists(filepath.Join(root, ".agents", "AGENTS.md")) {
		t.Fatal("headless must still write AGENTS.md")
	}
	vendors, _ := res.Stages["prime_workspace"].Detail["vendors"].([]string)
	for _, id := range vendors {
		if id != string(VendorAgentsMD) {
			t.Fatalf("headless vendors=%v want only agents_md", vendors)
		}
	}
}

func TestMarketProbeChecklist_NonEmpty(t *testing.T) {
	t.Parallel()
	if len(MarketProbeChecklist()) < 3 {
		t.Fatal("expected probe questions")
	}
}

func TestWriteVendorConfigs_SkipsExistingUnlessForce(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, ".iderules")
	if err := fileutil.WriteFile(path, []byte("studio-dense rules"), 0o644); err != nil {
		t.Fatal(err)
	}
	vendors := []Vendor{{
		ID:             VendorIDE,
		ConfigRelPaths: []string{".iderules"},
	}}
	wr, err := WriteVendorConfigs(root, vendors, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(wr.Skipped) != 1 || len(wr.Written) != 0 {
		t.Fatalf("want skip existing, got written=%v skipped=%v", wr.Written, wr.Skipped)
	}
	body, _ := fileutil.ReadFile(path)
	if string(body) != "studio-dense rules" {
		t.Fatalf("must not clobber existing rules, got %q", body)
	}
	wr, err = WriteVendorConfigs(root, vendors, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(wr.Written) != 1 {
		t.Fatalf("force write failed: %+v", wr)
	}
	body, _ = fileutil.ReadFile(path)
	if string(body) == "studio-dense rules" {
		t.Fatal("force should overwrite")
	}
}

func TestBootPayloadFor_IncludesVendorAddendum(t *testing.T) {
	t.Parallel()
	body := BootPayloadFor(Vendor{ID: VendorIDE, DisplayName: "IDE"})
	if !strings.Contains(body, "Vendor: IDE") {
		t.Fatalf("missing ide addendum: %s", body)
	}
}

func TestWriteVendorPacks(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	vendors := []Vendor{{ID: VendorAgentsMD, DisplayName: "AGENTS"}}
	written, err := WriteVendorPacks(root, vendors, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 1 {
		t.Fatalf("written=%v", written)
	}
	if !pathExists(filepath.Join(root, filepath.FromSlash(written[0]))) {
		t.Fatal("pack file missing")
	}
}
