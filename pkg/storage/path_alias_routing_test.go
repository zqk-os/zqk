// Exercises BuildPathAliasCacheForProject + paths.ResolvePathStrict / GetStreamSegmentDir so
// brand-settings aliases and stream segment wiring stay consistent when remapped.
package storage

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestBuildPathAliasCacheForProject_CustomAliasesRemap(t *testing.T) {
	root := t.TempDir()
	zqkDir := filepath.Join(root, paths.ProjectDataDir)
	if err := fileutil.EnsureDir(zqkDir); err != nil {
		t.Fatal(err)
	}
	settingsYAML := `version: "1.0.0"
paths:
  aliases:
    process: "alpha/process"
    streams: ".zqk/streams_alt"
`
	configDir := filepath.Join(zqkDir, paths.ConfigDir)
	if err := fileutil.EnsureDir(configDir); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(configDir, paths.BrandSettingsFilename)
	if err := fileutil.WriteSecureFile(settingsPath, []byte(settingsYAML)); err != nil {
		t.Fatal(err)
	}

	BuildPathAliasCacheForProject(root)

	gotProcess, err := paths.ResolvePathStrict(root, "prefix:process")
	if err != nil {
		t.Fatalf("ResolvePathStrict process: %v", err)
	}
	wantProcess := filepath.Join(root, "alpha", "process")
	if gotProcess != wantProcess {
		t.Errorf("process dir = %q want %q", gotProcess, wantProcess)
	}

	seg, err := GetStreamSegmentDir(root, objects.KindAuditEvent)
	if err != nil {
		t.Fatalf("GetStreamSegmentDir: %v", err)
	}
	wantSeg := filepath.Join(root, paths.ProjectDataDir, "streams_alt", objects.KindAuditEvent)
	if seg != wantSeg {
		t.Errorf("stream segment = %q want %q", seg, wantSeg)
	}

	// Second swap: edit settings and rebuild — routing must follow new aliases.
	settingsYAML2 := `version: "1.0.0"
paths:
  aliases:
    process: "beta/process"
    streams: "vendor/streams"
`
	if err := fileutil.WriteSecureFile(settingsPath, []byte(settingsYAML2)); err != nil {
		t.Fatal(err)
	}
	BuildPathAliasCacheForProject(root)

	gotProcess2, err := paths.ResolvePathStrict(root, "prefix:process")
	if err != nil {
		t.Fatalf("after remap: %v", err)
	}
	wantProcess2 := filepath.Join(root, "beta", "process")
	if gotProcess2 != wantProcess2 {
		t.Errorf("after remap process = %q want %q", gotProcess2, wantProcess2)
	}

	seg2, err := GetStreamSegmentDir(root, objects.KindAuditEvent)
	if err != nil {
		t.Fatalf("GetStreamSegmentDir after remap: %v", err)
	}
	wantSeg2 := filepath.Join(root, "vendor", "streams", objects.KindAuditEvent)
	if seg2 != wantSeg2 {
		t.Errorf("after remap segment = %q want %q", seg2, wantSeg2)
	}
}

// TestBuildPathAliasCacheForProject_PartialBrandSettingsMergesDefaults ensures zqk-settings paths.aliases
// overlays defaults instead of replacing them, so datacell_* and other defaults remain (ITEM-EXAMPLE).
func TestBuildPathAliasCacheForProject_PartialBrandSettingsMergesDefaults(t *testing.T) {
	root := t.TempDir()
	zqkDir := filepath.Join(root, paths.ProjectDataDir)
	if err := fileutil.EnsureDir(zqkDir); err != nil {
		t.Fatal(err)
	}
	settingsYAML := `version: "1.0.0"
paths:
  aliases:
    process: "alpha/process"
    streams: ".zqk/streams_alt"
`
	if err := fileutil.WriteSecureFile(filepath.Join(root, paths.BrandSettingsFilename), []byte(settingsYAML)); err != nil {
		t.Fatal(err)
	}
	BuildPathAliasCacheForProject(root)

	gotDocs, err := paths.ResolvePathStrict(root, "prefix:docs")
	if err != nil {
		t.Fatalf("ResolvePathStrict docs (should still exist from defaults): %v", err)
	}
	wantDocs := filepath.Join(root, paths.DocsDir)
	if gotDocs != wantDocs {
		t.Errorf("docs = %q want %q", gotDocs, wantDocs)
	}
	wantFF := filepath.Join(root, paths.ProjectDataDir, paths.ConfigDir, paths.FeatureFlagsFile)
	if got := datacell.FeatureFlagsPath(root); got != wantFF {
		t.Errorf("datacell FeatureFlagsPath = %q want %q", got, wantFF)
	}
}
