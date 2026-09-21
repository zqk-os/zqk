package golang

import (
	"context"
	"path/filepath"
	"time"

	"github.com/zqk-os/zqk/pkg/adapters"
	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/execwrap"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

var _ adapters.WorktreeBuildCheck = WorktreeBuildCheck{}

const (
	worktreeBuildTimeout = 3 * time.Minute
	cmdDir               = "cmd"
	goTool               = "go"
	goBuildVerb          = "build"
	goOutputFlag         = "-o"
)

// WorktreeBuildCheck compiles the product CLI in a Go worktree before merge.
// Trees without go.mod are not this adapter's concern (return nil).
type WorktreeBuildCheck struct{}

// Vendor identifies this toolchain adapter.
func (WorktreeBuildCheck) Vendor() string { return "golang" }

// Verify fail-closes when this tree is a Go module that contains the product
// CLI package and `go build` of that package fails.
func (WorktreeBuildCheck) Verify(ctx context.Context, worktreeRoot string) error {
	if modulePath(worktreeRoot) == missingModuleLabel {
		return nil
	}
	pkg := productCLIPackage(worktreeRoot)
	if pkg == "" {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, worktreeBuildTimeout)
	defer cancel()

	outBin := filepath.Join(fileutil.TempDir(), "worktree-build-check")
	cmd := execwrap.CommandContext(timeoutCtx, goTool, goBuildVerb, goOutputFlag, outBin, pkg)
	cmd.Dir = worktreeRoot
	out, err := cmd.CombinedOutput()
	_ = fileutil.Remove(outBin)
	if err != nil {
		msg := truncateFeedback(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return errfmt.Errorf("worktree build check failed: %s", msg)
	}
	return nil
}

func productCLIPackage(root string) string {
	rel := filepath.Join(cmdDir, brand.CanonicalExecutableToken)
	if _, err := fileutil.Stat(filepath.Join(root, rel)); err != nil {
		return ""
	}
	return "./" + filepath.ToSlash(rel)
}
