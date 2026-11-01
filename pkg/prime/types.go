package prime

import "time"

// PrimeOptions defines configuration options for brownfield priming.
type PrimeOptions struct {
	ProjectRoot string        `json:"project_root"`
	Force       bool          `json:"force"`
	Format      string        `json:"format"`
	Quiet       bool          `json:"quiet"`
	Verify      bool          `json:"verify"`
	MaxDuration time.Duration `json:"max_duration"`
}

// ASTSymbolStats captures counts of indexed AST symbols.
type ASTSymbolStats struct {
	Structs    int `json:"structs"`
	Interfaces int `json:"interfaces"`
	Functions  int `json:"functions"`
	Methods    int `json:"methods"`
	Types      int `json:"types"`
	Constants  int `json:"constants"`
	Variables  int `json:"variables"`
	Total      int `json:"total"`
}

// ASTSymbolRecord captures an individual indexed AST declaration.
type ASTSymbolRecord struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Receiver string `json:"receiver,omitempty"`
}

// ASTIndexData represents the persisted symbol cache structure.
type ASTIndexData struct {
	GeneratedAt time.Time         `json:"generated_at"`
	ProjectRoot string            `json:"project_root"`
	FileCount   int               `json:"file_count"`
	LineCount   int               `json:"line_count"`
	Stats       ASTSymbolStats    `json:"stats"`
	Symbols     []ASTSymbolRecord `json:"symbols,omitempty"`
}

// PrimeResult summarizes the results of the brownfield priming process.
type PrimeResult struct {
	ProjectRoot           string         `json:"project_root"`
	FilesScanned          int            `json:"files_scanned"`
	LinesIndexed          int            `json:"lines_indexed"`
	SymbolStats           ASTSymbolStats `json:"symbol_stats"`
	TrigramIndexPath      string         `json:"trigram_index_path"`
	ASTIndexPath          string         `json:"ast_index_path"`
	Duration              time.Duration  `json:"duration_ns"`
	DurationFormatted     string         `json:"duration"`
	TargetDurationMet     bool           `json:"target_duration_met"`
	GitUnmodifiedVerified bool           `json:"git_unmodified_verified"`
	OrphanedStateVerified bool           `json:"orphaned_state_verified"`
	Success               bool           `json:"success"`
	Message               string         `json:"message"`
}
