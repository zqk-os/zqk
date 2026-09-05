package cli

import (
	"fmt"
	"strings"
)

// ParagraphBuilder helps compose line-oriented output blocks without repetitive fmt.Fprintf calls.
type ParagraphBuilder struct {
	b strings.Builder
}

// NewParagraphBuilder creates a new output paragraph builder.
func NewParagraphBuilder() *ParagraphBuilder {
	return &ParagraphBuilder{}
}

// AddLine appends a full line.
func (p *ParagraphBuilder) AddLine(line string) *ParagraphBuilder {
	p.b.WriteString(line)
	p.b.WriteString("\n")
	return p
}

// AddLinef appends a formatted line.
func (p *ParagraphBuilder) AddLinef(format string, args ...any) *ParagraphBuilder {
	p.b.WriteString(fmt.Sprintf(format, args...))
	p.b.WriteString("\n")
	return p
}

// AddLineWithIndent appends an indented line.
func (p *ParagraphBuilder) AddLineWithIndent(indent int, line string) *ParagraphBuilder {
	if indent > 0 {
		p.b.WriteString(strings.Repeat(" ", indent))
	}
	p.b.WriteString(line)
	p.b.WriteString("\n")
	return p
}

// AddLineWithIndentf appends an indented, formatted line.
func (p *ParagraphBuilder) AddLineWithIndentf(indent int, format string, args ...any) *ParagraphBuilder {
	if indent > 0 {
		p.b.WriteString(strings.Repeat(" ", indent))
	}
	p.b.WriteString(fmt.Sprintf(format, args...))
	p.b.WriteString("\n")
	return p
}

// Add appends raw content without adding a newline.
func (p *ParagraphBuilder) Add(content string) *ParagraphBuilder {
	p.b.WriteString(content)
	return p
}

// AddIf appends raw content only when cond is true.
func (p *ParagraphBuilder) AddIf(cond bool, content string) *ParagraphBuilder {
	if cond {
		p.b.WriteString(content)
	}
	return p
}

// BlankLine appends a single blank line.
func (p *ParagraphBuilder) BlankLine() *ParagraphBuilder {
	p.b.WriteString("\n")
	return p
}

// Build returns the final output string.
func (p *ParagraphBuilder) Build() string {
	return p.b.String()
}
