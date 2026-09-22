// BLI-STARTER-COMMUNITY-033 / PRI-STARTER-COMMUNITY-033 coverage elevation
package featureflags

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestFeatureFlags_LoadBadJSONAndLookups(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{Kind: "pkg.featureflags.extra"})
	root := proj.Root
	ff := NewFeatureFlags(root)
	if err := ff.Load(); err != nil {
		t.Fatal(err)
	}
	if ff.IsEnabled("no-such-flag") {
		t.Fatal("unknown enabled")
	}
	if _, ok := ff.GetFlag("no-such-flag"); ok {
		t.Fatal("unknown get")
	}
	all := ff.GetAllFlags()
	if len(all) == 0 || all[FlagDeferredHashUpdates] == nil {
		t.Fatalf("%v", all)
	}
	if err := ff.SetEnabled("custom_flag", true); err != nil {
		t.Fatal(err)
	}
	if !ff.IsEnabled("custom_flag") {
		t.Fatal("custom")
	}
	got, ok := ff.GetFlag("custom_flag")
	if !ok || got == nil || !got.Enabled {
		t.Fatalf("%v %v", got, ok)
	}

	path := datacell.FeatureFlagsPath(root)
	if err := fileutil.MkdirAll(filepath.Dir(path), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(path, []byte("{"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	ff2 := NewFeatureFlags(root)
	if err := ff2.Load(); err != nil {
		t.Fatal(err)
	}
	if !ff2.IsEnabled(FlagDeferredHashUpdates) {
		t.Fatal("defaults after bad json")
	}
}
