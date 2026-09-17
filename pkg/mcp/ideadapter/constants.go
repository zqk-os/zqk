package ideadapter

// JSON-RPC / MCP method and wire keys used by the IDE adapter.
const (
	rpcKeyJSONRPC = "jsonrpc"
	rpcKeyMethod  = "method"
	rpcKeyParams  = "params"
	rpcKeyResult  = "result"
	rpcKeyError   = "error"
	rpcKeyMessage = "message"
	rpcKeyCode    = "code"

	methodInitialize              = "initialize"
	methodInitializedNotification = "notifications/initialized"
	methodPing                    = "ping"
	methodToolsList               = "tools/list"
	methodToolsCall               = "tools/call"
	methodPromptsList             = "prompts/list"
	methodPromptsGet              = "prompts/get"
	methodResourcesList           = "resources/list"
	methodResourcesRead           = "resources/read"
	methodResourcesTemplatesList  = "resources/templates/list"
	methodCompletionComplete      = "completion/complete"
	methodLoggingSetLevel         = "logging/setLevel"
	methodEventsSubscribe         = "events/subscribe"

	// keepaliveIDPrefix marks server→IDE ping ids that flip the host hourglass.
	// Observed: IDE closes stdio ~120s when only daemon pings run (host-invisible).
	// TRACK: BLI-REDACTED — retire when Streamable HTTP is default
	// or IDE no longer requires host-visible keepalive.
	keepaliveIDPrefix = "zqk-ka-"

	wireProtocolVersion = "protocolVersion"
	wireClientInfo      = "clientInfo"
	wireServerInfo      = "serverInfo"
	wireTools           = "tools"
	wirePrompts         = "prompts"
	wireResources       = "resources"
	wireLogging         = "logging"
	wireRoots           = "roots"
	wireElicitation     = "elicitation"
	wireLevel           = "level"
	wireEventTypes      = "eventTypes"
	wireClientID        = "clientId"

	defaultProtocolVersion   = "2024-11-05"
	defaultServerVersion     = "0.1.0"
	defaultClientInfoVer     = "ide-adapter"
	defaultHeartbeatRaw      = "5s"
	defaultRequestTimeout    = "30s"
	defaultStdioKeepaliveRaw = "45s"

	errDaemonUnavailable = -32001
)
