package testdiscovery

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"
)

// TypeScriptDiscoverer statically discovers tests in TypeScript/JavaScript test files.
type TypeScriptDiscoverer struct{}

func NewTypeScriptDiscoverer() *TypeScriptDiscoverer {
	return &TypeScriptDiscoverer{}
}

func (d *TypeScriptDiscoverer) Language() string {
	return "typescript"
}

func (d *TypeScriptDiscoverer) CanHandle(relPath string) bool {
	return strings.HasSuffix(relPath, ".test.ts") ||
		strings.HasSuffix(relPath, ".spec.ts") ||
		strings.HasSuffix(relPath, ".test.tsx") ||
		strings.HasSuffix(relPath, ".spec.tsx") ||
		strings.HasSuffix(relPath, ".test.js") ||
		strings.HasSuffix(relPath, ".spec.js")
}

var (
	tsDescribeRegex = regexp.MustCompile(`(?:describe|suite)\s*\(\s*['"\x60]([^'"\x60]+)['"\x60]`)
	tsTestRegex     = regexp.MustCompile(`(?:it|test)\s*\(\s*['"\x60]([^'"\x60]+)['"\x60]`)
)

func (d *TypeScriptDiscoverer) Discover(ctx context.Context, projectRoot, relPath string, content []byte) ([]DiscoveredTarget, error) {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	var targets []DiscoveredTarget
	var currentSuite string
	var pendingTags []string
	var pendingCrit []string
	var pendingReq []string

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Extract comments & annotations
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
			cleanComment := strings.TrimLeft(trimmed, "/* \t")
			if m := critRegex.FindStringSubmatch(cleanComment); len(m) > 1 {
				for _, p := range strings.Split(m[1], ",") {
					p = strings.TrimSpace(p)
					if p != "" {
						pendingCrit = append(pendingCrit, p)
					}
				}
			}
			if m := reqRegex.FindStringSubmatch(cleanComment); len(m) > 1 {
				for _, p := range strings.Split(m[1], ",") {
					p = strings.TrimSpace(p)
					if p != "" {
						pendingReq = append(pendingReq, p)
					}
				}
			}
			if m := tagRegex.FindStringSubmatch(cleanComment); len(m) > 1 {
				pendingTags = append(pendingTags, strings.TrimSpace(m[1]))
			}
			continue
		}

		if m := tsDescribeRegex.FindStringSubmatch(line); len(m) > 1 {
			currentSuite = m[1]
			continue
		}

		if m := tsTestRegex.FindStringSubmatch(line); len(m) > 1 {
			testName := m[1]
			fullTarget := testName
			if currentSuite != "" {
				fullTarget = currentSuite + " > " + testName
			}

			execCmd := fmt.Sprintf("npm test -- -t '%s'", testName)

			target := DiscoveredTarget{
				Path:             relPath,
				Language:         "typescript",
				Suite:            currentSuite,
				Function:         fullTarget,
				Line:             lineNum,
				Tags:             append([]string{}, pendingTags...),
				CriteriaRefs:     append([]string{}, pendingCrit...),
				RequirementRefs:  append([]string{}, pendingReq...),
				ExecutionCommand: execCmd,
			}

			targets = append(targets, target)
			pendingTags = nil
			pendingCrit = nil
			pendingReq = nil
		}
	}

	return targets, scanner.Err()
}
