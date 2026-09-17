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
	scanner := bufio.NewScanner(bytes.NewReader(content))
	var targets []DiscoveredTarget
	var currentSuite string
	var pendingTags []string
	var pendingCrit []string
	var pendingReq []string
	hasTestAttr := false

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Extract doc/line comments for criteria & requirements
		if strings.HasPrefix(trimmed, "//") {
			commentBody := strings.TrimPrefix(trimmed, "//")
			if m := critRegex.FindStringSubmatch(commentBody); len(m) > 1 {
				for _, p := range strings.Split(m[1], ",") {
					p = strings.TrimSpace(p)
					if p != "" {
						pendingCrit = append(pendingCrit, p)
					}
				}
			}
			if m := reqRegex.FindStringSubmatch(commentBody); len(m) > 1 {
				for _, p := range strings.Split(m[1], ",") {
					p = strings.TrimSpace(p)
					if p != "" {
						pendingReq = append(pendingReq, p)
					}
				}
			}
			continue
		}

		// Detect test module: mod tests { ... }
		if m := rustModTestRegex.FindStringSubmatch(line); len(m) > 1 {
			currentSuite = m[1]
			continue
		}

		// Check for #[test] or #[tokio::test]
		if rustTestAttrRegex.MatchString(trimmed) {
			hasTestAttr = true
			continue
		}

		// Check for #[ignore]
		if rustIgnoreRegex.MatchString(trimmed) {
			pendingTags = append(pendingTags, "ignore")
			continue
		}

		// Match function declaration after #[test]
		if hasTestAttr {
			if m := rustFnRegex.FindStringSubmatch(line); len(m) > 1 {
				fnName := m[1]
				suite := currentSuite
				if suite == "" {
					base := filepath.Base(relPath)
					suite = strings.TrimSuffix(base, ".rs")
				}

				cmd := fmt.Sprintf("cargo test %s", fnName)

				targets = append(targets, DiscoveredTarget{
					ID:               fmt.Sprintf("rust:%s:%s", relPath, fnName),
					Path:             relPath,
					Language:         "rust",
					Suite:            suite,
					Function:         fnName,
					Line:             lineNum,
					Tags:             pendingTags,
					CriteriaRefs:     pendingCrit,
					RequirementRefs:  pendingReq,
					ExecutionCommand: cmd,
				})

				hasTestAttr = false
				pendingTags = nil
				pendingCrit = nil
				pendingReq = nil
				continue
			}

			// If other annotations intervene, keep hasTestAttr active; otherwise reset on empty lines/closing braces
			if trimmed == "" || trimmed == "}" {
				hasTestAttr = false
			}
		}
	}

	return targets, scanner.Err()
}
