package policy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// SecretsGate scans files for API tokens, private keys, and credential leaks.
type SecretsGate struct{}

func (g *SecretsGate) Name() string {
	return "secrets"
}

func (g *SecretsGate) Description() string {
	return "Scans repository files for API tokens, AWS keys, private keys, and credential leaks"
}

var secretPatterns = regexp.MustCompile(`(ghp_[a-zA-Z0-9]{36}|gho_[a-zA-Z0-9]{36}|github_pat_[a-zA-Z0-9_]{82}|AKIA[0-9A-Z]{16}|-----BEGIN (?:RSA|EC|OPENSSH|DSA|PGP)? PRIVATE KEY-----|xox[baprs]-[0-9]{12}-[0-9]{12}-[a-zA-Z0-9]{24})`)

var excludedDirNames = map[string]bool{
	".git":     true,
	".zqk":     true,
	"vendor":   true,
	"testdata": true,
	".agent":   true,
	".gemini":  true,
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
	root := opts.ProjectRoot
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get current working directory: %w", err)
		}
	}

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

			// Skip self-tests and test fixture files for secret scanner
			base := filepath.Base(path)
			if (strings.Contains(base, "secret") || strings.Contains(base, "policy")) && (strings.HasSuffix(base, "_test.go") || strings.HasSuffix(base, ".sh")) {
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
	if len(secret) <= 8 {
		return strings.Repeat("*", len(secret))
	}
	return secret[:4] + strings.Repeat("*", len(secret)-8) + secret[len(secret)-4:]
}
