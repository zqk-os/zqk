package golang

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/adapters"
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/swarm"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

var _ adapters.CompletionGate = CompletionGate{}

const (
	gateTimeout        = 5 * time.Minute
	gateFeedbackLimit  = 4000
	goSourceSuffix     = ".go"
	missingModuleLabel = "the path declared in go.mod"
)

// CompletionGate maps mutation writes onto go list patterns and fail-closes
// completion on `go vet` / `go test` for those packages.
type CompletionGate struct{}

// Vendor identifies this toolchain adapter.
func (CompletionGate) Vendor() string { return "golang" }

// WrittenFiles returns the repo-relative Go files a run wrote, in sorted
// order. Only write_code / write_file carry a path worth verifying, and
// paths that escape the project root are dropped rather than trusted.
func (g CompletionGate) WrittenFiles(history []swarm.ToolCallRecord, root string) []string {
	var files []string
	for _, file := range swarm.MutationWriteRelPaths(history, root) {
		if strings.HasSuffix(file, goSourceSuffix) {
			files = append(files, file)
		}
	}
	return files
}

// Packages maps written Go files to the package patterns to verify.
func (g CompletionGate) Packages(history []swarm.ToolCallRecord, root string) []string {
	seen := map[string]struct{}{}
	for _, file := range g.WrittenFiles(history, root) {
		seen["./"+path.Dir(file)] = struct{}{}
	}
	pkgs := make([]string, 0, len(seen))
	for pkg := range seen {
		pkgs = append(pkgs, pkg)
	}
	sort.Strings(pkgs)
	return pkgs
}

// Verify is the Go completion gate: mutation writes that are not Go files
// pass (scripts/specs). Go writes must compile and pass package tests.
func (g CompletionGate) Verify(ctx context.Context, root string, history []swarm.ToolCallRecord) (string, error) {
	pkgs := g.Packages(history, root)
	if len(pkgs) == 0 {
		return "", nil
	}

	if out, ok := runGoTool(ctx, root, append([]string{"vet"}, pkgs...)...); !ok {
		return fmt.Sprintf(
			"Your changes do not compile. `go vet %s` reported:\n\n%s\n\n"+
				"This module is %s. Do not invent other module paths. "+
				"Fix the code you wrote using the write tools. Do not claim completion until it compiles. "+
				"If you referenced an API you are unsure about, read the real source first instead of guessing.",
			strings.Join(pkgs, " "), out, modulePath(root),
		), nil
	}

	testArgs := append([]string{"test", "-count=1", "-timeout", "120s"}, pkgs...)
	if out, ok := runGoTool(ctx, root, testArgs...); !ok {
		return fmt.Sprintf(
			"Your changes compile but tests fail. `go test %s` reported:\n\n%s\n\n"+
				"Fix the code or the test so the package passes. Do not claim completion while it is red.",
			strings.Join(pkgs, " "), out,
		), nil
	}
	return "", nil
}

func runGoTool(ctx context.Context, root string, args ...string) (string, bool) {
	ctx, cancel := context.WithTimeout(ctx, gateTimeout)
	defer cancel()

	cmd := execwrap.CommandContext(ctx, "go", args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), zqkenv.TestRoot().Name()+"="+root)

	out, err := cmd.CombinedOutput()
	if err == nil {
		return "", true
	}
	return truncateFeedback(string(out)), false
}

func modulePath(root string) string {
	b, err := fileutil.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return missingModuleLabel
	}
	first, _, _ := strings.Cut(string(b), "\n")
	first = strings.TrimSpace(first)
	const prefix = "module "
	if !strings.HasPrefix(first, prefix) {
		return missingModuleLabel
	}
	mod := strings.TrimSpace(strings.TrimPrefix(first, prefix))
	if mod == "" {
		return missingModuleLabel
	}
	return mod
}

func truncateFeedback(out string) string {
	out = strings.TrimSpace(out)
	if len(out) <= gateFeedbackLimit {
		return out
	}
	return out[:gateFeedbackLimit] + "\n… (output truncated)"
}
