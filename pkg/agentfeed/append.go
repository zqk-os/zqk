package agentfeed

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

// Feed sender / event_type stamps for JSONL rows (Go-only ship path).
const (
	FeedSenderIDEChat         = "ide_chat"
	FeedSenderHumanSteer      = "human_steer"
	FeedSenderChatResponder   = "chat_responder"
	FeedSenderMeshStatus      = "mesh_status"
	FeedSenderDelivery        = "delivery"
	FeedSenderPeerAck         = "peer_ack"
	FeedSenderMessagingBridge = "messaging_bridge" // Phase C enterprise ingress (optionally _slack/_teams/…)
	FeedSenderHTTPAPI         = "http_api"         // Phase D private/public HTTP surface ([REDACTED-ID])

	FeedEventTypeSteering        = "steering"
	FeedEventTypeChat            = "chat"
	FeedEventTypeMeshStatus      = "mesh_status"
	FeedEventTypeKernelAck       = "kernel_ack" // not peer receipt — feed write confirmation only
	FeedEventTypePeerAck         = "peer_ack"
	FeedEventTypeDeliveryReceipt = "delivery_receipt"
	FeedEventTypeScanTests       = "scan-tests"
	FeedEventTypeCheckpoint      = "checkpoint"
	FeedEventTypePromote         = "promote"
	FeedEventTypeWake            = "wake"

	FeedKernelAgentID = "kernel"

	// JSONL / feed HTTP wire names for correspondence.
	// These are intentionally NOT objects.FieldKey* — the FieldKey literal fixer only
	// rewrites wires registered in pkg/objects/field_keys.go, so mixed maps look "half
	// fixed" unless feed code uses these JSONField* constants for the rest.
	JSONFieldEventID         = "event_id"
	JSONFieldInReplyTo       = "in_reply_to"
	JSONFieldToAgentID       = "to_agent_id"
	JSONFieldFromAgentID     = "from_agent_id"
	JSONFieldAgentID         = "agent_id"
	JSONFieldMessage         = "message"
	JSONFieldTimestamp       = "timestamp"
	JSONFieldSender          = "sender"
	JSONFieldFeedID          = "feed_id"
	JSONFieldEventPath       = "event_path"
	JSONFieldPersonaID       = "persona_id"
	JSONFieldSkipReason      = "skip_reason"
	JSONFieldInboxUnacked    = "inbox_unacked"
	JSONFieldOutboxAwait     = "outbox_awaiting_peer_ack"
	JSONFieldSchema          = "schema"
	JSONFieldSurface         = "surface"
	JSONFieldError           = "error"
	JSONFieldFields          = "fields"
	JSONFieldPeerAckAwaitID  = "peer_ack_await_id"
	JSONFieldAwaitPeerAck    = "await_peer_ack"
	JSONFieldAwaitsCompleted = "awaits_completed"
)

// EventEmitter interface abstracts MCP notification broadcasting.
type EventEmitter interface {
	Emit(event any) int
}

// AppendEventInput is the shared write path for MCP chat_send and zqk feed steer.
type AppendEventInput struct {
	ProjectRoot string
	Message     string
	AgentID     string
	// PersonaRef is a kernel persona id (PER-*). Prefer this over inventing role enums.
	PersonaRef string
	// Role is optional and should be copied from the persona object when known
	// (persona.role field), not an ad-hoc CLI vocabulary.
	Role string
	// ToAgentID optionally addresses this event to a peer swarm seat (inbox routing).
	ToAgentID string
	SessionID string
	Sender    string
	EventType string
	// InReplyTo correlates peer_ack / delivery_receipt / kernel_ack to a parent event_id.
	InReplyTo string
	// EventID overrides generated id (tests). Empty → NewEventID().
	EventID string
	// SelfACK appends a kernel_ack row (feed write confirmation — NOT peer receipt).
	SelfACK    bool
	ACKMessage string
	// SkipEnabledCheck allows privileged callers to append even when lite enabled=false (tests / recovery).
	SkipEnabledCheck bool
	// EventEmitter optionally broadcasts EventTypeActionRequired to live MCP subscribers.
	EventEmitter EventEmitter
}

// AppendEventResult describes a successful append.
type AppendEventResult struct {
	EventPath    string
	StartSize    int64
	Event        map[string]any
	ACK          map[string]any
	Enabled      bool
	DeliveryMode string
	FeedID       string
	EventID      string
}

// NewEventID returns a stable feed event id (AFE-<nano>-<hex>).
func NewEventID() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("AFE-%d-%s", time.Now().UTC().UnixNano(), hex.EncodeToString(b[:]))
}

// AppendEvent appends one JSONL event (and optional kernel_ack) to the agent chat channel.
// Respects lite config enabled flag unless SkipEnabledCheck is set.
func AppendEvent(in AppendEventInput) (AppendEventResult, error) {
	msg := strings.TrimSpace(in.Message)
	if msg == "" {
		return AppendEventResult{}, errfmt.Errorf("missing required parameter: message")
	}
	root := strings.TrimSpace(in.ProjectRoot)
	if root == "" {
		cwd, err := fileutil.Getwd()
		if err != nil {
			return AppendEventResult{}, errfmt.Errorf("failed to get working directory: %w", err)
		}
		root = cwd
	}

	if paths.IsAgentWorktreePath(root) {
		if seated := paths.LoadBrandSettingsProjectRoot(root); seated != "" && !paths.IsAgentWorktreePath(seated) {
			root = seated
		} else {
			return AppendEventResult{}, errfmt.Errorf("refusing to append event to unbonded agent worktree: %s", root)
		}
	}

	cfg, err := datacell.ReadAgentChatChannelConfig(root)
	if err != nil {
		return AppendEventResult{}, errfmt.Errorf("read agent chat channel config: %w", err)
	}
	if !in.SkipEnabledCheck && !cfg.Enabled {
		return AppendEventResult{}, errfmt.Errorf("agent chat feed disabled (enabled=false in agent_chat_channel lite file)")
	}
	if !in.SkipEnabledCheck && strings.TrimSpace(cfg.DeliveryMode) == datacell.DeliveryModeOff {
		return AppendEventResult{}, errfmt.Errorf("agent chat feed delivery_mode=off")
	}

	eventPath := datacell.EffectiveAgentChatChannelEventsJSONLPath(root, cfg)
	if err := fileutil.EnsureDir(filepath.Dir(eventPath)); err != nil {
		return AppendEventResult{}, errfmt.Errorf("failed to create agent chat directory: %w", err)
	}

	var startSize int64
	if size, ok := fileutil.FileSize(eventPath); ok {
		startSize = size
	}

	agentID := strings.TrimSpace(in.AgentID)
	if agentID == "" {
		agentID = "unknown"
	}
	sender := strings.TrimSpace(in.Sender)
	if sender == "" {
		sender = FeedSenderIDEChat
	}

	eventID := strings.TrimSpace(in.EventID)
	if eventID == "" {
		eventID = NewEventID()
	}

	timestamp := time.Now().UTC()
	event := map[string]any{
		JSONFieldTimestamp:        timestamp.Format(time.RFC3339),
		JSONFieldSender:           sender,
		JSONFieldAgentID:          agentID,
		objects.FieldKeySessionID: in.SessionID,
		JSONFieldMessage:          msg,
		JSONFieldEventID:          eventID,
	}
	if pref := strings.TrimSpace(in.PersonaRef); pref != "" {
		event[objects.FieldKeyPersonaRef] = pref
	}
	if role := strings.TrimSpace(in.Role); role != "" {
		event[objects.FieldKeyRole] = role
	}
	if et := strings.TrimSpace(in.EventType); et != "" {
		event[objects.FieldKeyEventType] = et
	}
	if to := strings.TrimSpace(in.ToAgentID); to != "" {
		event[JSONFieldToAgentID] = to
	}
	if ir := strings.TrimSpace(in.InReplyTo); ir != "" {
		event[JSONFieldInReplyTo] = ir
	}
	if csv := strings.TrimSpace(cfg.ContractSchemaVersion); csv != "" {
		event[objects.FieldKeyContractSchemaVersion] = csv
	}
	if fid := strings.TrimSpace(cfg.FeedID); fid != "" {
		event[JSONFieldFeedID] = fid
	}

	f, err := fileutil.OpenAppend(eventPath)
	if err != nil {
		return AppendEventResult{}, errfmt.Errorf("failed to open chat events file: %w", err)
	}
	defer f.Close()

	b, err := json.Marshal(event)
	if err != nil {
		return AppendEventResult{}, errfmt.Errorf("failed to marshal event: %w", err)
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		return AppendEventResult{}, errfmt.Errorf("failed to write event: %w", err)
	}

	var ack map[string]any
	if in.SelfACK {
		ackMsg := strings.TrimSpace(in.ACKMessage)
		if ackMsg == "" {
			ackMsg = "ZQK_KERNEL_ACK — message received by kernel. Processing: " + msg
		}
		ackID := NewEventID()
		ack = map[string]any{
			JSONFieldTimestamp:        timestamp.Add(time.Millisecond).Format(time.RFC3339Nano),
			JSONFieldSender:           FeedSenderChatResponder,
			JSONFieldAgentID:          FeedKernelAgentID,
			objects.FieldKeySessionID: in.SessionID,
			JSONFieldMessage:          ackMsg,
			JSONFieldEventID:          ackID,
			JSONFieldInReplyTo:        eventID,
			objects.FieldKeyEventType: FeedEventTypeKernelAck,
		}
		if ab, err := json.Marshal(ack); err == nil {
			if _, wErr := f.Write(append(ab, '\n')); wErr != nil {
				return AppendEventResult{}, errfmt.Errorf("failed to write ack: %w", wErr)
			}
		}
	}

	if in.EventEmitter != nil {
		_ = in.EventEmitter.Emit(map[string]any{
			objects.FieldKeyType:     "action.required",
			JSONFieldTimestamp:       timestamp.Format(time.RFC3339),
			JSONFieldMessage:         fmt.Sprintf("Feed event from %s: %s", agentID, msg),
			objects.FieldKeySeverity: "info",
			objects.FieldKeyPriority: "high",
			JSONFieldFields:          event,
		})
	}

	return AppendEventResult{
		EventPath:    eventPath,
		StartSize:    startSize,
		Event:        event,
		ACK:          ack,
		Enabled:      cfg.Enabled,
		DeliveryMode: cfg.DeliveryMode,
		FeedID:       cfg.FeedID,
		EventID:      eventID,
	}, nil
}

// AppendPeerAck records cognitive peer acknowledgment of a prior event.
// Directed steers (to_agent_id set) may only be peer_acked by that seat
// (or a documented alias). Cross-seat impersonation is denied.
func AppendPeerAck(projectRoot, agentID, personaRef, inReplyTo, summary string) (AppendEventResult, error) {
	return AppendPeerAckWithSession(projectRoot, agentID, personaRef, "", inReplyTo, summary)
}

// AppendPeerAckWithSession records a peer_ack stamped with the authoring zqk_session.
// TRACK: REQ-COMMS-RUNTIME-SESSION-001 — provenance via session_id, not seat-name heuristics.
func AppendPeerAckWithSession(projectRoot, agentID, personaRef, sessionID, inReplyTo, summary string) (AppendEventResult, error) {
	msg := strings.TrimSpace(summary)
	if msg == "" {
		msg = "PEER_ACK — received " + strings.TrimSpace(inReplyTo)
	}
	if strings.TrimSpace(inReplyTo) == "" {
		return AppendEventResult{}, errfmt.Errorf("in_reply_to is required for peer_ack")
	}
	if err := AuthorizePeerAck(projectRoot, agentID, inReplyTo); err != nil {
		return AppendEventResult{}, err
	}
	res, err := AppendEvent(AppendEventInput{
		ProjectRoot: projectRoot,
		Message:     msg,
		AgentID:     agentID,
		PersonaRef:  personaRef,
		SessionID:   strings.TrimSpace(sessionID),
		Sender:      FeedSenderPeerAck,
		EventType:   FeedEventTypePeerAck,
		InReplyTo:   inReplyTo,
		SelfACK:     false,
	})
	if err != nil {
		return res, err
	}
	// Seat-worker and HTTP/CLI all go through here. Completing here is the
	// hourglass close — POL-AGENT-ORCH-HOURGLASS-001. Do not leave this to
	// zqk feed ack alone (that path is unused by seat-worker).
	if _, cerr := CompletePeerAckAwaits(projectRoot, inReplyTo, agentID); cerr != nil {
		return res, errfmt.Newf("peer_ack appended but await complete failed for %s", inReplyTo).Wrap(cerr)
	}
	return res, nil
}

// AppendDeliveryReceipt records transport delivery success for a prior event.
func AppendDeliveryReceipt(projectRoot, fromAgentID, inReplyTo, summary string) (AppendEventResult, error) {
	msg := strings.TrimSpace(summary)
	if msg == "" {
		msg = "DELIVERY_RECEIPT — wake/notify succeeded for " + strings.TrimSpace(inReplyTo)
	}
	if strings.TrimSpace(inReplyTo) == "" {
		return AppendEventResult{}, errfmt.Errorf("in_reply_to is required for delivery_receipt")
	}
	return AppendEvent(AppendEventInput{
		ProjectRoot:      projectRoot,
		Message:          msg,
		AgentID:          fromAgentID,
		Sender:           FeedSenderDelivery,
		EventType:        FeedEventTypeDeliveryReceipt,
		InReplyTo:        inReplyTo,
		SelfACK:          false,
		SkipEnabledCheck: false,
	})
}
