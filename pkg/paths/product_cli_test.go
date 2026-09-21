package paths

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/brand"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestResolveProductCLI_ignoresCwdZqkMcpSidecar(t *testing.T) {
	origExe := brand.ExecutableName()
	t.Cleanup(func() { brand.SetExecutableName(origExe) })
	brand.SetExecutableName("acme-cli")

	cwd := t.TempDir()
	sidecar := filepath.Join(cwd, RepoBinDir, "zqk-mcp")
	legacy := filepath.Join(cwd, RepoBinDir, "zqk")
	if err := fileutil.EnsureDir(filepath.Dir(sidecar)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(sidecar, []byte{0}, DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(legacy, []byte{0}, DirPerm755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(cwd)

	emptyRoot := t.TempDir()
	t.Setenv(zqkenv.Bin().Name(), "")
	t.Setenv(zqkenv.StableBinaryPath().Name(), "")
	t.Setenv(zqkenv.ProjectRoot().Name(), emptyRoot)
	t.Setenv(zqkenv.TestRoot().Name(), emptyRoot)

	got := ResolveProductCLI(emptyRoot)
	if strings.Contains(got, "zqk-mcp") {
		t.Fatalf("resolved %q; cwd sidecar must not be assumed", got)
	}
	if got == sidecar || got == legacy || got == "./bin/zqk-mcp" || got == "./bin/zqk" {
		t.Fatalf("resolved cwd-relative studio path %q", got)
	}
	if got != "acme-cli" && filepath.Base(got) != "acme-cli" {
		t.Fatalf("got %q, want branded executable acme-cli", got)
	}

	bin, args := MCPServeArgv("", emptyRoot)
	if bin != got {
		t.Fatalf("MCPServeArgv bin %q, want %q", bin, got)
	}
	if !slices.Equal(args, MCPServeArgs) {
		t.Fatalf("MCPServeArgv args %v, want %v", args, MCPServeArgs)
	}
}

func TestResolveProductCLI_usesRepoBrandBinary(t *testing.T) {
	origExe := brand.ExecutableName()
	t.Cleanup(func() { brand.SetExecutableName(origExe) })
	brand.SetExecutableName("acme-cli")

	root := t.TempDir()
	want := filepath.Join(root, RepoBinDir, "acme-cli")
	sidecar := filepath.Join(root, RepoBinDir, "zqk-mcp")
	if err := fileutil.EnsureDir(filepath.Dir(want)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(want, []byte{0}, DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(sidecar, []byte{0}, DirPerm755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(zqkenv.Bin().Name(), "")
	t.Setenv(zqkenv.StableBinaryPath().Name(), "")

	got := ResolveProductCLI(root)
	if got != want {
		t.Fatalf("got %q, want repo brand binary %q", got, want)
	}
	if strings.Contains(got, "zqk-mcp") {
		t.Fatal("must not prefer zqk-mcp sidecar when the product CLI exists")
	}
}

func TestResolveProductCLI_binEnvWins(t *testing.T) {
	root := t.TempDir()
	override := filepath.Join(root, "custom-cli")
	if err := fileutil.WriteFile(override, []byte{0}, DirPerm755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(zqkenv.Bin().Name(), override)
	t.Setenv(zqkenv.StableBinaryPath().Name(), "")

	got := ResolveProductCLI(root)
	if got != override {
		t.Fatalf("got %q, want BIN override %q", got, override)
	}
}

func TestMCPServeArgv_overrideIsArgvNotConcatenatedPath(t *testing.T) {
	name, args := MCPServeArgv("/tmp/hang.sh", "")
	if name != "/tmp/hang.sh" || len(args) != 0 {
		t.Fatalf("got name=%q args=%v", name, args)
	}
	name, args = MCPServeArgv("/opt/acme-cli mcp serve", "")
	if name != "/opt/acme-cli" || !slices.Equal(args, MCPServeArgs) {
		t.Fatalf("got name=%q args=%v", name, args)
	}
	if got := MCPServeCommandLine("/opt/acme-cli mcp serve", ""); got != "/opt/acme-cli mcp serve" {
		t.Fatalf("command line %q", got)
	}
}
