package transceiver

import (
	"context"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/scheduler/transceiver/types"
)

// MockAdapter is a test adapter for unit testing
type MockAdapter struct {
	name        string
	sendCalled  bool
	sendError   error
	lastMessage types.Message
	lastAction  types.Action
}

func (m *MockAdapter) Name() string {
	return m.name
}

func (m *MockAdapter) Validate(action types.Action) error {
	return nil
}

//nolint:gocritic // Message passed by value in test mock
func (m *MockAdapter) Send(ctx context.Context, message types.Message, action types.Action) error {
	m.sendCalled = true
	m.lastMessage = message
	m.lastAction = action
	return m.sendError
}

func TestRouter_RegisterAdapter(t *testing.T) {
	t.Parallel()
	router := NewRouter(logging.GetLoggerFromProfile("test"))
	mockAdapter := &MockAdapter{name: "test"}

	err := router.RegisterAdapter(mockAdapter)
	if err != nil {
		t.Fatalf("Failed to register adapter: %v", err)
	}

	// Try to register duplicate
	err = router.RegisterAdapter(mockAdapter)
	if err == nil {
		t.Error("Expected error when registering duplicate adapter")
	}
}

func TestRouter_LoadRules(t *testing.T) {
	t.Parallel()
	router := NewRouter(logging.GetLoggerFromProfile("test"))

	rules := []RoutingRule{
		{Name: "rule1", Priority: 10, Enabled: true},
		{Name: "rule2", Priority: 20, Enabled: true},
		{Name: "rule3", Priority: 5, Enabled: true},
	}

	router.LoadRules(rules)

	loadedRules := router.GetRules()
	if len(loadedRules) != 3 {
		t.Fatalf("Expected 3 rules, got %d", len(loadedRules))
	}

	// Check sorting (higher priority first)
	if loadedRules[0].Priority != 20 {
		t.Errorf("Expected first rule to have priority 20, got %d", loadedRules[0].Priority)
	}
	if loadedRules[2].Priority != 5 {
		t.Errorf("Expected last rule to have priority 5, got %d", loadedRules[2].Priority)
	}
}

func TestRouter_Route(t *testing.T) {
	t.Parallel()
	router := NewRouter(logging.GetLoggerFromProfile("test"))
	mockAdapter := &MockAdapter{name: "test"}
	if err := router.RegisterAdapter(mockAdapter); err != nil {
		t.Fatalf("Failed to register adapter: %v", err)
	}

	// Create routing rule
	rules := []RoutingRule{
		{
			Name:    "test-route",
			Enabled: true,
			Match: MessageMatcher{
				EventType: "test_event",
			},
			Actions: []Action{
				{
					Protocol: "test",
					Endpoint: "test-endpoint",
				},
			},
		},
	}
	router.LoadRules(rules)

	// Route matching message
	message := types.Message{
		EventType: "test_event",
		Source:    "test",
		Timestamp: time.Now(),
		Payload:   make(map[string]any),
		Metadata:  make(map[string]string),
	}

	err := router.Route(pkgctx.NewSystemContext(), message)
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}

	if !mockAdapter.sendCalled {
		t.Error("Adapter Send was not called")
	}

	if mockAdapter.lastMessage.EventType != "test_event" {
		t.Errorf("Expected event type 'test_event', got '%s'", mockAdapter.lastMessage.EventType)
	}
}

func TestRouter_Route_NoMatch(t *testing.T) {
	t.Parallel()
	router := NewRouter(logging.GetLoggerFromProfile("test"))
	mockAdapter := &MockAdapter{name: "test"}
	if err := router.RegisterAdapter(mockAdapter); err != nil {
		t.Fatalf("Failed to register adapter: %v", err)
	}

	rules := []RoutingRule{
		{
			Name:    "test-route",
			Enabled: true,
			Match: MessageMatcher{
				EventType: "other_event",
			},
			Actions: []Action{
				{Protocol: "test", Endpoint: "test-endpoint"},
			},
		},
	}
	router.LoadRules(rules)

	// Route non-matching message
	message := types.Message{
		EventType: "test_event",
		Source:    "test",
		Timestamp: time.Now(),
		Payload:   make(map[string]any),
		Metadata:  make(map[string]string),
	}

	err := router.Route(pkgctx.NewSystemContext(), message)
	if err != nil {
		t.Fatalf("Route should not fail for no matches: %v", err)
	}

	if mockAdapter.sendCalled {
		t.Error("Adapter Send should not be called for non-matching message")
	}
}

func TestRouter_Route_DisabledRule(t *testing.T) {
	t.Parallel()
	router := NewRouter(logging.GetLoggerFromProfile("test"))
	mockAdapter := &MockAdapter{name: "test"}
	if err := router.RegisterAdapter(mockAdapter); err != nil {
		t.Fatalf("Failed to register adapter: %v", err)
	}

	rules := []RoutingRule{
		{
			Name:    "test-route",
			Enabled: false, // Disabled
			Match: MessageMatcher{
				EventType: "test_event",
			},
			Actions: []Action{
				{Protocol: "test", Endpoint: "test-endpoint"},
			},
		},
	}
	router.LoadRules(rules)

	message := types.Message{
		EventType: "test_event",
		Source:    "test",
		Timestamp: time.Now(),
		Payload:   make(map[string]any),
		Metadata:  make(map[string]string),
	}

	err := router.Route(pkgctx.NewSystemContext(), message)
	if err != nil {
		t.Fatalf("Route should not fail: %v", err)
	}

	if mockAdapter.sendCalled {
		t.Error("Adapter Send should not be called for disabled rule")
	}
}

func TestRouter_Matches(t *testing.T) {
	t.Parallel()
	router := NewRouter(logging.GetLoggerFromProfile("test"))

	tests := []struct {
		name    string
		message types.Message
		matcher MessageMatcher
		want    bool
	}{
		{
			name: "event type match",
			message: types.Message{
				EventType: "test_event",
				Metadata:  make(map[string]string),
			},
			matcher: MessageMatcher{EventType: "test_event"},
			want:    true,
		},
		{
			name: "event type mismatch",
			message: types.Message{
				EventType: "test_event",
				Metadata:  make(map[string]string),
			},
			matcher: MessageMatcher{EventType: "other_event"},
			want:    false,
		},
		{
			name: "job ID match",
			message: types.Message{
				EventType: "test_event",
				Metadata:  map[string]string{"job_id": "SCH-001"},
			},
			matcher: MessageMatcher{JobID: "SCH-001"},
			want:    true,
		},
		{
			name: "job ID mismatch",
			message: types.Message{
				EventType: "test_event",
				Metadata:  map[string]string{"job_id": "SCH-001"},
			},
			matcher: MessageMatcher{JobID: "SCH-002"},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := router.matches(tt.message, tt.matcher)
			if got != tt.want {
				t.Errorf("matches() = %v, want %v", got, tt.want)
			}
		})
	}
}
