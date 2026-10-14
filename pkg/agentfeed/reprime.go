package agentfeed

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	InFlightStateSchema   = "zqk_in_flight_state_v1"
	InFlightStatusAligned = "aligned"
	InFlightStatusPending = "pending_reprime"
)

// InFlightSteering represents an unacked steering event delivered to an active agent.
type InFlightSteering struct {
	EventID         string `json:"event_id"`
	FromAgentID     string `json:"from_agent_id"`
	ToAgentID       string `json:"to_agent_id,omitempty"`
	Message         string `json:"message"`
	Timestamp       string `json:"timestamp,omitempty"`
	NonInterrupting bool   `json:"non_interrupting,omitempty"`
	InFlightRePrime bool   `json:"in_flight_reprime,omitempty"`
}

// AgentInFlightState captures the persistent in-flight state of an active agent.
type AgentInFlightState struct {
	Schema          string             `json:"schema"`
	AgentID         string             `json:"agent_id"`
	PersonaRef      string             `json:"persona_ref,omitempty"`
	ActiveGuidance  []InFlightSteering `json:"active_guidance"`
	LastEventID     string             `json:"last_event_id,omitempty"`
	LastRePrimeTime string             `json:"last_reprime_time,omitempty"`
	UpdatedAt       string             `json:"updated_at"`
	Status          string             `json:"status"`
}

// RePrimeInput specifies input parameters for in-flight context re-priming.
type RePrimeInput struct {
	ProjectRoot    string
	AgentID        string
	PersonaRef     string
	CurrentContext string
	SessionID      string
	AutoAck        bool
}

// RePrimeResult contains the outcome of an in-flight context re-priming operation.
type RePrimeResult struct {
	RePrimedContext string             `json:"reprimed_context"`
	Guidance        []InFlightSteering `json:"guidance"`
	AckedEventIDs   []string           `json:"acked_event_ids,omitempty"`
	Applied         bool               `json:"applied"`
	StatePath       string             `json:"state_path,omitempty"`
}

// AgentInFlightStatePath returns the disk path for a seat's persistent in-flight state file.
func AgentInFlightStatePath(projectRoot, agentID string) string {
	safe := strings.ReplaceAll(strings.TrimSpace(agentID), "/", "_")
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir, "mesh", "in_flight", safe+".json")
}

// LoadAgentInFlightState loads the persistent in-flight state for an agent.
func LoadAgentInFlightState(projectRoot, agentID string) (AgentInFlightState, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return AgentInFlightState{}, errfmt.Errorf("agent_id required to load in-flight state")
	}
	path := AgentInFlightStatePath(projectRoot, agentID)
	raw, err := fileutil.ReadFile(path)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return AgentInFlightState{
				Schema:         InFlightStateSchema,
				AgentID:        agentID,
				Status:         InFlightStatusAligned,
				ActiveGuidance: []InFlightSteering{},
			}, nil
		}
		return AgentInFlightState{}, errfmt.Errorf("read in-flight state: %w", err)
	}
	var state AgentInFlightState
	if err := json.Unmarshal(raw, &state); err != nil {
		return AgentInFlightState{}, errfmt.Errorf("unmarshal in-flight state: %w", err)
	}
	return state, nil
}

// SaveAgentInFlightState saves the persistent in-flight state for an agent.
func SaveAgentInFlightState(projectRoot string, state AgentInFlightState) error {
	agentID := strings.TrimSpace(state.AgentID)
	if agentID == "" {
		return errfmt.Errorf("agent_id required to save in-flight state")
	}
	if state.Schema == "" {
		state.Schema = InFlightStateSchema
	}
	if state.UpdatedAt == "" {
		state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if state.ActiveGuidance == nil {
		state.ActiveGuidance = []InFlightSteering{}
	}
	path := AgentInFlightStatePath(projectRoot, agentID)
	if err := fileutil.EnsureDir(filepath.Dir(path)); err != nil {
		return errfmt.Errorf("mkdir in_flight: %w", err)
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return errfmt.Errorf("marshal in-flight state: %w", err)
	}
	return fileutil.WriteStandardFile(path, append(raw, '\n'))
}

// RecordInFlightSteering records or appends in-flight steering guidance into persistent state.
func RecordInFlightSteering(projectRoot, agentID string, item InFlightSteering) error {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return errfmt.Errorf("agent_id required to record in-flight steering")
	}
	state, err := LoadAgentInFlightState(projectRoot, agentID)
	if err != nil {
		state = AgentInFlightState{
			Schema:         InFlightStateSchema,
			AgentID:        agentID,
			ActiveGuidance: []InFlightSteering{},
		}
	}
	if item.Timestamp == "" {
		item.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	found := false
	for _, g := range state.ActiveGuidance {
		if g.EventID != "" && g.EventID == item.EventID {
			found = true
			break
		}
	}
	if !found {
		state.ActiveGuidance = append(state.ActiveGuidance, item)
	}
	state.LastEventID = item.EventID
	state.Status = InFlightStatusPending
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return SaveAgentInFlightState(projectRoot, state)
}

// ListInFlightSteering lists pending unacknowledged steering items for an agent seat.
func ListInFlightSteering(projectRoot, agentID string) ([]InFlightSteering, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, errfmt.Errorf("agent_id required to list in-flight steering")
	}
	seat := Seat{AgentID: agentID}
	snap, err := LoadCorrespondence(projectRoot, seat, 50)
	if err != nil {
		return nil, errfmt.Errorf("load correspondence: %w", err)
	}
	var res []InFlightSteering
	for _, item := range snap.InboxUnacked {
		if item.EventType == FeedEventTypeSteering {
			msg := item.Message
			if msg == "" {
				msg = item.Summary
			}
			res = append(res, InFlightSteering{
				EventID:         item.EventID,
				FromAgentID:     item.FromAgentID,
				ToAgentID:       item.ToAgentID,
				Message:         msg,
				Timestamp:       item.Timestamp,
				NonInterrupting: true,
			})
		}
	}
	return res, nil
}

// FormatInFlightGuidance formats guidance items into an in-flight prompt block.
func FormatInFlightGuidance(guidance []InFlightSteering) string {
	if len(guidance) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("### In-Flight Steering Guidance (Non-Interrupting Re-Priming)\n")
	sb.WriteString("The following operational steering directives were delivered in-flight. Incorporate this guidance into ongoing execution without interrupting or restarting running processes:\n")
	for _, item := range guidance {
		sender := item.FromAgentID
		if sender == "" {
			sender = "steering"
		}
		sb.WriteString(fmt.Sprintf("- [%s from %s]: %s\n", item.EventID, sender, item.Message))
	}
	return sb.String()
}

// RePrimeAgentContext incorporates pending in-flight steering guidance into the active agent context,
// aligns persistent state, and optionally acknowledges consumed events.
func RePrimeAgentContext(input RePrimeInput) (RePrimeResult, error) {
	root := strings.TrimSpace(input.ProjectRoot)
	if root == "" {
		return RePrimeResult{}, errfmt.Errorf("project_root required for context re-priming")
	}
	agentID := strings.TrimSpace(input.AgentID)
	if agentID == "" {
		return RePrimeResult{}, errfmt.Errorf("agent_id required for context re-priming")
	}

	steeringItems, err := ListInFlightSteering(root, agentID)
	if err != nil {
		return RePrimeResult{}, errfmt.Errorf("list in-flight steering: %w", err)
	}

	state, _ := LoadAgentInFlightState(root, agentID)
	if len(steeringItems) == 0 && len(state.ActiveGuidance) > 0 && state.Status == InFlightStatusPending {
		steeringItems = state.ActiveGuidance
	}

	if len(steeringItems) == 0 {
		return RePrimeResult{
			RePrimedContext: input.CurrentContext,
			Guidance:        nil,
			Applied:         false,
			StatePath:       AgentInFlightStatePath(root, agentID),
		}, nil
	}

	guidanceBlock := FormatInFlightGuidance(steeringItems)
	reprimed := strings.TrimSpace(input.CurrentContext)
	if reprimed != "" {
		reprimed = reprimed + "\n\n" + guidanceBlock
	} else {
		reprimed = guidanceBlock
	}

	var ackedIDs []string
	if input.AutoAck {
		persona := strings.TrimSpace(input.PersonaRef)
		for _, item := range steeringItems {
			if item.EventID != "" {
				summary := fmt.Sprintf("PEER_ACK — in-flight context re-primed for %s", item.EventID)
				if _, aerr := AppendPeerAckWithSession(root, agentID, persona, input.SessionID, item.EventID, summary); aerr == nil {
					ackedIDs = append(ackedIDs, item.EventID)
				}
			}
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	state.AgentID = agentID
	if input.PersonaRef != "" {
		state.PersonaRef = input.PersonaRef
	}
	state.LastRePrimeTime = now
	state.UpdatedAt = now
	state.Status = InFlightStatusAligned
	state.ActiveGuidance = steeringItems
	if len(steeringItems) > 0 {
		state.LastEventID = steeringItems[len(steeringItems)-1].EventID
	}
	_ = SaveAgentInFlightState(root, state)

	return RePrimeResult{
		RePrimedContext: reprimed,
		Guidance:        steeringItems,
		AckedEventIDs:   ackedIDs,
		Applied:         true,
		StatePath:       AgentInFlightStatePath(root, agentID),
	}, nil
}
