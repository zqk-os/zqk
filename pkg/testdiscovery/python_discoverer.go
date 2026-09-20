package testdiscovery

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// PythonDiscoverer statically discovers tests in Python files without code execution.
type PythonDiscoverer struct{}

func NewPythonDiscoverer() *PythonDiscoverer {
	return &PythonDiscoverer{}
}

func (d *PythonDiscoverer) Language() string {
	return "python"
}

func (d *PythonDiscoverer) CanHandle(relPath string) bool {
	base := filepath.Base(relPath)
	return strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py") ||
		strings.HasSuffix(base, "_test.py")
}

var (
	pyFuncRegex  = regexp.MustCompile(`^\s*def\s+(test_[a-zA-Z0-9_]+)\s*\(`)
	pyClassRegex = regexp.MustCompile(`^\s*class\s+(Test[a-zA-Z0-9_]+)`)
	pyMarkRegex  = regexp.MustCompile(`@pytest\.mark\.([a-zA-Z0-9_]+)`)
)

func (d *PythonDiscoverer) Discover(ctx context.Context, projectRoot, relPath string, content []byte) ([]DiscoveredTarget, error) {
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
		if strings.HasPrefix(trimmed, "#") {
			if m := critRegex.FindStringSubmatch(trimmed); len(m) > 1 {
				for _, p := range strings.Split(m[1], ",") {
					p = strings.TrimSpace(p)
					if p != "" {
						pendingCrit = append(pendingCrit, p)
					}
				}
			}
			if m := reqRegex.FindStringSubmatch(trimmed); len(m) > 1 {
				for _, p := range strings.Split(m[1], ",") {
					p = strings.TrimSpace(p)
					if p != "" {
						pendingReq = append(pendingReq, p)
					}
				}
			}
			if m := tagRegex.FindStringSubmatch(trimmed); len(m) > 1 {
				pendingTags = append(pendingTags, strings.TrimSpace(m[1]))
			}
			continue
		}

		if m := pyMarkRegex.FindStringSubmatch(trimmed); len(m) > 1 {
			pendingTags = append(pendingTags, m[1])
			continue
		}

		if m := pyClassRegex.FindStringSubmatch(line); len(m) > 1 {
			currentSuite = m[1]
			continue
		}

		if m := pyFuncRegex.FindStringSubmatch(line); len(m) > 1 {
			funcName := m[1]
			fullTarget := funcName
			if currentSuite != "" && !strings.HasPrefix(line, "def ") {
				fullTarget = currentSuite + "::" + funcName
			}

			execCmd := fmt.Sprintf("pytest %s::%s", relPath, fullTarget)

			target := DiscoveredTarget{
				Path:             relPath,
				Language:         "python",
				Suite:            currentSuite,
				Function:         funcName,
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
