// Package primaryorch resolves the host/project primary orchestrator and wakes
// them through pluggable vendor adapters (script, agent chat channel, inbox).
//
// Binding lives at .zqk/agent-runtime/primary_orchestrator.json — not in vendor brain dirs.
// See core-backlog.
package primaryorch

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// SchemaVersion is the binding file schema_version.
const SchemaVersion = "1"

// ConfigFileName is the lite binding under .zqk/agent-runtime/.
const ConfigFileName = "primary_orchestrator.json"

// DefaultAgentIDFallback is the vendor-neutral agent_id when no binding file and
// no ZQK_AGENT_ID (or brand-prefixed) env is set. Project-specific seating
// belongs in primary_orchestrator.json (e.g. peer-tpm-01, peer-agent-01).
const DefaultAgentIDFallback = "primary"

// Known adapter names (vendor-specific implementations behind a stable contract).
const (
	AdapterScript    = "script"     // run argv with message as final arg (e.g. wake-agy.sh)
	AdapterAgentChat = "agent_chat" // append to agent chat channel JSONL
	AdapterInbox     = "inbox"      // write under .zqk/inbox/<agent_id>/
	AdapterNoop      = "noop"       // record success without I/O (tests / dry-run)
)

const (
	wakeFilePrefix          = "wake-"
	wakeFileExt             = ".txt"
	wakeFileTimestampLayout = "20060102T150405Z"
	wakeEventSender         = "primaryorch"
)

// Binding is the machine-readable primary orchestrator for this host+project.
type Binding struct {
	SchemaVersion string `json:"schema_version"`
	// AgentID is a stable identity (e.g. peer-tpm-01, peer-agent-01, human).
	AgentID string `json:"agent_id"`
	// Adapter selects the wake transport (script | agent_chat | inbox | noop).
	Adapter string `json:"adapter"`
	// Script is relative to project root or absolute; used when Adapter=script.
	Script string `json:"script,omitempty"`
	// InboxSubdir overrides the inbox leaf when Adapter=inbox (default: agent_id).
	InboxSubdir string `json:"inbox_subdir,omitempty"`
	Note        string `json:"note,omitempty"`
}

// WakeRequest is a TPM/attentiveness wake payload.
type WakeRequest struct {
	TaskID  string
	Persona string
	Message string
	// PlanID is the active priority_plan id (PRI-*) when known.
	PlanID string
	// TopBLIs are up to three open P0/P1 backlog briefs appended to the wake text.
	TopBLIs []BacklogBrief
}

// WakeResult records where the wake landed (for logs / audit).
type WakeResult struct {
	AgentID     string
	Adapter     string
	DeliveredTo string
}

// Adapter wakes the seated orchestrator through one transport.
type Adapter interface {
	Name() string
	Wake(ctx context.Context, projectRoot string, b Binding, req WakeRequest) (WakeResult, error)
}

// ConfigPath returns .zqk/agent-runtime/primary_orchestrator.json.
func ConfigPath(projectRoot string) string {
	return paths.AgentRuntimeFile(projectRoot, ConfigFileName)
}

// DefaultBinding is used when no config file exists.
// AgentID comes from ZQK_AGENT_ID (brand-prefixed) when set; otherwise
// DefaultAgentIDFallback. Adapter defaults to agent_chat (transport, not vendor).
// wake-agy / IDE / etc. are selected only via binding.script or project config.
func DefaultBinding() Binding {
	agentID := strings.TrimSpace(zqkenv.AgentID().Get())
	if agentID == "" {
		agentID = DefaultAgentIDFallback
	}
	return Binding{
		SchemaVersion: SchemaVersion,
		AgentID:       agentID,
		Adapter:       AdapterAgentChat,
		Note:          "default: primary via agent chat; set agent_id in primary_orchestrator.json or " + zqkenv.AgentID().Name(),
	}
}

// LoadBinding reads the project binding or returns DefaultBinding when missing.
func LoadBinding(projectRoot string) (Binding, error) {
	p := ConfigPath(projectRoot)
	b, err := fileutil.ReadFile(p)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return DefaultBinding(), nil
		}
		return Binding{}, errfmt.Newf("primaryorch: read binding").Wrap(err)
	}
	var cfg Binding
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Binding{}, errfmt.Newf("primaryorch: parse binding").Wrap(err)
	}
	if cfg.SchemaVersion == "" {
		cfg.SchemaVersion = SchemaVersion
	}
	if cfg.AgentID == "" {
		cfg.AgentID = DefaultBinding().AgentID
	}
	if cfg.Adapter == "" {
		cfg.Adapter = DefaultBinding().Adapter
	}
	return cfg, nil
}

// WriteBinding writes the binding lite file (0600).
func WriteBinding(projectRoot string, cfg Binding) error {
	if cfg.SchemaVersion == "" {
		cfg.SchemaVersion = SchemaVersion
	}
	p := ConfigPath(projectRoot)
	if err := fileutil.MkdirAll(filepath.Dir(p), paths.DirPerm755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return fileutil.WriteSecureFile(p, raw)
}

// ResolveAdapter returns the Adapter implementation for the binding.
func ResolveAdapter(b Binding) (Adapter, error) {
	switch strings.ToLower(strings.TrimSpace(b.Adapter)) {
	case AdapterScript:
		return ScriptAdapter{}, nil
	case AdapterAgentChat:
		return AgentChatAdapter{}, nil
	case AdapterInbox:
		return InboxAdapter{}, nil
	case AdapterNoop:
		return NoopAdapter{}, nil
	default:
		return nil, errfmt.Errorf("primaryorch: unknown adapter %q (want script|agent_chat|inbox|noop)", b.Adapter)
	}
}

// WakePrimary loads the project binding and wakes the primary orchestrator.
func WakePrimary(ctx context.Context, projectRoot string, req WakeRequest) (WakeResult, error) {
	b, err := LoadBinding(projectRoot)
	if err != nil {
		return WakeResult{}, err
	}
	ad, err := ResolveAdapter(b)
	if err != nil {
		return WakeResult{}, err
	}
	req.Message = ComposeWakeMessage(req)
	return ad.Wake(ctx, projectRoot, b, req)
}

func defaultWakeMessage(req WakeRequest) string {
	parts := []string{"Primary orchestrator wake"}
	if req.TaskID != "" {
		parts = append(parts, "task="+req.TaskID)
	}
	if req.Persona != "" {
		parts = append(parts, "persona="+req.Persona)
	}
	return strings.Join(parts, " ")
}

// ScriptAdapter runs Binding.Script with the wake message as the last argument.
type ScriptAdapter struct{}

func (ScriptAdapter) Name() string { return AdapterScript }

func (ScriptAdapter) Wake(ctx context.Context, projectRoot string, b Binding, req WakeRequest) (WakeResult, error) {
	script := strings.TrimSpace(b.Script)
	if script == "" {
		return WakeResult{}, errfmt.Errorf("primaryorch: script adapter requires binding.script")
	}
	if !filepath.IsAbs(script) {
		script = filepath.Join(projectRoot, script)
	}
	cmd := execwrap.CommandContext(ctx, script, req.Message)
	cmd.Dir = projectRoot
	if err := cmd.Run(); err != nil {
		return WakeResult{}, errfmt.Newf("primaryorch: script %s", script).Wrap(err)
	}
	return WakeResult{AgentID: b.AgentID, Adapter: AdapterScript, DeliveredTo: "script:" + script}, nil
}

// AgentChatAdapter appends a wake event to the agent chat channel JSONL.
type AgentChatAdapter struct{}

func (AgentChatAdapter) Name() string { return AdapterAgentChat }

func (AgentChatAdapter) Wake(ctx context.Context, projectRoot string, b Binding, req WakeRequest) (WakeResult, error) {
	_ = ctx
	if err := datacell.EnsureAgentChatChannelEventsDir(projectRoot); err != nil {
		return WakeResult{}, errfmt.Newf("primaryorch: agent_chat mkdir").Wrap(err)
	}
	eventPath := datacell.AgentChatChannelEventsJSONLPath(projectRoot)
	f, err := fileutil.OpenFile(eventPath, fileutil.O_APPEND|fileutil.O_CREATE|fileutil.O_WRONLY, paths.FilePerm644)
	if err != nil {
		return WakeResult{}, errfmt.Newf("primaryorch: agent_chat open").Wrap(err)
	}
	defer f.Close()

	event := map[string]any{
		"timestamp":             time.Now().UTC().Format(time.RFC3339),
		"sender":                wakeEventSender,
		objects.FieldKeyAgentID: b.AgentID,
		"task_id":               req.TaskID,
		"persona":               req.Persona,
		"message":               req.Message,
	}
	raw, err := json.Marshal(event)
	if err != nil {
		return WakeResult{}, err
	}
	if _, err := f.Write(append(raw, '\n')); err != nil {
		return WakeResult{}, errfmt.Newf("primaryorch: agent_chat write").Wrap(err)
	}
	return WakeResult{AgentID: b.AgentID, Adapter: AdapterAgentChat, DeliveredTo: "agent_chat:" + eventPath}, nil
}

// InboxAdapter writes a wake note under .zqk/inbox/<agent_id>/.
type InboxAdapter struct{}

func (InboxAdapter) Name() string { return AdapterInbox }

func (InboxAdapter) Wake(ctx context.Context, projectRoot string, b Binding, req WakeRequest) (WakeResult, error) {
	_ = ctx
	leaf := strings.TrimSpace(b.InboxSubdir)
	if leaf == "" {
		leaf = b.AgentID
	}
	leaf = sanitizePathSegment(leaf)
	dir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.InboxSubdir, leaf)
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		return WakeResult{}, err
	}
	name := wakeFilePrefix + time.Now().UTC().Format(wakeFileTimestampLayout) + wakeFileExt
	if req.TaskID != "" {
		name = wakeFilePrefix + sanitizePathSegment(req.TaskID) + wakeFileExt
	}
	path := filepath.Join(dir, name)
	body := req.Message + "\n"
	if err := fileutil.WriteFile(path, []byte(body), paths.FilePerm644); err != nil {
		return WakeResult{}, errfmt.Newf("primaryorch: inbox write").Wrap(err)
	}
	return WakeResult{AgentID: b.AgentID, Adapter: AdapterInbox, DeliveredTo: "inbox:" + path}, nil
}

// NoopAdapter succeeds without side effects.
type NoopAdapter struct{}

func (NoopAdapter) Name() string { return AdapterNoop }

func (NoopAdapter) Wake(ctx context.Context, projectRoot string, b Binding, req WakeRequest) (WakeResult, error) {
	_ = ctx
	_ = projectRoot
	_ = req
	return WakeResult{AgentID: b.AgentID, Adapter: AdapterNoop, DeliveredTo: "noop"}, nil
}

// sanitizePathSegment makes agent ids / task ids safe as a single path element.
func sanitizePathSegment(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "unnamed"
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_':
			b.WriteRune(r)
			lastUnderscore = false
		default:
			if !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "unnamed"
	}
	return out
}
