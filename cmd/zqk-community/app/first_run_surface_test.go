package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	commsystem "github.com/zqk-os/zqk/cmd/zqk-community/system"
	studiosystem "github.com/zqk-os/zqk/cmd/zqk/system"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestFirstRunRootOmitsStudioAdminSurface(t *testing.T) {
	cmd := NewRootCommand()
	got := commandNames(cmd)
	for _, banned := range []string{"healthchk", "learn", "inbox", "keystore", "join", "internal", "automation"} {
		if _, ok := got[banned]; ok {
			t.Fatalf("community root still ships %q", banned)
		}
	}
	for _, need := range []string{"object", "system", "test", "workflow", "scheduler", "mcp"} {
		if _, ok := got[need]; !ok {
			t.Fatalf("community root missing %q", need)
		}
	}
}

func TestCommunitySystemFilterDoesNotChangeStudioSystem(t *testing.T) {
	t.Parallel()
	community := commandNames(commsystem.NewSystemCmd())
	studio := commandNames(studiosystem.NewSystemCmd())
	for _, name := range []string{"spec-origination", "update-specs", "federate"} {
		if _, ok := community[name]; ok {
			t.Fatalf("community system still ships %q", name)
		}
		if _, ok := studio[name]; !ok {
			t.Fatalf("community filter removed %q from Studio system surface", name)
		}
	}
}

func TestStudioModuleIdentityRemainsPrivateSourceIdentity(t *testing.T) {
	t.Parallel()
	cwd, err := fileutil.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join(cwd, "../../.."))
	goMod, err := fileutil.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if first, _, _ := strings.Cut(string(goMod), "\n"); first != "module github.com/zqk-os/zqk" {
		t.Fatalf("Studio go.mod module line = %q", first)
	}
}

func TestCommunityReleaseTemplatesAreIsolatedUnderOverlay(t *testing.T) {
	t.Parallel()
	cwd, err := fileutil.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join(cwd, "../../.."))
	for _, rel := range []string{
		"scripts/open-core/sku-overlay/NOTICE",
		"scripts/open-core/sku-overlay/SECURITY.md",
		"scripts/open-core/sku-overlay/CI.yml",
		"scripts/open-core/check-public-release-payload.sh",
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("missing community release template %s: %v", rel, err)
		}
	}
}

func commandNames(cmd *cobra.Command) map[string]struct{} {
	got := map[string]struct{}{}
	for _, sub := range cmd.Commands() {
		got[sub.Name()] = struct{}{}
	}
	return got
}
