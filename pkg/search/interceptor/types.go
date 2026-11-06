package interceptor

import "context"

// Canonical pipeline and stage constants.
const (
	PipelineKindSearchInterceptor = "search_command_interceptor"

	StageFunnelFilter         = "funnel_filter"
	StageStreamFilterDetector = "stream_filter_detector"
	StagePathBoundaryDetector = "path_boundary_detector"
	StageIntentClassifier     = "intent_classifier"
	StageRewriteTransformer   = "rewrite_transformer"
)

// Supported search tool names.
const (
	ToolGrep    = "grep"
	ToolEGrep   = "egrep"
	ToolFGrep   = "fgrep"
	ToolRG      = "rg"
	ToolRipgrep = "ripgrep"
	ToolGitGrep = "git grep"
)

// Reason constants for interception decisions.
const (
	ReasonSearchIntercepted   = "Intercepted search: redirected to token-budgeted zqk grep trigram engine"
	ReasonRewritten           = ReasonSearchIntercepted
	ReasonNotTargetSearchTool = "command does not invoke a supported search tool"
	ReasonStreamFilter        = "standard input pipe detected (stream filter)"
	ReasonExternalPath        = "target path is outside repository root"
	ReasonNonCodeFile         = "target path is a single non-code file"
	ReasonEmptyCommand        = "empty command line"
)

// Classification constants for search queries.
const (
	ClassificationAST    = "ast"
	ClassificationText   = "text"
	ClassificationBypass = "bypass"
)

// Pipeline context outcome keys.
const (
	OutcomeKeyIntercepted      = "search.intercepted"
	OutcomeKeyRewrittenCommand = "search.rewritten_command"
	OutcomeKeyReason           = "search.reason"
	OutcomeKeyClassification   = "search.classification"
)

// Default execution parameters.
const (
	DefaultCLICommand = "zqk"
	DefaultMaxTokens  = 4000
	GrepSubcommand    = "grep"
	DotPath           = "."
	TildePrefix       = "~/"
	TildeOnly         = "~"
)

// InterceptRequest encapsulates an incoming command execution candidate.
type InterceptRequest struct {
	CommandLine string            `json:"command_line" yaml:"command_line"`
	Cwd         string            `json:"cwd" yaml:"cwd"`
	ProjectRoot string            `json:"project_root" yaml:"project_root"`
	Env         map[string]string `json:"env" yaml:"env"`
}

// Request is a compatibility alias for InterceptRequest.
type Request = InterceptRequest

// InterceptResult describes the decision, classification, and rewritten command.
type InterceptResult struct {
	Intercepted      bool   `json:"intercepted" yaml:"intercepted"`
	RewrittenCommand string `json:"rewritten_command" yaml:"rewritten_command"`
	Reason           string `json:"reason" yaml:"reason"`
	Classification   string `json:"classification" yaml:"classification"`

	// Backward-compatibility aliases for agent hooks
	Rewritten   bool   `json:"rewritten,omitempty" yaml:"rewritten,omitempty"`
	Replacement string `json:"replacement,omitempty" yaml:"replacement,omitempty"`
}

// Result is a compatibility alias for InterceptResult.
type Result = InterceptResult

// Interceptor provides an interface for inspecting and transforming tool commands.
type Interceptor interface {
	Intercept(ctx context.Context, req InterceptRequest) (*InterceptResult, error)
}

// Options configures the search interceptor behavior.
type Options struct {
	BinaryName string
	MaxTokens  int
}

// Option modifies Options.
type Option func(*Options)

// WithBinaryName configures the binary name used for rewritten commands (e.g. "./bin/zqk").
func WithBinaryName(name string) Option {
	return func(o *Options) {
		o.BinaryName = name
	}
}

// WithMaxTokens configures an explicit token budget flag for the rewritten command.
func WithMaxTokens(tokens int) Option {
	return func(o *Options) {
		o.MaxTokens = tokens
	}
}
