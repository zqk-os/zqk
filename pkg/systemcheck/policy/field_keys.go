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

// FieldKeysGate checks that object field keys are referenced via objects.FieldKey* constants
// rather than raw string literals in Go source files.
type FieldKeysGate struct{}

func (g *FieldKeysGate) Name() string {
	return "field-keys"
}

func (g *FieldKeysGate) Description() string {
	return "Asserts that system object field access uses typed objects.FieldKey* constants instead of raw string literals"
}

var keyDefRegex = regexp.MustCompile(`FieldKey[a-zA-Z0-9_]+\s*=\s*"([^"]+)"`)

func (g *FieldKeysGate) Run(ctx context.Context, opts RunOptions) (*Result, error) {
	return runWithResolvedRoot(opts, func(root string) (*Result, error) {
		fieldKeysFile := filepath.Join(root, "pkg", "objects", "field_keys.go")
	content, err := os.ReadFile(fieldKeysFile)
	if err != nil {
		return &Result{
			GateName: g.Name(),
			Passed:   true,
			Message:  "pkg/objects/field_keys.go not found, skipping field key checks",
		}, nil
	}

	keysMap := make(map[string]bool)
	matches := keyDefRegex.FindAllStringSubmatch(string(content), -1)
	for _, m := range matches {
		if len(m) >= 2 {
			keysMap[m[1]] = true
		}
	}

	if len(keysMap) == 0 {
		return &Result{
			GateName: g.Name(),
			Passed:   true,
			Message:  "No canonical field keys extracted from pkg/objects/field_keys.go",
		}, nil
	}

	var filesToScan []string
	if len(opts.Files) > 0 {
		for _, f := range opts.Files {
			if strings.HasSuffix(f, ".go") {
				filesToScan = append(filesToScan, f)
			}
		}
	}

	if len(filesToScan) == 0 {
		return &Result{
			GateName: g.Name(),
			Passed:   true,
			Message:  fmt.Sprintf("Loaded %d canonical field keys; no scoped Go files specified to scan", len(keysMap)),
		}, nil
	}

	var violations []string
	for _, file := range filesToScan {
		rel, relErr := filepath.Rel(root, file)
		if relErr != nil {
			rel = file
		}
		normalized := filepath.ToSlash(rel)

		if normalized == "pkg/objects/field_keys.go" ||
			strings.HasPrefix(normalized, "pkg/specbuilder/") ||
			strings.HasSuffix(normalized, "_test.go") {
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
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") {
				continue
			}

			for key := range keysMap {
				pattern := fmt.Sprintf(`"%s"`, key)
				if strings.Contains(line, pattern) {
					if strings.Contains(line, fmt.Sprintf(`:"%s"`, key)) || strings.Contains(line, fmt.Sprintf(`,"%s"`, key)) {
						continue
					}
					if strings.Contains(line, "FieldKey") {
						continue
					}
					violations = append(violations, fmt.Sprintf("%s:%d: literal string %q used instead of objects.FieldKey constant", rel, lineNum, key))
					break
				}
			}
		}
		_ = f.Close()
	}

	if len(violations) > 0 {
		return &Result{
			GateName:   g.Name(),
			Passed:     false,
			Message:    fmt.Sprintf("Detected %d raw field key literal violation(s)", len(violations)),
			Violations: violations,
		}, nil
	}

	return &Result{
		GateName: g.Name(),
		Passed:   true,
		Message:  "All Go files use typed objects.FieldKey constants",
	}, nil
	})
}
