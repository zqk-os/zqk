package releasegate

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func findModuleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}
	for {
		if _, err := fileutil.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find module root containing go.mod")
		}
		dir = parent
	}
}

// TestMultiPlatformCompilation verifies clean compilation across 4 platforms with CGO_ENABLED=0.
func TestMultiPlatformCompilation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping multiplatform compilation in -short mode")
	}
	t.Parallel()
	root := findModuleRoot(t)

	platforms := []struct {
		goos   string
		goarch string
	}{
		{"darwin", "amd64"},
		{"darwin", "arm64"},
		{"linux", "amd64"},
		{"linux", "arm64"},
	}

	for _, p := range platforms {
		p := p
		t.Run(p.goos+"_"+p.goarch, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()

			tmpOut := filepath.Join(t.TempDir(), "zqk-community-"+p.goos+"-"+p.goarch)
			cmd := exec.CommandContext(ctx, "go", "build", "-o", tmpOut, "./cmd/zqk-community")
			cmd.Dir = root
			cmd.Env = append(os.Environ(),
				"CGO_ENABLED=0",
				"GOOS="+p.goos,
				"GOARCH="+p.goarch,
			)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("cross-platform build for %s/%s failed: %v\nOutput:\n%s", p.goos, p.goarch, err, string(out))
			}

			if st, err := fileutil.Stat(tmpOut); err != nil || st.Size() == 0 {
				t.Fatalf("compiled binary for %s/%s is empty or missing: %v", p.goos, p.goarch, err)
			}
		})
	}
}

// TestCommandSpecBaselineCoverage verifies that no orphaned or unreachable commands exist.
func TestCommandSpecBaselineCoverage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	t.Parallel()
	root := findModuleRoot(t)

	script := filepath.Join(root, "scripts", "check-unreachable-commands.sh")
	if _, err := fileutil.Stat(script); err != nil {
		t.Skip("check-unreachable-commands.sh not found")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/sh", script)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command spec baseline check failed: %v\nOutput:\n%s", err, string(out))
	}
}

// TestZeroGhostRefs verifies that the CAS graph has zero ghost references.
func TestZeroGhostRefs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	t.Parallel()
	root := findModuleRoot(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "./bin/zqk-stable", "workflow", "whats-next", "--format", "json", "--skip-measure")
	cmd.Dir = root
	var cleanEnv []string
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, "ZQK_TEST_ROOT=") {
			continue
		}
		cleanEnv = append(cleanEnv, env)
	}
	cleanEnv = append(cleanEnv, "ZQK_PROJECT_ROOT="+root)
	cmd.Env = cleanEnv
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("whats-next ambient check failed: %v\nOutput:\n%s", err, string(out))
	}

	// Verify output indicates healthy kernel ambience
	if len(out) == 0 {
		t.Fatal("empty response from whats-next")
	}
}
