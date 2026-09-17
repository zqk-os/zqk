package search

import "time"

// Mode represents the search strategy mode.
type Mode string

const (
	// ModeText performs text/regex matching with optional trigram indexing.
	ModeText Mode = "text"
	// ModeAST performs Go AST structural pattern matching.
	ModeAST Mode = "ast"
)

// SearchOptions defines the parameters for a search operation.
type SearchOptions struct {
	// Query is the search query string or regex pattern.
	Query string `json:"query" yaml:"query"`
	// Path is the root directory or file to search in.
	Path string `json:"path" yaml:"path"`
	// Mode is the search mode: "text" (default) or "ast".
	Mode Mode `json:"mode" yaml:"mode"`
	// CaseInsensitive indicates whether search should ignore case.
	CaseInsensitive bool `json:"case_insensitive" yaml:"case_insensitive"`
	// Regex indicates whether Query is a regular expression.
	Regex bool `json:"regex" yaml:"regex"`
	// WordMatch requires matching whole words only.
	WordMatch bool `json:"word_match" yaml:"word_match"`

	// ASTKind filters AST search by declaration kind ("func", "type", "struct", "interface", "var", "const").
	ASTKind string `json:"ast_kind,omitempty" yaml:"ast_kind,omitempty"`
	// ASTReceiver filters AST method search by receiver type name (e.g. "Engine").
	ASTReceiver string `json:"ast_receiver,omitempty" yaml:"ast_receiver,omitempty"`

	// FileExtensions restricts search to files with matching extensions (e.g. []string{".go"}).
	FileExtensions []string `json:"file_extensions,omitempty" yaml:"file_extensions,omitempty"`
	// ExcludePatterns defines glob patterns or directory names to skip.
	ExcludePatterns []string `json:"exclude_patterns,omitempty" yaml:"exclude_patterns,omitempty"`

	// MaxMatches caps the total number of matches returned (default 100).
	MaxMatches int `json:"max_matches,omitempty" yaml:"max_matches,omitempty"`
	// MaxTokens caps output to prevent context window explosion (default 4000). 0 means unconstrained.
	MaxTokens int `json:"max_tokens,omitempty" yaml:"max_tokens,omitempty"`
	// ContextLines is the number of surrounding lines to include before and after matches.
	ContextLines int `json:"context_lines,omitempty" yaml:"context_lines,omitempty"`

	// IncludeHidden includes hidden files and dot-directories.
	IncludeHidden bool `json:"include_hidden,omitempty" yaml:"include_hidden,omitempty"`
	// UseIndex indicates whether to use or build a trigram index when available.
	UseIndex bool `json:"use_index,omitempty" yaml:"use_index,omitempty"`
}

// Match represents a single search match.
type Match struct {
	// File is the relative or normalized file path.
	File string `json:"file" yaml:"file"`
	// Line is the 1-based starting line number.
	Line int `json:"line" yaml:"line"`
	// Column is the 1-based column offset.
	Column int `json:"column,omitempty" yaml:"column,omitempty"`
	// EndLine is the 1-based ending line number (for AST multi-line nodes).
	EndLine int `json:"end_line,omitempty" yaml:"end_line,omitempty"`
	// LineContent is the text content of the matching line.
	LineContent string `json:"line_content" yaml:"line_content"`
	// ContextBefore contains context lines preceding the match.
	ContextBefore []string `json:"context_before,omitempty" yaml:"context_before,omitempty"`
	// ContextAfter contains context lines following the match.
	ContextAfter []string `json:"context_after,omitempty" yaml:"context_after,omitempty"`

	// SymbolKind indicates the AST declaration kind ("func", "struct", "interface", etc.) if in AST mode.
	SymbolKind string `json:"symbol_kind,omitempty" yaml:"symbol_kind,omitempty"`
	// SymbolName is the identifier name if matched in AST mode.
	SymbolName string `json:"symbol_name,omitempty" yaml:"symbol_name,omitempty"`
	// Receiver is the method receiver type name if matched in AST mode.
	Receiver string `json:"receiver,omitempty" yaml:"receiver,omitempty"`
}

// SearchResult aggregates matches and performance metadata.
type SearchResult struct {
	// Matches is the list of matches found within limits.
	Matches []Match `json:"matches" yaml:"matches"`
	// TotalMatches is the total count of matches found before truncation.
	TotalMatches int `json:"total_matches" yaml:"total_matches"`
	// FilesSearched is the total number of files scanned.
	FilesSearched int `json:"files_searched" yaml:"files_searched"`
	// Duration is the total execution time.
	Duration time.Duration `json:"duration_ns" yaml:"duration_ns"`
	// DurationMs is execution time in milliseconds.
	DurationMs float64 `json:"duration_ms" yaml:"duration_ms"`
	// EstimatedTokens is the estimated token count of the output payload.
	EstimatedTokens int `json:"estimated_tokens" yaml:"estimated_tokens"`
	// Truncated indicates whether results were capped by MaxMatches or MaxTokens.
	Truncated bool `json:"truncated" yaml:"truncated"`
	// TruncateReason indicates why results were truncated ("max_matches", "max_tokens", "").
	TruncateReason string `json:"truncate_reason,omitempty" yaml:"truncate_reason,omitempty"`
}
