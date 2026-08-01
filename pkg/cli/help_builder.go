package cli

import (
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/lanceman/zqk/pkg/cliexamples"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/termfd"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/term"
)

const (
	defaultTerminalWidth = 80
	minTerminalWidth     = 40
)

// HelpBuilder provides a fluent builder pattern for creating consistent command help text
// This standardizes help menu structure across all commands and auto-discovers flags/subcommands
type HelpBuilder struct {
	short       string
	description string
	examples    []HelpExample
	sections    []HelpSection

	// Dynamic discovery options
	autoDiscoverFlags       bool
	autoDiscoverSubcommands bool
	includeCommonFlags      bool                // Whether to include common flags in flag docs
	flagCategories          map[string][]string // Category -> flag names
	excludeFlags            map[string]bool     // Flags to exclude from auto-discovery

	// Spec-driven example generation
	enableSpecExamples bool   // Enable spec-driven examples for object commands
	specExampleKind    string // Kind to generate examples for (if applicable)
	exampleGenerator   *cliexamples.Generator

	// Terminal width for text wrapping
	terminalWidth int // Terminal width (0 = auto-detect)
}

// HelpExample represents a single example with optional comment
type HelpExample struct {
	Comment string // Optional comment explaining the example (e.g., "# List all backlog items")
	Command string // The example command
}

// HelpSection represents an additional section in the help text
type HelpSection struct {
	Title   string
	Content string
}

// NewHelpBuilder creates a new help builder
func NewHelpBuilder() *HelpBuilder {
	return &HelpBuilder{
		examples:          make([]HelpExample, 0),
		sections:          make([]HelpSection, 0),
		flagCategories:    make(map[string][]string),
		excludeFlags:      make(map[string]bool),
		autoDiscoverFlags: true, // Default to auto-discovering flags
	}
}

// WithShort sets the short description
func (b *HelpBuilder) WithShort(short string) *HelpBuilder {
	b.short = short
	return b
}

// WithDescription sets the main description
// Can be called multiple times - each call appends to the description
func (b *HelpBuilder) WithDescription(description string) *HelpBuilder {
	if b.description != emptyValue {
		b.description += "\n\n" + description
	} else {
		b.description = description
	}
	return b
}

// WithDescriptionLines sets the description from multiple lines
// Each string becomes a line, joined with newlines
func (b *HelpBuilder) WithDescriptionLines(lines ...string) *HelpBuilder {
	description := strings.Join(lines, "\n")
	return b.WithDescription(description)
}

// AddDescriptionLine adds a single line to the description
// Automatically adds newline if description already exists
func (b *HelpBuilder) AddDescriptionLine(line string) *HelpBuilder {
	if b.description != emptyValue {
		b.description += "\n" + line
	} else {
		b.description = line
	}
	return b
}

// AddDescriptionParagraph adds a paragraph to the description
// Automatically adds blank line before if description already exists
func (b *HelpBuilder) AddDescriptionParagraph(paragraph string) *HelpBuilder {
	if b.description != emptyValue {
		b.description += "\n\n" + paragraph
	} else {
		b.description = paragraph
	}
	return b
}

// AddExample adds an example with optional comment
func (b *HelpBuilder) AddExample(comment, command string) *HelpBuilder {
	b.examples = append(b.examples, HelpExample{
		Comment: comment,
		Command: command,
	})
	return b
}

// AddSection adds an additional section (e.g., "Notes:", "See Also:")
func (b *HelpBuilder) AddSection(title, content string) *HelpBuilder {
	b.sections = append(b.sections, HelpSection{
		Title:   title,
		Content: content,
	})
	return b
}

// WithAutoDiscoverFlags enables/disables automatic flag discovery from command
func (b *HelpBuilder) WithAutoDiscoverFlags(enabled bool) *HelpBuilder {
	b.autoDiscoverFlags = enabled
	return b
}

// WithAutoDiscoverSubcommands enables/disables automatic subcommand discovery
func (b *HelpBuilder) WithAutoDiscoverSubcommands(enabled bool) *HelpBuilder {
	b.autoDiscoverSubcommands = enabled
	return b
}

// WithIncludeCommonFlags controls whether common flags are included in flag documentation
func (b *HelpBuilder) WithIncludeCommonFlags(include bool) *HelpBuilder {
	b.includeCommonFlags = include
	return b
}

// ExcludeFlag excludes a flag from auto-discovery documentation
func (b *HelpBuilder) ExcludeFlag(flagName string) *HelpBuilder {
	b.excludeFlags[flagName] = true
	return b
}

// ExcludeFlags excludes multiple flags from auto-discovery documentation
func (b *HelpBuilder) ExcludeFlags(flagNames ...string) *HelpBuilder {
	for _, name := range flagNames {
		b.excludeFlags[name] = true
	}
	return b
}

// ExcludeCommonFlags excludes the canonical common-flag set from help output.
func (b *HelpBuilder) ExcludeCommonFlags() *HelpBuilder {
	return b.ExcludeFlags(defaultHelpExcludedCommonFlags...)
}

// ExcludeCommonFlagsWithout excludes canonical common flags minus one flag name.
func (b *HelpBuilder) ExcludeCommonFlagsWithout(flagName string) *HelpBuilder {
	return b.ExcludeFlags(CommonExcludedFlagsWithout(flagName)...)
}

// CategorizeFlag adds a flag to a category for organized flag documentation
func (b *HelpBuilder) CategorizeFlag(category, flagName string) *HelpBuilder {
	b.flagCategories[category] = append(b.flagCategories[category], flagName)
	return b
}

// WithSpecExamples enables spec-driven example generation for object commands
// kind should be the object kind (e.g., "backlog_item", "goal")
func (b *HelpBuilder) WithSpecExamples(kind string) *HelpBuilder {
	b.enableSpecExamples = true
	b.specExampleKind = kind
	return b
}

// WithExampleGenerator sets a custom example generator
func (b *HelpBuilder) WithExampleGenerator(gen *cliexamples.Generator) *HelpBuilder {
	b.exampleGenerator = gen
	return b
}

// WithTerminalWidth sets a specific terminal width (0 = auto-detect)
func (b *HelpBuilder) WithTerminalWidth(width int) *HelpBuilder {
	b.terminalWidth = width
	return b
}

// Build builds the complete help text (Long description)
// If cmd is provided, auto-discovers flags and subcommands
func (b *HelpBuilder) Build() string {
	return b.BuildForCommand(nil)
}

// BuildForCommand builds the complete help text for a specific command
// Auto-discovers flags and subcommands if enabled
func (b *HelpBuilder) BuildForCommand(cmd *cobra.Command) string {
	width := b.getTerminalWidth()
	var parts []string

	// Add description (wrapped to terminal width)
	if b.description != emptyValue {
		wrappedDesc := b.wrapText(b.description, width, "")
		parts = append(parts, wrappedDesc)
		parts = append(parts, "") // Blank line
	}

	// Auto-discover subcommands if enabled
	if b.autoDiscoverSubcommands && cmd != nil {
		subcommands := b.discoverSubcommands(cmd)
		if len(subcommands) > 0 {
			parts = append(parts, "Available Subcommands:")
			for _, subcmd := range subcommands {
				// Format subcommand line with wrapping for long descriptions
				subcmdPrefix := fmt.Sprintf("  %-15s", subcmd.Use)
				if subcmd.Short != emptyValue {
					// Calculate available width for description (account for prefix + spacing)
					descIndent := strings.Repeat(" ", len(subcmdPrefix)+2)
					descWidth := width - len(descIndent)
					if descWidth < 20 {
						descWidth = 20
					}

					// Wrap the short description
					wrappedShort := b.wrapText(subcmd.Short, descWidth+len(descIndent), descIndent)
					lines := make([]string, 0)
					for line := range strings.SplitSeq(wrappedShort, "\n") {
						lines = append(lines, line)
					}

					// First line includes the subcommand name
					if len(lines) > 0 {
						parts = append(parts, subcmdPrefix+"  "+strings.TrimPrefix(lines[0], descIndent))
						// Subsequent lines are already properly indented
						for i := 1; i < len(lines); i++ {
							parts = append(parts, lines[i])
						}
					}
				} else {
					parts = append(parts, subcmdPrefix)
				}
			}
			parts = append(parts, "") // Blank line
		}
	}

	// Auto-discover flags if enabled
	if b.autoDiscoverFlags && cmd != nil {
		flagDocs := b.discoverFlags(cmd)
		if flagDocs != emptyValue {
			parts = append(parts, flagDocs)
			parts = append(parts, "") // Blank line
		}
	}

	// Add examples section if any examples exist
	hasExamples := len(b.examples) > 0

	// Generate spec-driven examples if enabled
	if b.enableSpecExamples && b.specExampleKind != emptyValue {
		specExamples := b.generateSpecExamples()
		if len(specExamples) > 0 {
			if !hasExamples {
				parts = append(parts, "Examples:")
			}
			parts = append(parts, specExamples...)
			hasExamples = true
		}
	}

	// Add manual examples
	if len(b.examples) > 0 {
		if !hasExamples {
			parts = append(parts, "Examples:")
		}
		for _, ex := range b.examples {
			if ex.Comment != emptyValue {
				parts = append(parts, fmt.Sprintf("  %s", ex.Comment))
			}
			// Replace placeholder with actual command name
			command := strings.ReplaceAll(ex.Command, "%s", paths.CLICommandName)
			parts = append(parts, fmt.Sprintf("  %s", command))
		}
		hasExamples = true
	}

	if hasExamples {
		parts = append(parts, "") // Blank line after examples
	}

	// Add additional sections (wrapped to terminal width)
	for _, section := range b.sections {
		parts = append(parts, fmt.Sprintf("%s:", section.Title))
		wrappedContent := b.wrapText(section.Content, width, "  ")
		parts = append(parts, wrappedContent)
		parts = append(parts, "") // Blank line after section
	}

	// Join all parts
	result := strings.Join(parts, "\n")
	// Remove trailing newlines
	result = strings.TrimRight(result, "\n")
	return result
}

// discoverSubcommands discovers and returns visible subcommands
func (b *HelpBuilder) discoverSubcommands(cmd *cobra.Command) []*cobra.Command {
	var subcommands []*cobra.Command
	for _, subcmd := range cmd.Commands() {
		if !subcmd.Hidden {
			subcommands = append(subcommands, subcmd)
		}
	}
	// Sort by Use for consistent output
	sort.Slice(subcommands, func(i, j int) bool {
		return subcommands[i].Use < subcommands[j].Use
	})
	return subcommands
}

// discoverFlags discovers flags and generates documentation
func (b *HelpBuilder) discoverFlags(cmd *cobra.Command) string {
	var flags []flagInfo

	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		// Skip hidden flags
		if flag.Hidden {
			return
		}

		// Skip excluded flags
		if b.excludeFlags[flag.Name] {
			return
		}

		// Skip common flags if not including them
		if !b.includeCommonFlags && isCommonFlag(flag.Name) {
			return
		}

		flags = append(flags, flagInfo{
			Name:      flag.Name,
			Shorthand: flag.Shorthand,
			Type:      flag.Value.Type(),
			Default:   flag.DefValue,
			Usage:     flag.Usage,
			Category:  b.getFlagCategory(flag.Name),
		})
	})

	if len(flags) == 0 {
		return ""
	}

	// Sort flags by category, then by name
	sort.Slice(flags, func(i, j int) bool {
		if flags[i].Category != flags[j].Category {
			return flags[i].Category < flags[j].Category
		}
		return flags[i].Name < flags[j].Name
	})

	var parts []string
	parts = append(parts, "Flags:")

	currentCategory := ""
	for _, flag := range flags {
		// Add category header if needed
		if flag.Category != emptyValue && flag.Category != currentCategory {
			if currentCategory != emptyValue {
				parts = append(parts, "") // Blank line between categories
			}
			parts = append(parts, fmt.Sprintf("  %s:", flag.Category))
			currentCategory = flag.Category
		}

		// Build flag line
		flagLine := "    "
		if flag.Shorthand != emptyValue {
			flagLine += fmt.Sprintf("-%s, ", flag.Shorthand)
		}
		flagLine += fmt.Sprintf("--%s", flag.Name)

		// Add type info if not boolean
		if flag.Type != "bool" {
			flagLine += fmt.Sprintf(" <%s>", flag.Type)
		}

		// Add default value if present
		if flag.Default != emptyValue && flag.Default != "false" {
			flagLine += fmt.Sprintf(" (default: %s)", flag.Default)
		}

		parts = append(parts, flagLine)

		// Add usage description (wrapped to terminal width)
		if flag.Usage != emptyValue {
			width := b.getTerminalWidth()
			// Account for the "      " indent (6 chars) when wrapping
			wrappedUsage := b.wrapText(flag.Usage, width, "      ")
			parts = append(parts, wrappedUsage)
		}
	}

	return strings.Join(parts, "\n")
}

// getFlagCategory returns the category for a flag
func (b *HelpBuilder) getFlagCategory(flagName string) string {
	for category, flags := range b.flagCategories {
		if slices.Contains(flags, flagName) {
			return category
		}
	}
	return "" // Uncategorized
}

// isCommonFlag checks if a flag is a common flag
// Common flags are those typically added via AddCommonFlags()
func isCommonFlag(flagName string) bool {
	return slices.Contains(defaultHelpExcludedCommonFlags, flagName)
}

// flagInfo represents information about a command flag
type flagInfo struct {
	Name      string
	Shorthand string
	Type      string
	Default   string
	Usage     string
	Category  string
}

// ApplyToCommand applies the help builder to a cobra command
// Auto-discovers flags and subcommands if enabled
func (b *HelpBuilder) ApplyToCommand(cmd *cobra.Command) {
	if b.short != emptyValue {
		cmd.Short = b.short
	}
	cmd.Long = b.BuildForCommand(cmd)

	// If auto-discovering subcommands, also set up dynamic help for subcommands
	if b.autoDiscoverSubcommands {
		b.setupDynamicSubcommandHelp(cmd)
	}
}

// generateSpecExamples generates spec-driven examples using the cliexamples generator
func (b *HelpBuilder) generateSpecExamples() []string {
	// Get or create example generator
	gen := b.exampleGenerator
	if gen == nil {
		var err error
		gen, err = cliexamples.New()
		if err != nil {
			return nil // Fail silently if generator can't be created
		}
	}

	// Generate examples for the kind
	examples, err := gen.GenerateCLICommandExamples(b.specExampleKind)
	if err != nil || len(examples) == 0 {
		return nil
	}

	// Format examples with proper indentation
	var formatted []string
	for _, line := range examples {
		if strings.TrimSpace(line) == emptyValue {
			formatted = append(formatted, "")
		} else {
			// Ensure proper indentation (2 spaces for examples)
			if !strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "#") {
				formatted = append(formatted, "  "+line)
			} else {
				formatted = append(formatted, line)
			}
		}
	}

	return formatted
}

// getTerminalWidth gets the terminal width, using cached value or auto-detecting
func (b *HelpBuilder) getTerminalWidth() int {
	if b.terminalWidth > 0 {
		return b.terminalWidth
	}

	// Try to detect terminal width
	width := defaultTerminalWidth // Default fallback
	if w, _, err := term.GetSize(termfd.Int(os.Stdout)); err == nil && w > 0 {
		width = w
	} else if w, _, err := term.GetSize(termfd.Int(os.Stderr)); err == nil && w > 0 {
		width = w
	} else if w, _, err := term.GetSize(0); err == nil && w > 0 {
		width = w
	}

	// Ensure minimum width
	if width < minTerminalWidth {
		width = minTerminalWidth
	}

	// Cache the detected width
	b.terminalWidth = width
	return width
}

// wrapText wraps text to the specified width, respecting the indent
func (b *HelpBuilder) wrapText(text string, width int, indent string) string {
	if width <= 0 {
		width = b.getTerminalWidth()
	}

	// Account for indent in width calculation
	effectiveWidth := width - len(indent)
	if effectiveWidth < 20 {
		effectiveWidth = 20 // Minimum width
	}

	// Split into paragraphs (double newlines)
	var wrappedParagraphs []string

	for para := range strings.SplitSeq(text, "\n\n") {
		// Preserve single newlines within paragraphs (e.g., in code blocks)
		if strings.Contains(para, "\n") && !strings.HasPrefix(strings.TrimSpace(para), "```") {
			// This might be a code block or preformatted text, preserve as-is
			wrappedParagraphs = append(wrappedParagraphs, indent+para)
			continue
		}

		words := strings.Fields(para)
		if len(words) == 0 {
			wrappedParagraphs = append(wrappedParagraphs, "")
			continue
		}

		var lines []string
		currentLine := words[0]

		for _, word := range words[1:] {
			// Check if adding this word would exceed the width
			if len(currentLine)+1+len(word) > effectiveWidth {
				lines = append(lines, indent+currentLine)
				currentLine = word
			} else {
				currentLine += " " + word
			}
		}

		// Add the last line
		if currentLine != emptyValue {
			lines = append(lines, indent+currentLine)
		}

		wrappedParagraphs = append(wrappedParagraphs, strings.Join(lines, "\n"))
	}

	return strings.Join(wrappedParagraphs, "\n\n")
}

// setupDynamicSubcommandHelp sets up help augmentation for subcommands
// This allows subcommands to show spec-driven examples when appropriate
func (b *HelpBuilder) setupDynamicSubcommandHelp(cmd *cobra.Command) {
	// This is a placeholder for future enhancement
	// Could integrate with cliexamples.Generator here for spec-driven examples
}

// StandardHelpBuilder is a convenience function that creates a help builder with common patterns
func StandardHelpBuilder(short string, descriptionLines ...string) *HelpBuilder {
	builder := NewHelpBuilder().WithShort(short)
	if len(descriptionLines) > 0 {
		builder.WithDescriptionLines(descriptionLines...)
	}
	return builder
}

// DynamicHelpBuilder creates a help builder with all dynamic features enabled
// This is the recommended way to create help text that automatically reflects command structure
// Description can be provided as multiple lines (variadic) - each becomes a line
func DynamicHelpBuilder(short string, descriptionLines ...string) *HelpBuilder {
	builder := NewHelpBuilder().
		WithShort(short).
		WithAutoDiscoverFlags(true).
		WithAutoDiscoverSubcommands(true).
		WithIncludeCommonFlags(false) // Exclude common flags by default (they're shown by cobra)

	if len(descriptionLines) > 0 {
		builder.WithDescriptionLines(descriptionLines...)
	}

	return builder
}

// ObjectCommandHelpBuilder creates a help builder optimized for object commands
// Automatically includes spec-driven examples if kind is provided
// Description can be provided as multiple lines (variadic) - each becomes a line
func ObjectCommandHelpBuilder(short string, kind string, descriptionLines ...string) *HelpBuilder {
	builder := NewHelpBuilder().
		WithShort(short).
		WithAutoDiscoverFlags(true).
		WithIncludeCommonFlags(false)

	if len(descriptionLines) > 0 {
		builder.WithDescriptionLines(descriptionLines...)
	}

	// Enable spec-driven examples if kind is provided
	if kind != emptyValue {
		builder.WithSpecExamples(kind)
	}

	return builder
}
