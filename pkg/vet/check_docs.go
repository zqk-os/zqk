package vet

import (
	"bufio"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/zqk-os/zqk/pkg/acronyms"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	checkIDTopicBudget    = "doc/topic_character_budget"
	checkIDKernelIDLeak   = "doc/kernel_id_leak"
	checkIDCycleNarrative = "doc/cycle_narrative_leak"
	checkIDBadAcronym     = "doc/contradictory_acronym"
	suiteDocs             = "docs"
)

var (
	// Leaked project-specific kernel ID pattern (e.g. nanos-hex IDs)
	kernelIDRegex = regexp.MustCompile(`\b(BLI|REQ|PRI|TST|CRIT|GOAL|MIL|ACC)-179\d{16}-[0-9a-f]{8}\b`)

	// Cycle sprint implementation narrative patterns
	cycleNarrativeRegex = regexp.MustCompile(`(?i)\b(Cycle \d+ (Target Verified|achievement|elevation|sprint)|achieved in Cycle \d+|In Cycle \d+, statement|Execution Cycle \d+ Work Plan)\b`)

	// Heading pattern for high-level topics
	h2Regex = regexp.MustCompile(`(?m)^##\s+(.+)$`)
)

// Known contradictory expansions to detect and reject
var knownAcronymContradictions = map[string][]string{
	"ATK": {
		"adversarial test kit",
		"automated test kit",
	},
	"CAS": {
		"centralized application server",
		"content addressable server",
	},
	"VDS": {
		"virtual data store",
		"verifiable done-gates specification",
		"verifiable done-gate specification",
		"virtual dataset",
	},
	"CAP": {
		"continuous autonomy pipeline",
		"continuous automated pipeline",
	},
}

// CheckDocumentationQuality inspects markdown documentation for verbosity,
// contradictory acronyms, cycle sprint leakage, and leaked project-specific kernel IDs.
func CheckDocumentationQuality(root string, cfg *GatesConfig) ([]Finding, error) {
	if cfg != nil && !cfg.DocScrutiny.Enabled {
		return nil, nil
	}

	maxConceptChars := 4000
	maxArchChars := 8000
	if cfg != nil && cfg.DocScrutiny.MaxTopicCharsConcept > 0 {
		maxConceptChars = cfg.DocScrutiny.MaxTopicCharsConcept
	}
	if cfg != nil && cfg.DocScrutiny.MaxTopicCharsArchitecture > 0 {
		maxArchChars = cfg.DocScrutiny.MaxTopicCharsArchitecture
	}

	var exemptions []string
	if cfg != nil {
		exemptions = cfg.DocScrutiny.Exemptions
	}

	// Always exempt releases, archive, internal reports, root catalog, and internal code packages from public doc scrutiny
	defaultExemptions := []string{
		"docs/releases/*",
		"docs/archive/*",
		"docs/reports/*",
		"docs/guides/README.md",
		".zqk/process/*",
		"pkg/*",
		"cmd/*",
		"internal/*",
		"*_test.go",
	}
	exemptions = append(exemptions, defaultExemptions...)

	docsDir := filepath.Join(root, "docs")
	if _, err := fileutil.Stat(docsDir); fileutil.IsNotExist(err) {
		return nil, nil
	}

	canonicalAcronyms := make(map[string]string)
	for _, a := range acronyms.CanonicalSeedTerms() {
		canonicalAcronyms[a.Code] = a.FullName
	}

	var findings []Finding

	err := filepath.Walk(docsDir, func(path string, info fileutil.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return nil
		}

		if !strings.HasSuffix(strings.ToLower(info.Name()), ".md") {
			return nil
		}

		relPath, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		relPathSlash := filepath.ToSlash(relPath)

		if isPathExempt(relPathSlash, exemptions) {
			return nil
		}

		contentBytes, err := fileutil.ReadFile(path)
		if err != nil {
			return nil
		}
		content := string(contentBytes)

		// 1. Topic Character Budget Check
		isArchitecture := strings.Contains(relPathSlash, "docs/architecture/") || strings.Contains(relPathSlash, "docs/specs/")
		maxTopicChars := maxConceptChars
		if isArchitecture {
			maxTopicChars = maxArchChars
		}

		failOnTopicOverflow := cfg != nil && cfg.DocScrutiny.FailOnTopicOverflow
		topicFindings := checkTopicCharacterBudgets(relPathSlash, content, maxTopicChars, failOnTopicOverflow)
		findings = append(findings, topicFindings...)

		// 2. Leaked Kernel ID Sanitization Check (public guides & tutorials)
		if strings.Contains(relPathSlash, "docs/guides/") || strings.Contains(relPathSlash, "docs/tutorials/") {
			idFindings := checkKernelIDLeaks(relPathSlash, content)
			findings = append(findings, idFindings...)
		}

		// 3. Cycle Implementation Narrative Purging Check
		if strings.Contains(relPathSlash, "docs/guides/") || strings.Contains(relPathSlash, "docs/tutorials/") {
			cycleFindings := checkCycleNarratives(relPathSlash, content)
			findings = append(findings, cycleFindings...)
		}

		// 4. Canonical Acronym Consistency Check
		acronymFindings := checkAcronymConsistency(relPathSlash, content, canonicalAcronyms)
		findings = append(findings, acronymFindings...)

		return nil
	})

	return findings, err
}

func checkTopicCharacterBudgets(file, content string, maxChars int, failOnOverflow bool) []Finding {
	var findings []Finding

	severity := SeverityWarn
	if failOnOverflow {
		severity = SeverityError
	}

	sections := h2Regex.Split(content, -1)
	matches := h2Regex.FindAllStringSubmatch(content, -1)

	// First section before any H2
	if len(sections) > 0 {
		introLen := len(strings.TrimSpace(sections[0]))
		if introLen > maxChars {
			findings = append(findings, Finding{
				CheckID:  checkIDTopicBudget,
				Suite:    suiteDocs,
				File:     file,
				Line:     1,
				Message:  fmt.Sprintf("Introduction topic exceeds character budget (%d > %d characters)", introLen, maxChars),
				Severity: severity,
			})
		}
	}

	for i, match := range matches {
		if i+1 < len(sections) {
			sectionTitle := match[1]
			sectionBody := strings.TrimSpace(sections[i+1])
			bodyLen := len(sectionBody)
			if bodyLen > maxChars {
				findings = append(findings, Finding{
					CheckID:  checkIDTopicBudget,
					Suite:    suiteDocs,
					File:     file,
					Line:     1,
					Message:  fmt.Sprintf("Topic '## %s' exceeds character budget (%d > %d characters)", sectionTitle, bodyLen, maxChars),
					Severity: severity,
				})
			}
		}
	}

	return findings
}

func scanDocumentLines(content string, handler func(lineNum int, line string) []Finding) []Finding {
	var findings []Finding
	scanner := bufio.NewScanner(strings.NewReader(content))
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if res := handler(lineNum, scanner.Text()); len(res) > 0 {
			findings = append(findings, res...)
		}
	}
	return findings
}

func checkKernelIDLeaks(file, content string) []Finding {
	inCodeBlock := false
	return scanDocumentLines(content, func(lineNum int, line string) []Finding {
		var lineFindings []Finding
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			return nil
		}

		if inCodeBlock && (strings.Contains(trimmed, "yaml") || strings.Contains(trimmed, "json")) {
			return nil
		}

		matches := kernelIDRegex.FindAllString(line, -1)
		for _, m := range matches {
			lineFindings = append(lineFindings, Finding{
				CheckID:  checkIDKernelIDLeak,
				Suite:    suiteDocs,
				File:     file,
				Line:     lineNum,
				Message:  fmt.Sprintf("Leaked project-specific kernel ID '%s' in public documentation", m),
				Severity: SeverityError,
			})
		}
		return lineFindings
	})
}

func checkCycleNarratives(file, content string) []Finding {
	return scanDocumentLines(content, func(lineNum int, line string) []Finding {
		if cycleNarrativeRegex.MatchString(line) {
			return []Finding{{
				CheckID:  checkIDCycleNarrative,
				Suite:    suiteDocs,
				File:     file,
				Line:     lineNum,
				Message:  fmt.Sprintf("Internal cycle sprint narrative leaked in public documentation: %q", strings.TrimSpace(line)),
				Severity: SeverityError,
			}}
		}
		return nil
	})
}

func checkAcronymConsistency(file, content string, canonical map[string]string) []Finding {
	return scanDocumentLines(content, func(lineNum int, line string) []Finding {
		var lineFindings []Finding
		lineLower := strings.ToLower(line)

		for acr, contradictions := range knownAcronymContradictions {
			if !strings.Contains(line, acr) {
				continue
			}
			for _, c := range contradictions {
				if strings.Contains(lineLower, c) {
					canon := canonical[acr]
					lineFindings = append(lineFindings, Finding{
						CheckID:  checkIDBadAcronym,
						Suite:    suiteDocs,
						File:     file,
						Line:     lineNum,
						Message:  fmt.Sprintf("Contradictory acronym definition for %s: found %q, canonical is %q", acr, c, canon),
						Severity: SeverityError,
					})
				}
			}
		}
		return lineFindings
	})
}
