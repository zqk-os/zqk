package policy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DocLinksGate validates documentation links and Divio quadrant structure.
type DocLinksGate struct{}

func (g *DocLinksGate) Name() string {
	return "doc-links"
}

func (g *DocLinksGate) Description() string {
	return "Validates documentation links in docs/INDEX.md, Divio quadrant structure, and community files"
}

var mdLinkRegex = regexp.MustCompile(`\[.*?\]\((.*?)\)`)

func (g *DocLinksGate) Run(ctx context.Context, opts RunOptions) (*Result, error) {
	return runWithResolvedRoot(opts, func(root string) (*Result, error) {
		var violations []string

	// 1. Verify docs/INDEX.md
	indexPath := filepath.Join(root, "docs", "INDEX.md")
	content, err := os.ReadFile(indexPath)
	if err != nil {
		violations = append(violations, fmt.Sprintf("docs/INDEX.md does not exist or cannot be read: %v", err))
	} else {
		matches := mdLinkRegex.FindAllStringSubmatch(string(content), -1)
		linkCount := 0
		for _, m := range matches {
			if len(m) < 2 {
				continue
			}
			link := strings.TrimSpace(m[1])
			if strings.HasPrefix(link, "http://") || strings.HasPrefix(link, "https://") || strings.HasPrefix(link, "#") {
				continue
			}
			linkCount++
			cleanLink := strings.Split(link, "#")[0]
			target := filepath.Clean(filepath.Join(root, "docs", cleanLink))
			if _, statErr := os.Stat(target); os.IsNotExist(statErr) {
				violations = append(violations, fmt.Sprintf("Broken link in docs/INDEX.md: %s -> %s", link, cleanLink))
			}
		}
	}

	// 2. Verify Divio quadrants
	quadrants := []string{"tutorials", "howto", "manual", "explanation"}
	for _, q := range quadrants {
		qDir := filepath.Join(root, "docs", q)
		info, statErr := os.Stat(qDir)
		if statErr != nil || !info.IsDir() {
			violations = append(violations, fmt.Sprintf("Required Divio directory does not exist: docs/%s", q))
			continue
		}

		entries, readErr := os.ReadDir(qDir)
		if readErr != nil {
			violations = append(violations, fmt.Sprintf("Cannot read Divio directory docs/%s: %v", q, readErr))
			continue
		}

		mdCount := 0
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
				mdCount++
			}
		}
		if mdCount == 0 {
			violations = append(violations, fmt.Sprintf("Divio directory docs/%s contains no markdown documents", q))
		}
	}

	// 3. Verify community files
	communityFiles := []string{
		"CONTRIBUTING.md",
		"CODE_OF_CONDUCT.md",
		filepath.Join(".github", "PULL_REQUEST_TEMPLATE.md"),
	}
	for _, cf := range communityFiles {
		cfPath := filepath.Join(root, cf)
		if _, statErr := os.Stat(cfPath); os.IsNotExist(statErr) {
			violations = append(violations, fmt.Sprintf("Required community file does not exist: %s", cf))
		}
	}

	if len(violations) > 0 {
		return &Result{
			GateName:   g.Name(),
			Passed:     false,
			Message:    fmt.Sprintf("Found %d documentation/structure violations", len(violations)),
			Violations: violations,
		}, nil
	}

	return &Result{
		GateName: g.Name(),
		Passed:   true,
		Message:  "All documentation links, Divio quadrants, and community files verified",
	}, nil
	})
}
