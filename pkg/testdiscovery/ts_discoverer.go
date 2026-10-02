package testdiscovery

import (
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
	state := newLineDiscoveryState(content)

	for state.scanner.Scan() {
		state.lineNum++
		line := state.scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Extract comments & annotations
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
			cleanComment := strings.TrimLeft(trimmed, "/* \t")
			crit, req, tags := extractCommentMetadata(cleanComment)
			state.appendMetadata(crit, req, tags)
			continue
		}

		if m := tsDescribeRegex.FindStringSubmatch(line); len(m) > 1 {
			state.currentSuite = m[1]
			continue
		}

		if m := tsTestRegex.FindStringSubmatch(line); len(m) > 1 {
			testName := m[1]
			fullTarget := testName
			if state.currentSuite != "" {
				fullTarget = state.currentSuite + " > " + testName
			}

			execCmd := fmt.Sprintf("npm test -- -t '%s'", testName)

			target := DiscoveredTarget{
				Path:             relPath,
				Language:         "typescript",
				Suite:            state.currentSuite,
				Function:         fullTarget,
				Line:             state.lineNum,
				Tags:             append([]string{}, state.pendingTags...),
				CriteriaRefs:     append([]string{}, state.pendingCrit...),
				RequirementRefs:  append([]string{}, state.pendingReq...),
				ExecutionCommand: execCmd,
			}

			state.targets = append(state.targets, target)
			state.resetPending()
		}
	}

	return state.targets, state.scanner.Err()
}
