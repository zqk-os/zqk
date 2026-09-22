// BLI-STARTER-COMMUNITY-026 / PRI-STARTER-COMMUNITY-026 coverage elevation
package entitlements

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestCommunityChecker_MeshAndUnknownBundle(t *testing.T) {
	t.Parallel()
	c := &CommunityChecker{}
	ctx := context.Background()

	if err := c.CheckMeshEntitlement(ctx); err == nil || !strings.Contains(err.Error(), "Enterprise") {
		t.Fatalf("mesh should refuse: %v", err)
	}
	if err := c.CheckBundle(ctx, "mesh"); err == nil {
		t.Fatal("bundle mesh should refuse")
	}
	if err := c.CheckBundle(ctx, "unknown-pack"); err == nil || !strings.Contains(err.Error(), "unknown-pack") {
		t.Fatalf("unknown bundle should name itself: %v", err)
	}
}

func TestEnterpriseChecker_MeshAndEvolve(t *testing.T) {
	t.Parallel()
	e := &EnterpriseChecker{Features: []string{"mesh"}}
	ctx := context.Background()
	if err := e.CheckMeshEntitlement(ctx); err != nil {
		t.Fatalf("enterprise mesh: %v", err)
	}
	if err := e.CheckEvolveLimit(ctx, 99); err != nil {
		t.Fatalf("enterprise evolve: %v", err)
	}
	if err := e.CheckBundle(ctx, "anything"); err != nil {
		t.Fatalf("enterprise bundle: %v", err)
	}
}

func TestRegisterChecker_NilKeepsPrior(t *testing.T) {
	prior := CurrentCheckerForTest()
	defer RegisterChecker(prior)

	RegisterChecker(&CommunityChecker{})
	RegisterChecker(nil)
	got := CurrentCheckerForTest()
	if _, ok := got.(*CommunityChecker); !ok {
		t.Fatalf("nil register must keep community checker, got %T", got)
	}
}

func TestCheckWrappers_DelegateToGlobal(t *testing.T) {
	prior := CurrentCheckerForTest()
	defer RegisterChecker(prior)

	RegisterChecker(&CommunityChecker{})
	ctx := context.Background()
	if err := CheckMeshEntitlement(ctx); err == nil {
		t.Fatal("community mesh wrapper")
	}
	if err := CheckEntitlementBundle(ctx, BundleElevatedObject); err == nil {
		t.Fatal("community elevated wrapper")
	}

	RegisterChecker(&EnterpriseChecker{})
	if err := CheckMeshEntitlement(ctx); err != nil {
		t.Fatalf("enterprise mesh wrapper: %v", err)
	}
	if err := CheckEntitlementBundle(ctx, BundleElevatedObject); err != nil {
		t.Fatalf("enterprise elevated wrapper: %v", err)
	}
}

func TestLoadLicense_InvalidJWTFileKeepsCommunity(t *testing.T) {
	prior := CurrentCheckerForTest()
	t.Cleanup(func() {
		RegisterChecker(prior)
		_ = os.Unsetenv(brand.DefaultEnvPrefix + "_LICENSE_KEY")
	})
	_ = os.Unsetenv(brand.DefaultEnvPrefix + "_LICENSE_KEY")

	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, paths.ProjectStateDir)
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(dir, "license.jwt"), []byte("not-a-jwt"), paths.FilePerm600); err != nil {
		t.Fatal(err)
	}

	RegisterChecker(&CommunityChecker{})
	LoadLicense()
	if _, ok := CurrentCheckerForTest().(*CommunityChecker); !ok {
		t.Fatalf("invalid jwt must stay community, got %T", CurrentCheckerForTest())
	}
}
