package logging

// Common structured-log wire keys (aligned with ontology FieldKey* where names match).
// Kept as package-local literals to avoid importing pkg/objects (import cycle).
// Grouped by intent; alphabetical within each group.
const (
	// Identity / IDs
	logFieldKeyAgentID    = "agent_id"
	logFieldKeyChannel    = "channel"
	logFieldKeyEngineID   = "engineID"
	logFieldKeyEnvelopeID = "envelope_id"
	logFieldKeyEventID    = "event_id"
	logFieldKeyFeedID     = "feed_id"
	logFieldKeyID         = "id"
	logFieldKeyItemID     = "item_id"
	logFieldKeyJobID      = "job_id"
	logFieldKeyObjectID   = "object_id"
	logFieldKeyParentID   = "parent_id"
	logFieldKeyPlanID     = "plan_id"
	logFieldKeySessionID  = "session_id"
	logFieldKeySkillID    = "skill_id"
	logFieldKeyTaskID     = "task_id"
	logFieldKeyToolCallID = "toolCallID"

	// Actor / seating
	logFieldKeyDeliveryMode = "delivery_mode"
	logFieldKeyPersonaRef   = "persona_ref"
	logFieldKeyPersonaTitle = "persona_title"
	logFieldKeyRole         = "role"

	// Lifecycle / status
	logFieldKeyAction     = "action"
	logFieldKeyChangeType = "changeType"
	logFieldKeyCode       = "code"
	logFieldKeyEvent      = "event"
	logFieldKeyStage      = "stage"
	logFieldKeyStatus     = "status"

	// Time / quantity
	logFieldKeyActiveInstances = "active_instances"
	logFieldKeyCount           = "count"
	logFieldKeyDuration        = "duration"
	logFieldKeyInterval        = "interval"
	logFieldKeyLimit           = "limit"
	logFieldKeyPeriod          = "period"
	logFieldKeyPRNumber        = "pr_number"
	logFieldKeyScene           = "scene"

	// Location / IO
	logFieldKeyAddr        = "addr"
	logFieldKeyDest        = "dest"
	logFieldKeyFile        = "file"
	logFieldKeyFormat      = "format"
	logFieldKeyOutput      = "output"
	logFieldKeyPath        = "path"
	logFieldKeyProjectRoot = "project_root"
	logFieldKeyScript      = "script"

	// Transport / protocol
	logFieldKeyHandler  = "handler"
	logFieldKeyMethod   = "method"
	logFieldKeyProtocol = "protocol"

	// Content / tooling
	logFieldKeyArguments      = "arguments"
	logFieldKeyError          = "error"
	logFieldKeyIDE            = "ide"
	logFieldKeyKind           = "kind"
	logFieldKeyNormalizedName = "normalizedName"
	logFieldKeyNote           = "note"
	logFieldKeyPrompt         = "prompt"
	logFieldKeyToolName       = "toolName"
	logFieldKeyVersion        = "version"
)

// --- Field constructors for variadic Logger.* calls (same wire keys as fluentEntry helpers) ---
// Order mirrors the const groups above.

// Identity / IDs
func AgentIDField(id string) Field    { return String(logFieldKeyAgentID, id) }
func ChannelField(ch string) Field    { return String(logFieldKeyChannel, ch) }
func EngineIDField(id string) Field   { return String(logFieldKeyEngineID, id) }
func EnvelopeIDField(id string) Field { return String(logFieldKeyEnvelopeID, id) }
func EventIDField(id string) Field    { return String(logFieldKeyEventID, id) }
func FeedIDField(id string) Field     { return String(logFieldKeyFeedID, id) }
func IDField(id string) Field         { return String(logFieldKeyID, id) }
func ItemIDField(id string) Field     { return String(logFieldKeyItemID, id) }
func JobIDField(id string) Field      { return String(logFieldKeyJobID, id) }
func ObjectIDField(id string) Field   { return String(logFieldKeyObjectID, id) }
func ParentIDField(id string) Field   { return String(logFieldKeyParentID, id) }
func PlanIDField(id string) Field     { return String(logFieldKeyPlanID, id) }
func SessionIDField(id string) Field  { return String(logFieldKeySessionID, id) }
func SkillIDField(id string) Field    { return String(logFieldKeySkillID, id) }
func TaskIDField(id string) Field     { return String(logFieldKeyTaskID, id) }
func ToolCallIDField(id string) Field { return String(logFieldKeyToolCallID, id) }

// Actor / seating
func DeliveryModeField(m string) Field { return String(logFieldKeyDeliveryMode, m) }
func PersonaRefField(id string) Field  { return String(logFieldKeyPersonaRef, id) }
func RoleField(role string) Field      { return String(logFieldKeyRole, role) }

// Lifecycle / status
func CodeField(n int) Field           { return Int(logFieldKeyCode, n) }
func EventField(event string) Field   { return String(logFieldKeyEvent, event) }
func StageField(stage string) Field   { return String(logFieldKeyStage, stage) }
func StatusField(status string) Field { return String(logFieldKeyStatus, status) }

// StatusIntField logs an HTTP (or similar) numeric status under wire key "status".
func StatusIntField(n int) Field { return Int(logFieldKeyStatus, n) }

// Time / quantity
func ActiveInstancesField(n int) Field { return Int(logFieldKeyActiveInstances, n) }
func CountField(n int) Field           { return Int(logFieldKeyCount, n) }
func ElapsedStringField(s string) Field {
	return String(logFieldKeyDuration, s)
}
func LimitField(n int) Field    { return Int(logFieldKeyLimit, n) }
func PeriodField(n int) Field   { return Int(logFieldKeyPeriod, n) }
func PRNumberField(n int) Field { return Int(logFieldKeyPRNumber, n) }
func SceneField(n int) Field    { return Int(logFieldKeyScene, n) }

// Location / IO
func AddrField(addr string) Field        { return String(logFieldKeyAddr, addr) }
func FileField(name string) Field        { return String(logFieldKeyFile, name) }
func FormatField(format string) Field    { return String(logFieldKeyFormat, format) }
func OutputField(out string) Field       { return String(logFieldKeyOutput, out) }
func PathField(p string) Field           { return String(logFieldKeyPath, p) }
func ProjectRootField(root string) Field { return String(logFieldKeyProjectRoot, root) }
func ScriptField(script string) Field    { return String(logFieldKeyScript, script) }

// Transport / protocol
func CommandField(cmd string) Field { return String(logFieldKeyCommand, cmd) } // key in fluent_builder.go
func HandlerField(name string) Field {
	return String(logFieldKeyHandler, name)
}
func MethodField(name string) Field { return String(logFieldKeyMethod, name) }
func ProtocolField(p string) Field  { return String(logFieldKeyProtocol, p) }

// Content / tooling
// maxLoggedErrorTextBytes caps error/arguments strings written into JSONL sinks
// (e.g. .zqk/scheduler/diagnostics.jsonl). Unbounded MCP tool dumps previously
// produced ~1MB lines that broke SCH-evag ("bufio.Scanner: token too long").
// TRACK: BLI-CAS-HAND-DUP-CHECK-001
const maxLoggedErrorTextBytes = 8 * 1024

func truncateLoggedText(msg string) string {
	if len(msg) <= maxLoggedErrorTextBytes {
		return msg
	}
	return msg[:maxLoggedErrorTextBytes] + "…[truncated]"
}

func ArgumentsField(args string) Field {
	return String(logFieldKeyArguments, truncateLoggedText(args))
}
func ErrorTextField(msg string) Field { return String(logFieldKeyError, truncateLoggedText(msg)) }
func KindField(kind string) Field     { return String(logFieldKeyKind, kind) }
func NormalizedNameField(name string) Field {
	return String(logFieldKeyNormalizedName, name)
}
func NoteField(note string) Field { return String(logFieldKeyNote, note) }
func ToolNameField(name string) Field {
	return String(logFieldKeyToolName, name)
}

// --- fluentEntry helpers (prefer these over .String("key", …) / .Int("key", …)) ---

// PlanID logs a priority-plan / CAP plan id (wire key "plan_id").
func (e *fluentEntry) PlanID(id string) *fluentEntry {
	e.fields = append(e.fields, PlanIDField(id))
	return e
}

// TaskID logs an agent_task / work-item id (wire key "task_id").
func (e *fluentEntry) TaskID(id string) *fluentEntry {
	e.fields = append(e.fields, TaskIDField(id))
	return e
}

// AgentID logs a swarm seat / agent stamp (wire key "agent_id").
func (e *fluentEntry) AgentID(id string) *fluentEntry {
	e.fields = append(e.fields, AgentIDField(id))
	return e
}

// Channel logs a messaging/bridge channel name (wire key "channel").
func (e *fluentEntry) Channel(ch string) *fluentEntry {
	e.fields = append(e.fields, ChannelField(ch))
	return e
}

// EventID logs a feed/event id stamp (wire key "event_id").
func (e *fluentEntry) EventID(id string) *fluentEntry {
	e.fields = append(e.fields, EventIDField(id))
	return e
}

// FeedID logs an agent_feed id (wire key "feed_id").
func (e *fluentEntry) FeedID(id string) *fluentEntry {
	e.fields = append(e.fields, FeedIDField(id))
	return e
}

// PersonaRef logs a persona object id (wire key "persona_ref").
func (e *fluentEntry) PersonaRef(id string) *fluentEntry {
	e.fields = append(e.fields, PersonaRefField(id))
	return e
}

// PersonaTitle logs a persona title (wire key "persona_title").
func (e *fluentEntry) PersonaTitle(title string) *fluentEntry {
	e.fields = append(e.fields, String(logFieldKeyPersonaTitle, title))
	return e
}

// DeliveryMode logs agent chat delivery mode (wire key "delivery_mode").
func (e *fluentEntry) DeliveryMode(mode string) *fluentEntry {
	e.fields = append(e.fields, DeliveryModeField(mode))
	return e
}

// Role logs a persona/job role string (wire key "role").
func (e *fluentEntry) Role(role string) *fluentEntry {
	e.fields = append(e.fields, RoleField(role))
	return e
}

// Status logs a lifecycle or outcome status (wire key "status").
// Prefer SessionStatus for convergence_session.status (wire key "session_status").
func (e *fluentEntry) Status(status string) *fluentEntry {
	e.fields = append(e.fields, StatusField(status))
	return e
}

// ObjectKeyID logs wire key "object_id" (distinct from ObjectID which uses "id").
func (e *fluentEntry) ObjectKeyID(id string) *fluentEntry {
	e.fields = append(e.fields, ObjectIDField(id))
	return e
}

// ParentID logs a parent object id (wire key "parent_id").
func (e *fluentEntry) ParentID(id string) *fluentEntry {
	e.fields = append(e.fields, ParentIDField(id))
	return e
}

// ItemID logs a mesh/sync item id (wire key "item_id").
func (e *fluentEntry) ItemID(id string) *fluentEntry {
	e.fields = append(e.fields, ItemIDField(id))
	return e
}

// Handler logs a handler name (wire key "handler").
func (e *fluentEntry) Handler(name string) *fluentEntry {
	e.fields = append(e.fields, HandlerField(name))
	return e
}

// Stage logs a pipeline/CAP stage label (wire key "stage").
func (e *fluentEntry) Stage(stage string) *fluentEntry {
	e.fields = append(e.fields, StageField(stage))
	return e
}

// Note logs an operator note (wire key "note").
func (e *fluentEntry) Note(note string) *fluentEntry {
	e.fields = append(e.fields, NoteField(note))
	return e
}

// ErrorText logs a string error message (wire key "error"). Prefer WithError(err) when you have an error value.
func (e *fluentEntry) ErrorText(msg string) *fluentEntry {
	e.fields = append(e.fields, ErrorTextField(msg))
	return e
}

// Output logs command or subprocess output text (wire key "output").
func (e *fluentEntry) Output(out string) *fluentEntry {
	e.fields = append(e.fields, OutputField(out))
	return e
}

// Format logs an output format name (wire key "format").
func (e *fluentEntry) Format(format string) *fluentEntry {
	e.fields = append(e.fields, FormatField(format))
	return e
}

// Limit logs a numeric limit (wire key "limit").
func (e *fluentEntry) Limit(n int) *fluentEntry {
	e.fields = append(e.fields, LimitField(n))
	return e
}

// EventName logs a short event label (wire key "event"). Prefer EventType for "event_type".
func (e *fluentEntry) EventName(event string) *fluentEntry {
	e.fields = append(e.fields, EventField(event))
	return e
}

// Addr logs a listen/dial address (wire key "addr").
func (e *fluentEntry) Addr(addr string) *fluentEntry {
	e.fields = append(e.fields, AddrField(addr))
	return e
}

// Script logs a script path or body label (wire key "script"). Prefer ScriptPath for "script_path".
func (e *fluentEntry) Script(script string) *fluentEntry {
	e.fields = append(e.fields, ScriptField(script))
	return e
}

// PRNumber logs a GitHub PR number (wire key "pr_number").
func (e *fluentEntry) PRNumber(n int) *fluentEntry {
	e.fields = append(e.fields, PRNumberField(n))
	return e
}

// Scene logs a media/composer scene index (wire key "scene").
func (e *fluentEntry) Scene(n int) *fluentEntry {
	e.fields = append(e.fields, Int(logFieldKeyScene, n))
	return e
}

// IDE logs an IDE product name (wire key "ide").
func (e *fluentEntry) IDE(name string) *fluentEntry {
	e.fields = append(e.fields, String(logFieldKeyIDE, name))
	return e
}

// Dest logs a destination path (wire key "dest").
func (e *fluentEntry) Dest(path string) *fluentEntry {
	e.fields = append(e.fields, String(logFieldKeyDest, path))
	return e
}

// Action logs an action verb/label (wire key "action").
func (e *fluentEntry) Action(action string) *fluentEntry {
	e.fields = append(e.fields, String(logFieldKeyAction, action))
	return e
}

// Version logs a version string (wire key "version").
func (e *fluentEntry) Version(v string) *fluentEntry {
	e.fields = append(e.fields, String(logFieldKeyVersion, v))
	return e
}

// Interval logs a duration/interval label (wire key "interval").
func (e *fluentEntry) Interval(s string) *fluentEntry {
	e.fields = append(e.fields, String(logFieldKeyInterval, s))
	return e
}

// Prompt logs a prompt snippet or label (wire key "prompt").
func (e *fluentEntry) Prompt(s string) *fluentEntry {
	e.fields = append(e.fields, String(logFieldKeyPrompt, s))
	return e
}

// ChangeType logs a journal change type (wire key "changeType").
func (e *fluentEntry) ChangeType(t string) *fluentEntry {
	e.fields = append(e.fields, String(logFieldKeyChangeType, t))
	return e
}

// SkillID logs an agent_skill id (wire key "skill_id").
func (e *fluentEntry) SkillID(id string) *fluentEntry {
	e.fields = append(e.fields, SkillIDField(id))
	return e
}

// EnvelopeID logs a TDE/envelope id (wire key "envelope_id").
func (e *fluentEntry) EnvelopeID(id string) *fluentEntry {
	e.fields = append(e.fields, EnvelopeIDField(id))
	return e
}

// ToolName logs an MCP/tool name (wire key "toolName").
func (e *fluentEntry) ToolName(name string) *fluentEntry {
	e.fields = append(e.fields, ToolNameField(name))
	return e
}
