package authcred

import (
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

var credTokens stampmemo.Table[string]

// ResolveCredentialPath picks the credentials file for AuthMiddleware.
// Isolated ZQK_TEST_ROOT (not the live projectRoot) never falls back to $HOME
// so tests cannot inherit the developer login.
// When TEST_ROOT equals the live projectRoot and that tree has no credentials
// file, fall through to project-local then $HOME — leftover TEST_ROOT=repo
// in IDE/agent shells must not look like a missing token.
// never export TEST_ROOT against the studio checkout.
func ResolveCredentialPath(projectRoot string) string {
	testRoot := strings.TrimSpace(zqkenv.TestRoot().Get())
	projectRoot = strings.TrimSpace(projectRoot)

	if testRoot != "" {
		cand := paths.CredentialsPath(testRoot)
		if fileutil.IsRegularFile(cand) {
			return cand
		}
		if !sameTree(testRoot, projectRoot) {
			return cand
		}
	}
	if projectRoot != "" {
		local := paths.CredentialsPath(projectRoot)
		if fileutil.IsRegularFile(local) {
			return local
		}
	}
	home, err := fileutil.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ""
	}
	return filepath.Join(home, paths.ProjectDataDir, paths.CredentialsFile)
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

// ReadCredentialToken returns the trimmed credentials-file payload, retained
// until that file's mtime changes. Missing files yield "".
func ReadCredentialToken(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	token, _ := credTokens.Load(path, stampmemo.Of(path), func() (string, error) {
		data, err := fileutil.ReadFile(path)
		if err != nil {
			return "", nil
		}
		return strings.TrimSpace(string(data)), nil
	})
	return token
}
