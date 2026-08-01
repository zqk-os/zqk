package mcp

import "time"

// Client ID generation constants
const (
	emptyValue = ""

	// DefaultClientIDPrefix is the prefix used for auto-generated client IDs
	DefaultClientIDPrefix = "client_"

	// SystemAccountID is the account ID used for system-level operations
	SystemAccountID = "account:system"

	// SystemAccountPrefix is the prefix for system accounts
	SystemAccountPrefix = "system:"
)

// Default context and profile constants
const (
	// DefaultMCPContext is the default context/profile used by MCP server
	DefaultMCPContext = "mcp"

	// DefaultAIAgentContext is the default context for AI agents
	DefaultAIAgentContext = "ai-agent"
)

// Tool prefix constants
const (
	// DefaultCLIToolPrefix is the prefix for CLI tools exposed via MCP
	DefaultCLIToolPrefix = "cli_"
)

// Async operation defaults
const (
	// DefaultMaxConcurrentOperations is the default maximum concurrent operations
	DefaultMaxConcurrentOperations = 10

	// DefaultOperationTimeout is the default timeout for async operations (e.g. tool calls).
	// Kept short so slow/hung tools fail fast and the client stops showing "Running"; set
	// Async.Timeout in MCP config for long-running operations.
	DefaultOperationTimeout = 90 * time.Second

	// MaxToolCallDuration is the maximum duration for a single tool call when the request
	// context has no deadline. Ensures tools (e.g. get_current_backlog_item) cannot hang
	// indefinitely; workflow tools use shorter internal timeouts (e.g. 20s) on top of this.
	MaxToolCallDuration = 120 * time.Second
)

// Event emitter defaults
const (
	// DefaultEventBufferSize is the default buffer size for event emitter subscribers
	DefaultEventBufferSize = 100
)

// Client metrics defaults
const (
	// DefaultLongGapThreshold is the threshold for detecting long gaps between events
	// Gaps longer than this might indicate connection issues
	DefaultLongGapThreshold = 5 * time.Minute

	// DefaultMetricsCompressionInterval is how often to compress metrics
	DefaultMetricsCompressionInterval = 24 * time.Hour

	// DefaultMetricsRetentionPeriod is how long to retain compressed metrics
	DefaultMetricsRetentionPeriod = 7 * 24 * time.Hour
)

// Graph operation defaults
const (
	// DefaultGraphMaxDepth is the default maximum traversal depth for graph operations
	DefaultGraphMaxDepth = 3

	// DefaultGraphLimit is the default limit for graph query results
	DefaultGraphLimit = 100

	// DefaultGraphTimeoutSeconds is the default timeout in seconds for graph operations
	DefaultGraphTimeoutSeconds = 5
)

// Logging defaults
const (
	// DefaultLogNotificationTimeout is the timeout for sending log notifications
	DefaultLogNotificationTimeout = 100 * time.Millisecond

	// DefaultLoggingProfile is the default logging profile for MCP server
	DefaultLoggingProfile = "mcp"
)

// Welcome Messages
const (
	// WelcomeMessageBase is the base welcome message for new clients
	WelcomeMessageBase = "Welcome! Please introduce yourself and let me know how I can help you today. I'm the MCP server, and I can help you interact with the knowledge kernel through various tools and commands."

	// WelcomeMessageSystemHealth is the system health reminder (use with fmt.Sprintf and GetToolName("system_status"))
	WelcomeMessageSystemHealth = "**Important**: Before starting, check system health using %s. System health monitoring is critical for maintaining system integrity."

	// WelcomeMessageRoleCheck is the role check reminder for fallback welcome
	WelcomeMessageRoleCheck = "**Check your role**: Use prompts/get with name='my_role' to see your current role and permissions."

	// WelcomeMessageQuickStart is the quick start guide section
	WelcomeMessageQuickStart = "You can use:\n- prompts/get with name='getting_started' to see a comprehensive guide\n- prompts/get with name='execution_context' for current execution context\n- resources/list to see available documentation (lifecycles, workflows, system health)\n- tools/list to see all available tools based on your role"

	// WelcomeMessageQuickStartFallback is the quick start guide for fallback welcome
	WelcomeMessageQuickStartFallback = "You can use:\n- prompts/get with name='getting_started' to see a comprehensive guide\n- prompts/get with name='current_role' to see your role and permissions\n- resources/list to see available documentation (lifecycles, workflows, system health)\n- tools/list to see all available tools based on your role"
)

// Temporary client ID prefix
const (
	// ConnectingClientIDPrefix is the prefix for temporary client IDs during connection
	ConnectingClientIDPrefix = "connecting_"
)

// Notification retry defaults
const (
	// DefaultNotificationMaxRetries is the maximum number of retries for sending notifications
	DefaultNotificationMaxRetries = 5
)

// Account ID pattern constants
const (
	// AccountIDPrefix is the prefix for account IDs
	AccountIDPrefix = "account:"
)

// Message queue defaults
const (
	// DefaultMessageQueueSize is the default maximum queue size per client
	DefaultMessageQueueSize = 1000

	// DefaultMessageWriteTimeout is the default timeout for writing a message
	DefaultMessageWriteTimeout = 5 * time.Second
)
