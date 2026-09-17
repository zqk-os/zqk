package docman

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

const (
	docStatusActive           = objects.ObjectStatusActive
	docStatusDeprecated       = objects.ObjectStatusDeprecated
	docStatusDesignComplete   = "design complete"
	docStatusImplemented      = "implemented"
	docStatusResearchComplete = "research complete"
)

// DocumentMetadata contains extracted metadata from a markdown file
type DocumentMetadata struct {
	Title    string
	Summary  string
	Status   string
	Group    string
	Category string
}

// Parser extracts metadata from markdown files
type Parser struct{}

// NewParser creates a new markdown parser
func NewParser() *Parser {
	return &Parser{}
}

// Parse extracts metadata from a markdown file
func (p *Parser) Parse(filePath string) (*DocumentMetadata, error) {
	data, err := fileutil.ReadFile(filePath)
	if err != nil {
		return nil, errfmt.Newf("failed to read file").Wrap(err)
	}

	content := string(data)
	lines := strings.Split(content, "\n")

	metadata := &DocumentMetadata{
		Status: docStatusActive,
	}

	// Extract title (first H1)
	metadata.Title = p.extractTitle(lines, filePath)

	// Extract summary
	metadata.Summary = p.extractSummary(content, lines)

	// Extract status
	metadata.Status = p.extractStatus(content)

	// Determine group and category from path
	metadata.Group, metadata.Category = p.determineGroupAndCategory(filePath)

	return metadata, nil
}

// extractTitle extracts the title from markdown (first H1)
func (p *Parser) extractTitle(lines []string, filePath string) string {
	for _, line := range lines {
		if strings.HasPrefix(line, "# ") {
			title := strings.TrimSpace(strings.TrimPrefix(line, "# "))
			// Remove version suffix if present (e.g., "v1.0")
			re := regexp.MustCompile(`\s+v\d+\.\d+$`)
			title = re.ReplaceAllString(title, "")
			return title
		}
	}
	// Fallback to filename
	base := filepath.Base(filePath)
	base = strings.TrimSuffix(base, ".md")
	base = strings.ReplaceAll(base, "-", " ")
	base = strings.ReplaceAll(base, "_", " ")
	return toTitleCase(base)
}

func toTitleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) > 0 {
			r := []rune(w)
			r[0] = unicode.ToUpper(r[0])
			words[i] = string(r)
		}
	}
	return strings.Join(words, " ")
}

// extractSummary extracts a summary from markdown content
func (p *Parser) extractSummary(content string, lines []string) string {
	// Try to find Overview section
	// Go regex doesn't support lookahead, so we'll find the section and extract manually
	overviewRe := regexp.MustCompile(`## Overview\s*\n\n(.*)`)
	matches := overviewRe.FindStringSubmatch(content)
	if len(matches) > 1 {
		// Extract content up to next ## or end of string
		overviewContent := matches[1]
		nextSection := regexp.MustCompile(`\n## `)
		if nextMatch := nextSection.FindStringIndex(overviewContent); nextMatch != nil {
			overviewContent = overviewContent[:nextMatch[0]]
		}
		matches[1] = overviewContent
	}
	if len(matches) > 1 {
		summary := matches[1]
		summary = p.cleanMarkdown(summary)
		summary = p.takeFirstSentence(summary)
		summary = p.truncateToMax(summary, 200)
		if summary != emptyValue {
			return summary
		}
	}

	// Try first paragraph after title
	inContent := false
	var summaryLines []string
	for _, line := range lines {
		if strings.HasPrefix(line, "#") && !inContent {
			inContent = true
			continue
		}
		if inContent && strings.TrimSpace(line) != emptyValue && !strings.HasPrefix(line, "#") {
			// Skip metadata lines
			if strings.HasPrefix(line, "**") && strings.Contains(line, ":") {
				continue
			}
			summaryLines = append(summaryLines, strings.TrimSpace(line))
			if len(strings.Join(summaryLines, " ")) > 200 {
				break
			}
		}
	}

	summary := strings.Join(summaryLines, " ")
	summary = p.cleanMarkdown(summary)
	summary = p.takeFirstSentence(summary)
	summary = p.truncateToMax(summary, 200)

	// Fallback if empty
	if summary == emptyValue || len(strings.TrimSpace(summary)) < 10 {
		base := filepath.Base(strings.Split(content, "\n")[0])
		summary = fmt.Sprintf("Documentation: %s", base)
		summary = p.truncateToMax(summary, 200)
	}

	return summary
}

// extractStatus extracts status from markdown metadata
func (p *Parser) extractStatus(content string) string {
	statusRe := regexp.MustCompile(`\*\*Status\*\*:\s*(\w+)`)
	matches := statusRe.FindStringSubmatch(content)
	if len(matches) > 1 {
		status := strings.ToLower(matches[1])
		// Map common status values
		if status == docStatusActive || status == docStatusImplemented || status == docStatusDesignComplete || status == docStatusResearchComplete {
			return docStatusActive
		}
		if status == docStatusDeprecated || status == objects.ObjectStatusSuperseded {
			return docStatusDeprecated
		}
	}
	return docStatusActive
}

// determineGroupAndCategory determines group and category from file path
func (p *Parser) determineGroupAndCategory(filePath string) (group, category string) {
	pathLower := strings.ToLower(filePath)

	if strings.Contains(pathLower, "architecture") {
		group := "architecture"
		var category string
		if strings.Contains(pathLower, "validation") {
			category = "validation"
		} else if strings.Contains(pathLower, "graph") || strings.Contains(pathLower, "graph-backend") {
			category = "graph-backend"
		} else if strings.Contains(pathLower, "storage") {
			category = "storage"
		} else if strings.Contains(pathLower, "cli") {
			category = "cli"
		} else if strings.Contains(pathLower, "migration") {
			category = "migration"
		} else if strings.Contains(pathLower, "audit") {
			category = "audit"
		} else if strings.Contains(pathLower, "check") {
			category = "check"
		} else {
			category = "architecture"
		}
		return group, category
	}

	if strings.Contains(pathLower, "onboarding") {
		return "onboarding", ""
	}

	if strings.Contains(pathLower, "marketing") || strings.Contains(pathLower, "strategic") {
		return "other", "strategic"
	}

	if strings.Contains(pathLower, "process/_internal") {
		return "process", "internal"
	}

	return "other", ""
}

// cleanMarkdown removes markdown formatting from text
func (p *Parser) cleanMarkdown(text string) string {
	// Remove bold
	text = regexp.MustCompile(`\*\*`).ReplaceAllString(text, "")
	// Remove code
	text = regexp.MustCompile("`").ReplaceAllString(text, "")
	// Remove links
	text = regexp.MustCompile(`\[([^\]]+)\]\([^\)]+\)`).ReplaceAllString(text, "$1")
	// Remove headers
	text = regexp.MustCompile(`#+`).ReplaceAllString(text, "")
	// Normalize whitespace
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}

// takeFirstSentence takes the first sentence from text
func (p *Parser) takeFirstSentence(text string) string {
	sentences := regexp.MustCompile(`[.!?]\s+`).Split(text, 2)
	if len(sentences) > 0 {
		return sentences[0]
	}
	return text
}

// truncateToMax truncates text to max length
func (p *Parser) truncateToMax(text string, maxLen int) string {
	if len(text) > maxLen {
		return text[:maxLen-3] + "..."
	}
	return text
}
