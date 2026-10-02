package policy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/security/secretpatterns"
)

// SecretsGate scans files for API tokens, private keys, and credential leaks.
type SecretsGate struct{}

func (g *SecretsGate) Name() string {
	return "secrets"
}

func (g *SecretsGate) Description() string {
	return "Scans repository files for API tokens, AWS keys, private keys, and credential leaks"
}

var secretPatterns = secretpatterns.CombinedRegex

var excludedDirNames = map[string]bool{
	".git":               true,
	paths.ProjectDataDir: true,
	"vendor":             true,
	"testdata":           true,
	".agent":             true,
	".gemini":            true,
}

var binaryExts = map[string]bool{
	".png":   true,
	".jpg":   true,
	".jpeg":  true,
	".gif":   true,
	".ico":   true,
	".pdf":   true,
	".tar":   true,
	".gz":    true,
	".zip":   true,
	".bin":   true,
	".csnap": true,
	".dylib": true,
	".so":    true,
	".a":     true,
	".o":     true,
	".exe":   true,
}

func (g *SecretsGate) Run(ctx context.Context, opts RunOptions) (*Result, error) {
	return runWithResolvedRoot(opts, func(root string) (*Result, error) {
		var violations []string

	if len(opts.Files) > 0 {
		for _, f := range opts.Files {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}

			fullPath := f
			if !filepath.IsAbs(fullPath) {
				fullPath = filepath.Join(root, f)
			}

			if isSecretScannerExemptTestFixture(fullPath) {
				continue
			}

			v, err := scanFileForSecrets(fullPath, root)
			if err != nil {
				continue
			}
			violations = append(violations, v...)
		}
	} else {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			if d.IsDir() {
				name := d.Name()
				if excludedDirNames[name] || strings.HasPrefix(name, ".zqk-") {
					return filepath.SkipDir
				}
				return nil
			}

			ext := strings.ToLower(filepath.Ext(path))
			if binaryExts[ext] {
				return nil
			}

			if isSecretScannerExemptTestFixture(path) {
				return nil
			}

			v, scanErr := scanFileForSecrets(path, root)
			if scanErr == nil && len(v) > 0 {
				violations = append(violations, v...)
			}
			return nil
		})

		if err != nil && err != context.Canceled {
			return nil, fmt.Errorf("failed during secret scan walk: %w", err)
		}
	}

	if len(violations) > 0 {
		return &Result{
			GateName:   g.Name(),
			Passed:     false,
			Message:    fmt.Sprintf("Secret scanner detected %d credential leak(s)", len(violations)),
			Violations: violations,
		}, nil
	}

	return &Result{
		GateName: g.Name(),
		Passed:   true,
		Message:  "Zero secrets or credential leaks detected",
	}, nil
	})
}

func scanFileForSecrets(filePath, root string) ([]string, error) {
	fi, err := os.Stat(filePath)
	if err != nil || fi.IsDir() || fi.Size() > 5*1024*1024 {
		return nil, err
	}

	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rel, err := filepath.Rel(root, filePath)
	if err != nil {
		rel = filePath
	}

	var hits []string
	reader := bufio.NewReader(f)
	lineNum := 0

	for {
		lineNum++
		line, readErr := reader.ReadString('\n')
		if len(line) > 0 {
			if secretPatterns.MatchString(line) {
				match := secretPatterns.FindString(line)
				redacted := redactSecret(match)
				hits = append(hits, fmt.Sprintf("%s:%d: matched pattern [%s]", rel, lineNum, redacted))
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			break
		}
	}

	return hits, nil
}

func redactSecret(secret string) string {
	return secretpatterns.Redact(secret)
}

// isSecretScannerExemptTestFixture reports whether a file is a test or script fixture
// that intentionally contains synthetic secret tokens (such as secret scanner unit tests,
// security policy tests, or escalation test fixtures) and should be excluded from scanning.
func isSecretScannerExemptTestFixture(path string) bool {
	base := filepath.Base(path)
	isTestOrScript := strings.HasSuffix(base, "_test.go") || strings.HasSuffix(base, ".sh")
	if !isTestOrScript {
		return false
	}
	return strings.Contains(base, "secret") ||
		strings.Contains(base, "policy") ||
		strings.Contains(base, "escalat")
}
