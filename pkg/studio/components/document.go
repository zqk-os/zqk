package components

import (
	"fmt"
	"strings"
)

// DocumentComponent dynamically renders the complete outer HTML document.
type DocumentComponent struct {
	Title      string
	StyleSheet string
	Header     Component
	Views      []Component
	Sidebar    Component
	Scripts    string
}

// NewDefaultDocument creates a new DocumentComponent.
func NewDefaultDocument(title string) *DocumentComponent {
	return &DocumentComponent{
		Title: title,
	}
}

// WriteHTMLHead writes standard metadata tags and title to the builder.
func WriteHTMLHead(sb *strings.Builder, title, styleSheet string) {
	sb.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n")
	sb.WriteString("  <meta charset=\"UTF-8\">\n")
	sb.WriteString("  <meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\">\n")
	sb.WriteString("  <meta http-equiv=\"Cache-Control\" content=\"no-cache, no-store, must-revalidate\">\n")
	sb.WriteString("  <meta http-equiv=\"Pragma\" content=\"no-cache\">\n")
	sb.WriteString("  <meta http-equiv=\"Expires\" content=\"0\">\n")
	fmt.Fprintf(sb, "  <title>%s</title>\n", title)

	if styleSheet != "" {
		sb.WriteString("  <style>\n")
		sb.WriteString(styleSheet)
		if !strings.HasSuffix(styleSheet, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("  </style>\n")
	}
}

// RenderHTML generates the complete HTML document string.
func (d *DocumentComponent) RenderHTML() string {
	var sb strings.Builder
	WriteHTMLHead(&sb, d.Title, d.StyleSheet)
	sb.WriteString("</head>\n")
	sb.WriteString("<body>\n")

	if d.Header != nil {
		sb.WriteString(d.Header.RenderHTML())
	}

	sb.WriteString("  <main>\n")
	for _, view := range d.Views {
		if view != nil {
			sb.WriteString(view.RenderHTML())
		}
	}
	if d.Sidebar != nil {
		sb.WriteString(d.Sidebar.RenderHTML())
	}
	sb.WriteString("  </main>\n\n")

	if d.Scripts != "" {
		sb.WriteString("  <script>\n")
		sb.WriteString(d.Scripts)
		if !strings.HasSuffix(d.Scripts, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("  </script>\n")
	}

	sb.WriteString("</body>\n")
	sb.WriteString("</html>\n")

	return sb.String()
}
