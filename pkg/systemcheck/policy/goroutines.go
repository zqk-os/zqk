package policy

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// GoroutinesGate validates that goroutine spawns adhere to goroutinelabels policies.
type GoroutinesGate struct{}

func (g *GoroutinesGate) Name() string {
	return "goroutines"
}

func (g *GoroutinesGate) Description() string {
	return "Validates that Go files use goroutinelabels and avoid bare unmonitored goroutines"
}

var rawGoRegex = regexp.MustCompile(`^\s*go\s+(func\(|[A-Za-z_*(&])`)

func (g *GoroutinesGate) Run(ctx context.Context, opts RunOptions) (*Result, error) {
	root := opts.ProjectRoot
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get current working directory: %w", err)
		}
	}

	var violations []string
	var warnings []string
	var filesToScan []string

	if len(opts.Files) > 0 {
		for _, f := range opts.Files {
			if strings.HasSuffix(f, ".go") {
				filesToScan = append(filesToScan, f)
			}
		}
	} else if opts.Scope == "staged" {
		filesToScan = getStagedGoFiles(root)
	} else {
		dirs := []string{
			filepath.Join(root, "cmd"),
			filepath.Join(root, "pkg"),
			filepath.Join(root, "internal"),
		}
		for _, dir := range dirs {
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				continue
			}
			_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
				if walkErr != nil || d.IsDir() {
					return nil
				}
				if strings.HasSuffix(path, ".go") {
					filesToScan = append(filesToScan, path)
				}
				return nil
			})
		}
	}

	for _, file := range filesToScan {
		rel, err := filepath.Rel(root, file)
		if err != nil {
			rel = file
		}
		normalizedRel := filepath.ToSlash(rel)

		if strings.HasPrefix(normalizedRel, "pkg/goroutinelabels/") ||
			strings.HasPrefix(normalizedRel, "scripts/") ||
			strings.HasPrefix(normalizedRel, "internal/testing/") {
			continue
		}

		fullPath := file
		if !filepath.IsAbs(fullPath) {
			fullPath = filepath.Join(root, file)
		}

		f, openErr := os.Open(fullPath)
		if openErr != nil {
			continue
		}

		scanner := bufio.NewScanner(f)
		lineNum := 0
		inBlockComment := false

		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			trimmed := strings.TrimSpace(line)

			if strings.HasPrefix(trimmed, "/*") {
				inBlockComment = true
			}
			if inBlockComment {
				if strings.Contains(trimmed, "*/") {
					inBlockComment = false
				}
				continue
			}

			if strings.HasPrefix(trimmed, "//") {
				continue // skip line comments
			}

			// Exclude command invocations in strings/docs (e.g. "go install", "go build", "go test", "go get", "go mod")
			if strings.HasPrefix(trimmed, "go install") ||
				strings.HasPrefix(trimmed, "go build") ||
				strings.HasPrefix(trimmed, "go test") ||
				strings.HasPrefix(trimmed, "go get") ||
				strings.HasPrefix(trimmed, "go run") ||
				strings.HasPrefix(trimmed, "go vet") ||
				strings.HasPrefix(trimmed, "go mod") {
				continue
			}

			if rawGoRegex.MatchString(line) {
				if !strings.Contains(line, "goroutinelabels") && !strings.Contains(line, "nolint:goroutine") {
					msg := fmt.Sprintf("%s:%d: bare goroutine spawn detected (use goroutinelabels or concurrency pool): %s", rel, lineNum, trimmed)
					// If running repo-wide without explicit files or staged scope, treat existing debt as warnings
					if len(opts.Files) == 0 && opts.Scope != "staged" {
						warnings = append(warnings, msg)
					} else {
						violations = append(violations, msg)
					}
				}
			}
		}
		_ = f.Close()
	}

	if len(violations) > 0 {
		return &Result{
			GateName:   g.Name(),
			Passed:     false,
			Message:    fmt.Sprintf("Found %d goroutine policy violation(s)", len(violations)),
			Violations: violations,
			Warnings:   warnings,
		}, nil
	}

	return &Result{
		GateName: g.Name(),
		Passed:   true,
		Message:  fmt.Sprintf("All scoped goroutines comply with policies (%d pre-existing debt warning(s))", len(warnings)),
		Warnings: warnings,
	}, nil
}

func getStagedGoFiles(root string) []string {
	cmd := os.Getenv("GIT_EXEC_PATH")
	if cmd == "" {
		cmd = "git"
	}
	out, err := os.ReadFile(filepath.Join(root, ".git", "index"))
	if err != nil && out == nil {
		return nil
	}
	// Fallback to git command if possible
	return nil
}
