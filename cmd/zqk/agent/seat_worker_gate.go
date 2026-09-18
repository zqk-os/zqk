package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/swarm"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	// seatWorkerGateTimeout bounds each toolchain command the gate runs.
	seatWorkerGateTimeout = 5 * time.Minute
	// seatWorkerGateFeedbackLimit truncates compiler output so a wall of errors
	// cannot crowd the model's context window out of repair range.
	seatWorkerGateFeedbackLimit = 4000
	// seatWorkerMaxRepairs is how many extra turns the model gets to fix work
	// the gate rejected.
	seatWorkerMaxRepairs = 2
)

// TRACK: BLI-1786951788129303000-f388e71d — remove when: CompletionVerifier
// evidence is project/ATK-policy pluggable (not hardcoded go vet/test +
// ZQK_TEST_ROOT) so non-Go / brand-portable repos reuse the seat-worker gate.

// goFilesWritten returns the repo-relative Go files a run wrote, in sorted
// order. Only write_code / write_file carry a path worth verifying, and paths
// that escape the project root are dropped rather than trusted.
func goFilesWritten(history []swarm.ToolCallRecord, root string) []string {
	seen := map[string]bool{}

	for _, record := range history {
		if !swarm.IsMutationEvidenceTool(record.Name) {
			continue
		}
		var args struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal([]byte(record.Arguments), &args); err != nil {
			continue
		}
		path := strings.TrimSpace(args.Path)
		if path == "" || !strings.HasSuffix(path, ".go") {
			continue
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		seen[filepath.ToSlash(rel)] = true
	}

	files := make([]string, 0, len(seen))
	for file := range seen {
		files = append(files, file)
	}
	sort.Strings(files)
	return files
}

// goPackagesWritten maps written Go files to the package patterns to verify.
func goPackagesWritten(history []swarm.ToolCallRecord, root string) []string {
	seen := map[string]bool{}
	for _, file := range goFilesWritten(history, root) {
		seen["./"+path.Dir(file)] = true
	}

	pkgs := make([]string, 0, len(seen))
	for pkg := range seen {
		pkgs = append(pkgs, pkg)
	}
	sort.Strings(pkgs)
	return pkgs
}

// revertSeatWrites undoes the Go files a rejected run wrote so a seat cannot
// leave the tree broken for the next agent or human. Tracked files go back to
// HEAD; files the run created are removed.
func revertSeatWrites(ctx context.Context, root string, files []string) error {
	var failures []string
	for _, file := range files {
		// Kernel CAS instance data is never reverted through git.
		if strings.HasPrefix(file, paths.ProcessDir+"/") || strings.HasPrefix(file, ".zqk/") {
			continue
		}
		tracked := execwrap.CommandContext(ctx, "git", "ls-files", "--error-unmatch", file)
		tracked.Dir = root
		if err := tracked.Run(); err == nil {
			restore := execwrap.CommandContext(ctx, "git", "checkout", "--", file)
			restore.Dir = root
			if out, err := restore.CombinedOutput(); err != nil {
				failures = append(failures, fmt.Sprintf("%s: %s", file, strings.TrimSpace(string(out))))
			}
			continue
		}
		if err := fileutil.Remove(filepath.Join(root, file)); err != nil && !fileutil.IsNotExist(err) {
			failures = append(failures, fmt.Sprintf("%s: %v", file, err))
		}
	}
	if len(failures) > 0 {
		return errfmt.Errorf("revert seat writes: %s", strings.Join(failures, "; "))
	}
	return nil
}

// runGoTool executes one go subcommand against the repo, returning combined
// output when it fails.
func runGoTool(ctx context.Context, root string, args ...string) (string, bool) {
	ctx, cancel := context.WithTimeout(ctx, seatWorkerGateTimeout)
	defer cancel()

	cmd := execwrap.CommandContext(ctx, "go", args...)
	cmd.Dir = root
	// The repo panics on `go test` launched outside make/CI unless the test root
	// is declared; the gate is a sanctioned runner, not an agent shortcut.
	cmd.Env = append(os.Environ(), zqkenv.TestRoot().Name()+"="+root)

	out, err := cmd.CombinedOutput()
	if err == nil {
		return "", true
	}
	return truncateFeedback(string(out)), false
}

// goModulePath returns the go.mod module path so compile-rejection feedback
// can name the real import root. Empty when go.mod is missing or malformed.
func goModulePath(root string) string {
	b, err := fileutil.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "the path declared in go.mod"
	}
	first, _, _ := strings.Cut(string(b), "\n")
	first = strings.TrimSpace(first)
	const prefix = "module "
	if !strings.HasPrefix(first, prefix) {
		return "the path declared in go.mod"
	}
	mod := strings.TrimSpace(strings.TrimPrefix(first, prefix))
	if mod == "" {
		return "the path declared in go.mod"
	}
	return mod
}

func truncateFeedback(out string) string {
	out = strings.TrimSpace(out)
	if len(out) <= seatWorkerGateFeedbackLimit {
		return out
	}
	return out[:seatWorkerGateFeedbackLimit] + "\n… (output truncated)"
}

func historyHasMutationWrite(history []swarm.ToolCallRecord) bool {
	for _, record := range history {
		if swarm.IsMutationEvidenceTool(record.Name) {
			return true
		}
	}
	return false
}

// verifyGoWorkAsCompletion is the seat-worker completion gate: a coding ATK
// cannot complete on narrative. A successful write tool call is the minimum
// evidence; Go writes must also compile and pass package tests.
// TRACK: BLI-1786948736717976000-a0522aac — compile+test still replaces
// tool-name evidence; this only closes the empty-history false-complete.
func verifyGoWorkAsCompletion(ctx context.Context, root string, history []swarm.ToolCallRecord) (string, error) {
	if !historyHasMutationWrite(history) {
		return "No mutation tool evidence. Successfully invoke write_code or write_file before completing. Narrative is not execution evidence.", nil
	}
	pkgs := goPackagesWritten(history, root)
	if len(pkgs) == 0 {
		// Script / spec / runbook ATKs write non-Go files; the mutation
		// tool already proved bytes landed. Do not invent a Go toolchain.
		return "", nil
	}

	// vet type-checks tests as well as sources, so it catches invented APIs
	// before the slower test run.
	if out, ok := runGoTool(ctx, root, append([]string{"vet"}, pkgs...)...); !ok {
		return fmt.Sprintf(
			"Your changes do not compile. `go vet %s` reported:\n\n%s\n\n"+
				"This module is %s. Do not invent other module paths. "+
				"Fix the code you wrote using the write tools. Do not claim completion until it compiles. "+
				"If you referenced an API you are unsure about, read the real source first instead of guessing.",
			strings.Join(pkgs, " "), out, goModulePath(root),
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
