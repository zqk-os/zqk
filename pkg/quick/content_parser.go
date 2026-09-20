// Package quick provides parsing and helpers for one-click creation of system objects from text or files.
package quick

import (
	"bufio"
	"strings"
)

const (
	markdownHeadingPrefix = "#"
	newlineSeparator      = "\n"
	emptyContent          = ""
)

// ParsedContent holds title and body extracted from markdown or plain text.
type ParsedContent struct {
	Title string
	Body  string
}

// ParseMarkdownOrText extracts a title and body from markdown or plain text.
// Title: first line that starts with # (strip # and trim), or first non-empty line.
// Body: remaining content (trimmed).
func ParseMarkdownOrText(content string) ParsedContent {
	content = strings.TrimSpace(content)
	if content == emptyContent {
		return ParsedContent{}
	}
	scanner := bufio.NewScanner(strings.NewReader(content))
	var first string
	var rest []string
	for scanner.Scan() {
		line := scanner.Text()
		if first == emptyContent {
			first = strings.TrimSpace(line)
			if first == emptyContent {
				continue
			}
			// If first non-empty line is a heading, use it as title (strip #)
			if strings.HasPrefix(first, markdownHeadingPrefix) {
				first = strings.TrimSpace(strings.TrimLeft(first, markdownHeadingPrefix))
			}
			continue
		}
		rest = append(rest, line)
	}
	body := strings.TrimSpace(strings.Join(rest, newlineSeparator))
	return ParsedContent{Title: first, Body: body}
}

// ParseFile reads a file and returns ParsedContent. Callers should use os.ReadFile
// and pass string(content) to ParseMarkdownOrText if they need to handle read errors.
func ParseFileContent(content []byte) ParsedContent {
	return ParseMarkdownOrText(string(content))
}
