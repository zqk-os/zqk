package bootstrap

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestPolyglotGreenfieldInit verifies greenfield initialization across
// multiple simulated language ecosystems (Python, TypeScript, Rust, Docs).
func TestPolyglotGreenfieldInit(t *testing.T) {
	t.Parallel()
	if ManifestPaths() == nil {
		t.Skip("no embedded manifest (archive missing from this build)")
	}

	testCases := []struct {
		name       string
		markerFile string
		markerData string
	}{
		{
			name:       "python_project",
			markerFile: "pyproject.toml",
			markerData: "[project]\nname = \"sample-py\"\nversion = \"0.1.0\"\n",
		},
		{
			name:       "typescript_project",
			markerFile: "package.json",
			markerData: "{\n  \"name\": \"sample-ts\",\n  \"version\": \"1.0.0\"\n}\n",
		},
		{
			name:       "rust_project",
			markerFile: "Cargo.toml",
			markerData: "[package]\nname = \"sample-rs\"\nversion = \"0.1.0\"\n",
		},
		{
			name:       "docs_project",
			markerFile: "README.md",
			markerData: "# Sample Documentation Project\n",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			projectRoot := t.TempDir()

			// Seed ecosystem marker file
			markerPath := filepath.Join(projectRoot, tc.markerFile)
			if err := fileutil.WriteFile(markerPath, []byte(tc.markerData), paths.FilePerm644); err != nil {
				t.Fatalf("failed to write ecosystem marker file: %v", err)
			}

			logger := logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewJSONFormatter(pkgctx.NewSystemContext()))
			if err := ExtractTo(projectRoot, logger, true); err != nil {
				t.Fatalf("ExtractTo failed in %s: %v", tc.name, err)
			}

			// Verify .zqk internal files were created
			internal := filepath.Join(projectRoot, paths.ProcessInternalDir)
			specs := filepath.Join(internal, "objects")
			if st, err := fileutil.Stat(specs); err != nil || !st.IsDir() {
				specs = filepath.Join(internal, "object_specs")
				if st, err := fileutil.Stat(specs); err != nil || !st.IsDir() {
					t.Fatalf("expected objects or object_specs under %s: %v", internal, err)
				}
			}

			// Verify no instance leakage
			var leaked []string
			err := filepath.Walk(projectRoot, func(path string, info fileutil.FileInfo, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if info == nil || info.IsDir() {
					return nil
				}
				base := info.Name()
				if strings.HasPrefix(base, "BLI-") || strings.HasPrefix(base, "PRI-") || strings.Contains(base, "hacked") {
					leaked = append(leaked, path)
				}
				return nil
			})
			if err != nil {
				t.Fatalf("error inspecting extracted project: %v", err)
			}
			if len(leaked) > 0 {
				t.Fatalf("ecosystem %s leaked instance files: %v", tc.name, leaked)
			}

			// Verify ecosystem marker remains intact
			data, err := fileutil.ReadFile(markerPath)
			if err != nil {
				t.Fatalf("marker file disappeared: %v", err)
			}
			if string(data) != tc.markerData {
				t.Fatalf("marker file corrupted during bootstrap extract: got %q, want %q", string(data), tc.markerData)
			}
		})
	}
}

// TestBootstrapPortableScript verifies that scripts/open-core/verify-bootstrap-portable.sh passes cleanly.
func TestBootstrapPortableScript(t *testing.T) {
	t.Parallel()
	moduleRoot, err := findModuleRoot()
	if err != nil || moduleRoot == "" {
		t.Fatalf("failed finding module root: %v", err)
	}

	scriptPath := filepath.Join(moduleRoot, "scripts", "open-core", "verify-bootstrap-portable.sh")
	if _, err := fileutil.Stat(scriptPath); err != nil {
		t.Skipf("verify-bootstrap-portable.sh not found at %s", scriptPath)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/sh", scriptPath, moduleRoot)
	cmd.Dir = moduleRoot
	cmd.Env = append(os.Environ(), "ZQK_ALLOW_FOREGROUND_GO_TEST=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("verify-bootstrap-portable.sh failed: %v\nOutput:\n%s", err, string(out))
	}
}

// TestCommunityBinaryBuildCGOZero verifies that cmd/zqk-community builds cleanly with CGO_ENABLED=0.
func TestCommunityBinaryBuildCGOZero(t *testing.T) {
	t.Parallel()
	moduleRoot, err := findModuleRoot()
	if err != nil || moduleRoot == "" {
		t.Fatalf("failed finding module root: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	tmpBin := filepath.Join(t.TempDir(), "zqk-community-probe")
	cmd := exec.CommandContext(ctx, "go", "build", "-o", tmpBin, "./cmd/zqk-community")
	cmd.Dir = moduleRoot
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("CGO_ENABLED=0 build of cmd/zqk-community failed: %v\nOutput:\n%s", err, string(out))
	}

	if st, err := fileutil.Stat(tmpBin); err != nil || st.Size() == 0 {
		t.Fatalf("built binary is missing or empty: %v", err)
	}
}
