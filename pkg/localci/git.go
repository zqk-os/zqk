package localci

import (
	"bytes"
	"context"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/execwrap"
)

func runGit(ctx context.Context, dir string, args ...string) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := execwrap.CommandContext(ctx, "git", args...)
	if strings.TrimSpace(dir) != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return out, errfmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return out, nil
}

func gitTrim(ctx context.Context, git GitRun, dir string, args ...string) (string, error) {
	out, err := git(ctx, dir, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func hasTrackedDirty(porcelain string) bool {
	for _, line := range strings.Split(porcelain, "\n") {
		line = strings.TrimRight(line, "\r")
		if len(line) < 2 {
			continue
		}
		code := line[:2]
		if code == "??" || code == "!!" {
			continue
		}
		return true
	}
	return false
}

func worktreeListContains(porcelain, absPath string) bool {
	want := "worktree " + absPath
	for _, line := range strings.Split(porcelain, "\n") {
		if strings.TrimSpace(line) == want {
			return true
		}
	}
	return false
}

func firstLine(b []byte) string {
	s := string(bytes.TrimSpace(b))
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}
