package testdiscovery

import (
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
	state := newLineDiscoveryState(content)

	for state.scanner.Scan() {
		state.lineNum++
		line := state.scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Extract comments & annotations
		if strings.HasPrefix(trimmed, "#") {
			crit, req, tags := extractCommentMetadata(trimmed)
			state.appendMetadata(crit, req, tags)
			continue
		}

		if m := pyMarkRegex.FindStringSubmatch(trimmed); len(m) > 1 {
			state.pendingTags = append(state.pendingTags, m[1])
			continue
		}

		if m := pyClassRegex.FindStringSubmatch(line); len(m) > 1 {
			state.currentSuite = m[1]
			continue
		}

		if m := pyFuncRegex.FindStringSubmatch(line); len(m) > 1 {
			funcName := m[1]
			fullTarget := funcName
			if state.currentSuite != "" && !strings.HasPrefix(line, "def ") {
				fullTarget = state.currentSuite + "::" + funcName
			}

			execCmd := fmt.Sprintf("pytest %s::%s", relPath, fullTarget)

			target := DiscoveredTarget{
				Path:             relPath,
				Language:         "python",
				Suite:            state.currentSuite,
				Function:         funcName,
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
