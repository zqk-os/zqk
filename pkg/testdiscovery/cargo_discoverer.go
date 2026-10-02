package testdiscovery

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// CargoDiscoverer statically discovers tests in Rust source files (.rs) without code execution.
type CargoDiscoverer struct{}

func NewCargoDiscoverer() *CargoDiscoverer {
	return &CargoDiscoverer{}
}

func (d *CargoDiscoverer) Language() string {
	return "rust"
}

func (d *CargoDiscoverer) CanHandle(relPath string) bool {
	return strings.HasSuffix(relPath, ".rs")
}

var (
	rustTestAttrRegex = regexp.MustCompile(`^\s*#\[(?:.*::)?test\]`)
	rustIgnoreRegex   = regexp.MustCompile(`^\s*#\[ignore\]`)
	rustFnRegex       = regexp.MustCompile(`^\s*(?:pub\s+)?(?:async\s+)?fn\s+([a-zA-Z0-9_]+)\s*\(`)
	rustModTestRegex  = regexp.MustCompile(`^\s*(?:pub\s+)?mod\s+(tests?|[a-zA-Z0-9_]+_tests?)\s*\{?`)
)

func (d *CargoDiscoverer) Discover(ctx context.Context, projectRoot, relPath string, content []byte) ([]DiscoveredTarget, error) {
	state := newLineDiscoveryState(content)
	hasTestAttr := false

	for state.scanner.Scan() {
		state.lineNum++
		line := state.scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Extract doc/line comments for criteria & requirements
		if strings.HasPrefix(trimmed, "//") {
			commentBody := strings.TrimPrefix(trimmed, "//")
			crit, req := extractCriteriaAndReqs(commentBody)
			state.appendMetadata(crit, req, nil)
			continue
		}

		// Detect test module: mod tests { ... }
		if m := rustModTestRegex.FindStringSubmatch(line); len(m) > 1 {
			state.currentSuite = m[1]
			continue
		}

		// Check for #[test] or #[tokio::test]
		if rustTestAttrRegex.MatchString(trimmed) {
			hasTestAttr = true
			continue
		}

		// Check for #[ignore]
		if rustIgnoreRegex.MatchString(trimmed) {
			state.pendingTags = append(state.pendingTags, "ignore")
			continue
		}

		// Match function declaration after #[test]
		if hasTestAttr {
			if m := rustFnRegex.FindStringSubmatch(line); len(m) > 1 {
				fnName := m[1]
				suite := state.currentSuite
				if suite == "" {
					base := filepath.Base(relPath)
					suite = strings.TrimSuffix(base, ".rs")
				}

				cmd := fmt.Sprintf("cargo test %s", fnName)

				state.targets = append(state.targets, DiscoveredTarget{
					ID:               fmt.Sprintf("rust:%s:%s", relPath, fnName),
					Path:             relPath,
					Language:         "rust",
					Suite:            suite,
					Function:         fnName,
					Line:             state.lineNum,
					Tags:             state.pendingTags,
					CriteriaRefs:     state.pendingCrit,
					RequirementRefs:  state.pendingReq,
					ExecutionCommand: cmd,
				})

				hasTestAttr = false
				state.resetPending()
				continue
			}

			// If other annotations intervene, keep hasTestAttr active; otherwise reset on empty lines/closing braces
			if trimmed == "" || trimmed == "}" {
				hasTestAttr = false
			}
		}
	}

	return state.targets, state.scanner.Err()
}
