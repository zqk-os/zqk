package authcred

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// ResolveCredentialPath picks the credentials file for AuthMiddleware.
// Isolated ZQK_TEST_ROOT (not the live projectRoot) never falls back to $HOME
// so tests cannot inherit the developer login.
// When TEST_ROOT equals the live projectRoot and that tree has no credentials
// file, fall through to project-local then $HOME — leftover TEST_ROOT=repo
// in IDE/agent shells must not look like a missing token.
// TRACK: REDACTED — remove when: agent/MCP shells
// never export TEST_ROOT against the studio checkout.
func ResolveCredentialPath(projectRoot string) string {
	testRoot := strings.TrimSpace(os.Getenv(zqkenv.TestRoot()))
	projectRoot = strings.TrimSpace(projectRoot)

	if testRoot != "" {
		cand := filepath.Join(testRoot, paths.ProjectDataDir, "credentials")
		if fileutil.IsRegularFile(cand) {
			return cand
		}
		if !sameTree(testRoot, projectRoot) {
			return cand
		}
	}
	if projectRoot != "" {
		local := filepath.Join(projectRoot, paths.ProjectDataDir, "credentials")
		if fileutil.IsRegularFile(local) {
			return local
		}
	}
	home, err := fileutil.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ""
	}
	return filepath.Join(home, paths.ProjectDataDir, "credentials")
}

func sameTree(a, b string) bool {
	ca, cb := canonDir(a), canonDir(b)
	return ca != "" && cb != "" && ca == cb
}

func canonDir(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	if ev, err := filepath.EvalSymlinks(abs); err == nil {
		return ev
	}
	return abs
}
