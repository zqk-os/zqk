// Package systemcheckwake evaluates system-check summaries and optionally wakes a mesh seat.
//
// Opt-in only via `zqk system check --notify [agent-id]` — never auto-fires on every check.
// TRACK: BLI-COMMS-TPM-LIVE-WAKE-001 — ActionRequired/toast still do not auto-start a Cursor turn.
package systemcheckwake

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/idebridge"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/mcp"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/primaryorch"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	// ConfigFileName is the optional threshold override under .zqk/config/.
	ConfigFileName = "system_check_wake.json"
	// SchemaVersion is the lite config schema_version.
	SchemaVersion = "1"
	// PrimarySentinel is the CLI NoOptDefVal for bare --notify (resolve primary orchestrator).
	PrimarySentinel = "@primary"
	// FromAgentID stamps feed events from system check notify.
	FromAgentID = "system-check"
)

// Config holds optional threshold / delivery overrides (not an auto-enable switch).
type Config struct {
	SchemaVersion          string `json:"schema_version"`
	PulseHuman             *bool  `json:"pulse_human,omitempty"`
	FeedWake               *bool  `json:"feed_wake,omitempty"`
	MinDraftPlane          *int   `json:"min_draft_plane,omitempty"`
	MinErrorStatus         *int   `json:"min_error_status,omitempty"`
	MinBlocking            *int   `json:"min_blocking,omitempty"`
	MinWarnings            *int   `json:"min_warnings,omitempty"`
	MinInformational       *int   `json:"min_informational,omitempty"`
	IncludeRecommendations bool   `json:"include_recommendations,omitempty"`
	MinRecommendations     *int   `json:"min_recommendations,omitempty"`
}

// Summary is the wake-relevant rollup from a finished system check.
type Summary struct {
	DraftPlaneTotal    int
	ErrorStatusObjects int
	BlockingIssues     int
	Warnings           int
	Informational      int
	Recommendations    int
}

// TripReason names one threshold that fired.
type TripReason struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
	Min   int    `json:"min"`
}

// Result is the outcome of Notify (best-effort; never fails the check).
type Result struct {
	Requested         bool         `json:"requested"`
	Tripped           bool         `json:"tripped"`
	Reasons           []TripReason `json:"reasons,omitempty"`
	ToAgentID         string       `json:"to_agent_id,omitempty"`
	Skipped           string       `json:"skipped,omitempty"`
	PeerWakeLive      bool         `json:"peer_wake_live,omitempty"`
	PeerWakeTransport string       `json:"peer_wake_transport,omitempty"`
	IdeBridgeQueued   bool         `json:"ide_bridge_queued,omitempty"`
	EventID           string       `json:"event_id,omitempty"`
	Error             string       `json:"error,omitempty"`
}

// DefaultConfig returns built-in thresholds (any count ≥ 1 for draft/error/blocking/warning/info).
func DefaultConfig() Config {
	one := 1
	pulse := true
	feed := true
	return Config{
		SchemaVersion:    SchemaVersion,
		PulseHuman:       &pulse,
		FeedWake:         &feed,
		MinDraftPlane:    &one,
		MinErrorStatus:   &one,
		MinBlocking:      &one,
		MinWarnings:      &one,
		MinInformational: &one,
	}
}

// ConfigPath returns .zqk/config/system_check_wake.json.
func ConfigPath(projectRoot string) string {
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.ConfigDir, ConfigFileName)
}

// LoadConfig reads optional overrides; missing file → DefaultConfig.
func LoadConfig(projectRoot string) (Config, error) {
	def := DefaultConfig()
	p := ConfigPath(projectRoot)
	b, err := fileutil.ReadFile(p)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return def, nil
		}
		return Config{}, errfmt.Newf("systemcheckwake: read config").Wrap(err)
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return Config{}, errfmt.Newf("systemcheckwake: parse config").Wrap(err)
	}
	if c.SchemaVersion == "" {
		c.SchemaVersion = SchemaVersion
	}
	mergeIntPtr := func(dst **int, src *int) {
		if src != nil {
			*dst = src
		}
	}
	mergeIntPtr(&def.MinDraftPlane, c.MinDraftPlane)
	mergeIntPtr(&def.MinErrorStatus, c.MinErrorStatus)
	mergeIntPtr(&def.MinBlocking, c.MinBlocking)
	mergeIntPtr(&def.MinWarnings, c.MinWarnings)
	mergeIntPtr(&def.MinInformational, c.MinInformational)
	mergeIntPtr(&def.MinRecommendations, c.MinRecommendations)
	if c.PulseHuman != nil {
		def.PulseHuman = c.PulseHuman
	}
	if c.FeedWake != nil {
		def.FeedWake = c.FeedWake
	}
	def.IncludeRecommendations = c.IncludeRecommendations
	def.SchemaVersion = c.SchemaVersion
	return def, nil
}

func derefMin(p *int, fallback int) int {
	if p == nil {
		return fallback
	}
	return *p
}

func derefBool(p *bool, fallback bool) bool {
	if p == nil {
		return fallback
	}
	return *p
}

// Evaluate returns trip reasons for summary against cfg (empty ⇒ no trip).
func Evaluate(sum Summary, cfg Config) []TripReason {
	var reasons []TripReason
	check := func(name string, count, min int) {
		if min <= 0 {
			return
		}
		if count >= min {
			reasons = append(reasons, TripReason{Name: name, Count: count, Min: min})
		}
	}
	check("draft_plane", sum.DraftPlaneTotal, derefMin(cfg.MinDraftPlane, 1))
	check("error_status", sum.ErrorStatusObjects, derefMin(cfg.MinErrorStatus, 1))
	check("blocking", sum.BlockingIssues, derefMin(cfg.MinBlocking, 1))
	check("warnings", sum.Warnings, derefMin(cfg.MinWarnings, 1))
	check("informational", sum.Informational, derefMin(cfg.MinInformational, 1))
	if cfg.IncludeRecommendations {
		minRec := derefMin(cfg.MinRecommendations, 1)
		check("recommendations", sum.Recommendations, minRec)
	}
	return reasons
}

// ResolveNotifyAgent returns the seat to wake.
// flagValue empty / PrimarySentinel → primaryorch binding agent_id.
func ResolveNotifyAgent(projectRoot, flagValue string) (string, error) {
	v := strings.TrimSpace(flagValue)
	if v == "" || v == PrimarySentinel {
		b, err := primaryorch.LoadBinding(projectRoot)
		if err != nil {
			return "", err
		}
		id := strings.TrimSpace(b.AgentID)
		if id == "" {
			return "", errfmt.Errorf("systemcheckwake: primary orchestrator agent_id empty")
		}
		return id, nil
	}
	return v, nil
}

// NotifyOpts configures a wake attempt.
type NotifyOpts struct {
	ProjectRoot string
	ToAgentID   string // already resolved
	Summary     Summary
	Config      Config
	Logger      logging.Logger
	// Context for MCP probe / wake (optional).
	Context context.Context
}

// Notify evaluates thresholds and wakes the seat when tripped. Never returns a hard error
// that should fail system check — errors are recorded on Result.
func Notify(opts NotifyOpts) Result {
	res := Result{Requested: true, ToAgentID: strings.TrimSpace(opts.ToAgentID)}
	cfg := opts.Config
	if cfg.SchemaVersion == "" {
		cfg = DefaultConfig()
	}
	reasons := Evaluate(opts.Summary, cfg)
	if len(reasons) == 0 {
		res.Skipped = "thresholds_clear"
		if opts.Logger != nil {
			logging.Fluent(opts.Logger).Info("system check --notify: thresholds clear; no wake").
				String("to_agent_id", res.ToAgentID).
				Log()
		}
		return res
	}
	res.Tripped = true
	res.Reasons = reasons

	msg := formatWakeMessage(opts.Summary, reasons)
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}
	root := strings.TrimSpace(opts.ProjectRoot)
	to := res.ToAgentID
	if to == "" {
		res.Skipped = "empty_to_agent_id"
		res.Error = "to_agent_id empty"
		return res
	}

	feedWake := derefBool(cfg.FeedWake, true)
	pulse := derefBool(cfg.PulseHuman, true)

	if feedWake {
		var appendRes agentfeed.AppendEventResult
		var appendErr error
		appendRes, appendErr = agentfeed.AppendEvent(agentfeed.AppendEventInput{
			ProjectRoot: root,
			Message:     msg,
			AgentID:     FromAgentID,
			ToAgentID:   to,
			Sender:      agentfeed.FeedSenderHumanSteer,
			EventType:   agentfeed.FeedEventTypeSteering,
			SelfACK:     true,
		})
		if appendErr != nil {
			res.Error = appendErr.Error()
			if opts.Logger != nil {
				logging.Fluent(opts.Logger).Warn("system check --notify: feed append failed").
					WithError(appendErr).
					Log()
			}
		} else {
			res.EventID = appendRes.EventID
			// POL-AGENT-ORCH-HOURGLASS-001: directed notify must register await (no CLI flag here).
			if aw, aerr := agentfeed.RegisterPeerAckAwait(root, agentfeed.PeerAckAwaitInput{
				EventID:     appendRes.EventID,
				FromAgentID: FromAgentID,
				ToAgentID:   to,
				Action:      agentfeed.AwaitActionWake,
				WakeMessage: agentfeed.PeerAckPasteStub(appendRes.EventID),
			}); aerr != nil {
				if res.Error == "" {
					res.Error = aerr.Error()
				}
				if opts.Logger != nil {
					logging.Fluent(opts.Logger).Warn("system check --notify: peer-ack await register failed").
						WithError(aerr).
						Log()
				}
			} else if opts.Logger != nil {
				logging.Fluent(opts.Logger).Info("system check --notify: peer-ack await registered").
					String("await_id", aw.ID).
					String("event_id", appendRes.EventID).
					Log()
			}
		}

		mcpIPC := false
		mcpProbed := false
		mcpSubs := -1
		if opts.Logger == nil {
			opts.Logger = logging.GetLoggerFromProfile("system")
		}
		probe := mcp.ProbeFeedSteerMCPWake(ctx, "", msg, FromAgentID, res.EventID, opts.Logger)
		mcpIPC = probe.IPCDelivered
		mcpProbed = probe.SubscribersProbed
		mcpSubs = probe.SubscriberCount

		wake := agentfeed.WakePeerOpts(ctx, agentfeed.WakePeerOptions{
			ProjectRoot:          root,
			Message:              msg,
			InReplyTo:            res.EventID,
			FromAgentID:          FromAgentID,
			DeliveryMode:         datacell.DeliveryModeNotify,
			ToAgentID:            to,
			MCPIPCDelivered:      mcpIPC,
			MCPSubscribersProbed: mcpProbed,
			MCPSubscriberCount:   mcpSubs,
		})
		res.PeerWakeLive = wake.Live
		res.PeerWakeTransport = wake.Transport
		res.IdeBridgeQueued = wake.IdeBridgeQueued
		if wake.Error != "" && res.Error == "" {
			res.Error = wake.Error
		}
		if opts.Logger != nil {
			logging.Fluent(opts.Logger).Info("system check --notify: wake attempted").
				String("to_agent_id", to).
				String("transport", wake.Transport).
				Bool("live", wake.Live).
				String("reasons", formatReasons(reasons)).
				Log()
		}
	}

	if pulse && !res.IdeBridgeQueued {
		if idebridge.QueueProofOfLife(root, msg) {
			res.IdeBridgeQueued = true
		}
	}
	return res
}

func formatReasons(reasons []TripReason) string {
	parts := make([]string, 0, len(reasons))
	for _, r := range reasons {
		parts = append(parts, fmt.Sprintf("%s=%d", r.Name, r.Count))
	}
	return strings.Join(parts, ",")
}

func formatWakeMessage(sum Summary, reasons []TripReason) string {
	return fmt.Sprintf(
		"ATTN system-check --notify: thresholds tripped [%s] (draft=%d error_status=%d blocking=%d warnings=%d info=%d)",
		formatReasons(reasons),
		sum.DraftPlaneTotal,
		sum.ErrorStatusObjects,
		sum.BlockingIssues,
		sum.Warnings,
		sum.Informational,
	)
}

// WriteExampleConfig writes the example file next to config (for docs/bootstrap).
func WriteExampleConfig(projectRoot string, c Config) error {
	p := ConfigPath(projectRoot) + ".example"
	if err := fileutil.MkdirAll(filepath.Dir(p), paths.DirPerm755); err != nil {
		return err
	}
	if c.SchemaVersion == "" {
		c = DefaultConfig()
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return fileutil.WriteSecureFile(p, b)
}
