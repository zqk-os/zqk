package paths

import (
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// MCPServeArgs is the product CLI subcommand that starts stdio MCP.
// There is no separate brand-mcp sidecar in the community kernel.
var MCPServeArgs = []string{"mcp", "serve"}

// ResolveProductCLI locates the branded product executable.
//
// Precedence:
//  1. BIN env (operator override) when the path exists
//  2. STABLE_BINARY_PATH env when the path exists
//  3. cli.binary_path from brand settings when it exists
//  4. running executable when it is not a test/go-build binary
//  5. project-local bin/<executable> and stable images
//  6. PATH lookup of brand.ExecutableName()
//  7. brand.ExecutableName() as a PATH name (not a cwd-relative ./bin guess)
var productCLIs stampmemo.Table[string] // keyed by projectRoot + BIN/STABLE env (closed per process)

func ResolveProductCLI(projectRoot string) string {
	root := strings.TrimSpace(projectRoot)
	key := root + "\x00" + strings.TrimSpace(zqkenv.Bin().Get()) + "\x00" + strings.TrimSpace(zqkenv.StableBinaryPath().Get())
	path, _ := productCLIs.Load(key, stampmemo.OfAll(ProductCLICandidates(root)...), func() (string, error) {
		return resolveProductCLI(root), nil
	})
	return path
}

func resolveProductCLI(projectRoot string) string {
	if bin := strings.TrimSpace(zqkenv.Bin().Get()); bin != emptyValue && existingFile(bin) {
		return bin
	}
	if stable := strings.TrimSpace(zqkenv.StableBinaryPath().Get()); stable != emptyValue && existingFile(stable) {
		return stable
	}

	root := strings.TrimSpace(projectRoot)
	if root == emptyValue {
		root = ResolveProjectRoot(".")
	}
	if root != emptyValue {
		if configured := strings.TrimSpace(LoadBrandCLIBinaryPath(root)); configured != emptyValue {
			candidate := configured
			if !filepath.IsAbs(configured) {
				candidate = filepath.Join(root, configured)
			}
			if existingFile(candidate) {
				return candidate
			}
		}
	}

	if !zqkenv.IsInTest() {
		if self, err := fileutil.Executable(); err == nil && strings.TrimSpace(self) != emptyValue && !isUnsafeProductCLI(self) {
			return self
		}
	}

	if root != emptyValue {
		for _, candidate := range ProductCLICandidates(root) {
			if existingFile(candidate) {
				return candidate
			}
		}
	}

	exe := brand.ExecutableName()
	if p, err := exec.LookPath(exe); err == nil {
		return p
	}
	return exe
}

// ProductCLICandidates returns on-disk locations for the branded CLI under projectRoot.
// Does not include cwd-relative guesses or a separate *-mcp sidecar.
func ProductCLICandidates(projectRoot string) []string {
	if strings.TrimSpace(projectRoot) == emptyValue {
		return nil
	}
	exe := brand.ExecutableName()
	stable := StableBinaryName()
	seen := make(map[string]struct{}, 6)
	add := func(cands []string, p string) []string {
		if p == emptyValue {
			return cands
		}
		if _, ok := seen[p]; ok {
			return cands
		}
		seen[p] = struct{}{}
		return append(cands, p)
	}
	var out []string
	out = add(out, RepoBinPath(projectRoot))
	out = add(out, StableBinaryPath(projectRoot))
	out = add(out, RepoStableBinaryPath(projectRoot))
	out = add(out, filepath.Join(projectRoot, RepoBinDir, exe))
	out = add(out, filepath.Join(projectRoot, exe))
	if stable != exe {
		out = add(out, filepath.Join(projectRoot, RepoBinDir, stable))
	}
	return out
}

// MCPServeArgv returns argv for a stdio MCP server.
// override, when set, is a command line (binary plus optional args) from the caller.
func MCPServeArgv(override, projectRoot string) (string, []string) {
	if parts := strings.Fields(strings.TrimSpace(override)); len(parts) > 0 {
		return parts[0], parts[1:]
	}
	return ResolveProductCLI(projectRoot), append([]string(nil), MCPServeArgs...)
}

// MCPServeCommandLine joins MCPServeArgv for callers that still pass a single string
// into a Fields-splitting executor.
func MCPServeCommandLine(override, projectRoot string) string {
	name, args := MCPServeArgv(override, projectRoot)
	if len(args) == 0 {
		return name
	}
	return strings.Join(append([]string{name}, args...), " ")
}

func existingFile(path string) bool {
	if strings.TrimSpace(path) == emptyValue {
		return false
	}
	info, err := fileutil.Stat(path)
	return err == nil && !info.IsDir()
}

func isUnsafeProductCLI(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return strings.HasSuffix(base, ".test") ||
		base == "main" ||
		base == "main.exe" ||
		strings.Contains(path, "go-build") ||
		strings.Contains(path, "___go_build")
}
