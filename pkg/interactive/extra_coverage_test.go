package interactive

import (
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestSessionManager_FullLifecycle(t *testing.T) {
	// Custom timeout
	mgr := NewInteractiveSessionManager(50 * time.Millisecond)

	// Default timeout constructor
	defaultMgr := NewInteractiveSessionManager(0)
	if defaultMgr.timeout != 30*time.Minute {
		t.Errorf("expected default timeout 30m, got %v", defaultMgr.timeout)
	}

	// Global session manager
	globalMgr := GetGlobalInteractiveSessionManager()
	if globalMgr == nil {
		t.Fatalf("expected non-nil global session manager")
	}

	// CreateSession
	loopState := &LoopState{
		IsComplete: false,
	}
	sessionID := mgr.CreateSession("criteria", loopState)
	if sessionID == "" {
		t.Fatalf("expected non-empty session ID")
	}

	// GetSession
	sess, ok := mgr.GetSession(sessionID)
	if !ok || sess == nil {
		t.Fatalf("expected session to exist")
	}
	if sess.Kind != "criteria" {
		t.Errorf("expected kind criteria, got %s", sess.Kind)
	}

	// Non-existent session
	if _, ok := mgr.GetSession("non-existent"); ok {
		t.Errorf("expected non-existent session to return false")
	}

	// UpdateSession
	updatedLoopState := &LoopState{
		IsComplete: true,
	}
	if err := mgr.UpdateSession(sessionID, updatedLoopState); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}
	sess, _ = mgr.GetSession(sessionID)
	if !sess.LoopState.IsComplete {
		t.Errorf("expected LoopState.IsComplete to be true")
	}

	// Update non-existent session
	if err := mgr.UpdateSession("non-existent", updatedLoopState); err == nil {
		t.Errorf("expected error updating non-existent session")
	}

	// UpdateSessionWithBuilder
	if err := mgr.UpdateSessionWithBuilder(sessionID, updatedLoopState, "mock-builder", "v2"); err != nil {
		t.Fatalf("failed to update session with builder: %v", err)
	}
	sess, _ = mgr.GetSession(sessionID)
	if sess.Builder != "mock-builder" || sess.SchemaVersion != "v2" {
		t.Errorf("expected builder and schema version updated")
	}

	// UpdateSessionWithBuilder non-existent
	if err := mgr.UpdateSessionWithBuilder("non-existent", updatedLoopState, "builder", "v1"); err == nil {
		t.Errorf("expected error updating non-existent session with builder")
	}

	// CreateSessionWithBuilder
	sessID2 := mgr.CreateSessionWithBuilder("task", loopState, "bldr", "v1")
	if sessID2 == "" {
		t.Fatalf("expected non-empty session ID")
	}

	// DeleteSession
	mgr.DeleteSession(sessID2)
	if _, ok := mgr.GetSession(sessID2); ok {
		t.Errorf("expected deleted session to not exist")
	}

	// Expiration and cleanup
	time.Sleep(70 * time.Millisecond)

	// GetSession after expiry should return false
	if _, ok := mgr.GetSession(sessionID); ok {
		t.Errorf("expected expired session to return false")
	}

	// Update after expiry should fail and delete
	sessID3 := mgr.CreateSession("criteria", loopState)
	time.Sleep(70 * time.Millisecond)
	if err := mgr.UpdateSession(sessID3, loopState); err == nil {
		t.Errorf("expected error updating expired session")
	}

	// UpdateSessionWithBuilder after expiry
	sessID4 := mgr.CreateSession("criteria", loopState)
	time.Sleep(70 * time.Millisecond)
	if err := mgr.UpdateSessionWithBuilder(sessID4, loopState, nil, ""); err == nil {
		t.Errorf("expected error updating expired session with builder")
	}

	// CleanupExpiredSessions
	mgr.CreateSession("criteria", loopState)
	time.Sleep(70 * time.Millisecond)
	cleaned := mgr.CleanupExpiredSessions()
	if cleaned == 0 {
		t.Errorf("expected at least one expired session cleaned up")
	}
}

func TestTemplateGenerator_ExtraCoverage(t *testing.T) {
	fieldRegistry := objects.GetGlobalFieldRegistry()
	_ = fieldRegistry.Reload()

	generator := NewTemplateGenerator(fieldRegistry)

	// GetFieldRegistry
	if reg := generator.GetFieldRegistry(); reg != fieldRegistry {
		t.Errorf("expected field registry to match")
	}

	// GetSchemaVersion fallback
	v := generator.GetSchemaVersion("criteria")
	if v == "" {
		t.Errorf("expected non-empty schema version")
	}

	// formatValueForYAML
	if val := formatValueForYAML("hello"); val != `"hello"` {
		t.Errorf("got %s", val)
	}
	if val := formatValueForYAML("hello\nworld"); val != `"hello\nworld"` {
		t.Errorf("got %s", val)
	}
	if val := formatValueForYAML([]any{}); val != "[]" {
		t.Errorf("got %s", val)
	}
	if val := formatValueForYAML([]any{"a", "b"}); val != `["a", "b"]` {
		t.Errorf("got %s", val)
	}
	if val := formatValueForYAML(map[string]any{}); val != "{}" {
		t.Errorf("got %s", val)
	}
	if val := formatValueForYAML(map[string]any{"a": 1}); val != "{}" {
		t.Errorf("got %s", val)
	}
	if val := formatValueForYAML(true); val != "true" {
		t.Errorf("got %s", val)
	}
	if val := formatValueForYAML(false); val != "false" {
		t.Errorf("got %s", val)
	}
	if val := formatValueForYAML(42); val != "42" {
		t.Errorf("got %s", val)
	}
	if val := formatValueForYAML(nil); val != "null" {
		t.Errorf("got %s", val)
	}
	if val := formatValueForYAML(struct{}{}); val != `"{}"` {
		t.Errorf("got %s", val)
	}
}

func TestTemplateLoop_GetFieldsToElicit(t *testing.T) {
	fieldRegistry := objects.GetGlobalFieldRegistry()
	_ = fieldRegistry.Reload()

	generator := NewTemplateGenerator(fieldRegistry)
	loop := NewTemplateLoop(generator)

	loopState := &LoopState{
		MissingRequired: []string{"title"},
		InvalidFields:   map[string]string{"title": "already missing", "extra": "invalid format"},
		Template: &TemplateWithTokens{
			FieldInfo: map[string]*FieldTokenInfo{
				"title": {Name: "title", Type: "string"},
				"extra": {Name: "extra", Type: "string"},
			},
		},
	}

	fields := loop.GetFieldsToElicit(loopState)
	if len(fields) != 2 {
		t.Errorf("expected 2 fields to elicit, got %d", len(fields))
	}
}
