package feed

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestValidateSeatAndPersona(t *testing.T) {
	tests := []struct {
		name       string
		agentID    string
		personaRef string
		wantErr    bool
	}{
		{
			name:       "valid seat and persona",
			agentID:    "SEAT-001",
			personaRef: "PER-DEVELOPER",
			wantErr:    false,
		},
		{
			name:       "missing persona ref",
			agentID:    "SEAT-001",
			personaRef: "",
			wantErr:    true,
		},
		{
			name:       "missing agent id",
			agentID:    "",
			personaRef: "PER-DEVELOPER",
			wantErr:    true,
		},
		{
			name:       "agent id identical to persona ref",
			agentID:    "PER-DEVELOPER",
			personaRef: "PER-DEVELOPER",
			wantErr:    true,
		},
		{
			name:       "agent id identical to persona ref case-insensitive",
			agentID:    "per-developer",
			personaRef: "PER-DEVELOPER",
			wantErr:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSeatAndPersona(tc.agentID, tc.personaRef)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateSeatAndPersona(%q, %q) err = %v, wantErr = %v", tc.agentID, tc.personaRef, err, tc.wantErr)
			}
		})
	}
}

func TestRequireProjectRoot_NilProcessor(t *testing.T) {
	_, err := requireProjectRoot(nil)
	if err == nil {
		t.Fatal("requireProjectRoot(nil) expected error, got nil")
	}
}

func TestRequireProjectRoot_EmptyRoot(t *testing.T) {
	proc := &cli.Processor{}
	_, err := requireProjectRoot(proc)
	if err == nil {
		t.Fatal("requireProjectRoot with empty root expected error, got nil")
	}
}

func TestValidateUsablePersonaStatus(t *testing.T) {
	tests := []struct {
		name       string
		status     string
		personaRef string
		wantErr    bool
	}{
		{"active", "active", "PER-1", false},
		{"approved", "approved", "PER-1", false},
		{"implemented", "implemented", "PER-1", false},
		{"archived", "archived", "PER-1", true},
		{"error", "error", "PER-1", true},
		{"rejected", "rejected", "PER-1", true},
		{"deprecated", "deprecated", "PER-1", true},
		{"empty", "", "PER-1", true},
		{"unknown non-terminal", "draft", "PER-1", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := map[string]any{
				objects.FieldKeyStatus: tc.status,
			}
			err := validateUsablePersonaStatus(p, tc.personaRef)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateUsablePersonaStatus(%q) err = %v, wantErr = %v", tc.status, err, tc.wantErr)
			}
		})
	}
}

func TestNewPersonaFeedResult(t *testing.T) {
	cmd := &cobra.Command{}
	res := agentfeed.AppendEventResult{
		FeedID:       "feed-123",
		EventID:      "evt-456",
		DeliveryMode: "notify",
	}
	out := newPersonaFeedResult(cmd, res, "PER-TEST", "AGENT-1")
	if out[objects.FieldKeyPersonaRef] != "PER-TEST" {
		t.Fatalf("expected personaRef PER-TEST, got %v", out[objects.FieldKeyPersonaRef])
	}
	if out[objects.FieldKeyAgentID] != "AGENT-1" {
		t.Fatalf("expected agentID AGENT-1, got %v", out[objects.FieldKeyAgentID])
	}
	if out[agentfeed.JSONFieldEventID] != "evt-456" {
		t.Fatalf("expected eventID evt-456, got %v", out[agentfeed.JSONFieldEventID])
	}
}
