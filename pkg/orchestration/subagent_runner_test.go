package orchestration

import (
	"context"
	"errors"
	"testing"

	"github.com/zqk-os/zqk/pkg/rollback"
)

// MockCAPLoop implements CAPLoop for testing
type MockCAPLoop struct {
	Alerts []CAPAlert
}

func (m *MockCAPLoop) Alert(ctx context.Context, alert CAPAlert) error {
	m.Alerts = append(m.Alerts, alert)
	return nil
}

// MockObjectStorageProvider implements storage.ObjectStorageProvider for testing
type MockObjectStorageProvider struct{}

func (m *MockObjectStorageProvider) ReadObject(ctx context.Context, id string) (map[string]interface{}, error) {
	return nil, nil
}
func (m *MockObjectStorageProvider) WriteObject(ctx context.Context, id string, obj map[string]interface{}) error {
	return nil
}
func (m *MockObjectStorageProvider) ListObjects(ctx context.Context, kind string) ([]map[string]interface{}, error) {
	return nil, nil
}
func (m *MockObjectStorageProvider) DeleteObject(ctx context.Context, id string) error {
	return nil
}
func (m *MockObjectStorageProvider) GetKind(ctx context.Context, kind string) (map[string]interface{}, error) {
	return nil, nil
}

func TestSubagentRunner_RunSubagentTask_Success(t *testing.T) {
	cfg := SentinelConfig{MaxRetries: 3}
	mockCap := &MockCAPLoop{}
	runner := NewSubagentRunner(cfg, "/tmp/mock", nil, mockCap)

	taskCalled := 0
	task := func(ctx context.Context) error {
		taskCalled++
		return nil
	}

	getStates := func() ([]rollback.ObjectState, error) {
		return nil, nil
	}

	err := runner.RunSubagentTask(context.Background(), "subagent-1", task, getStates)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if taskCalled != 1 {
		t.Fatalf("expected task to be called 1 time, got %d", taskCalled)
	}
	if len(mockCap.Alerts) != 0 {
		t.Fatalf("expected 0 alerts, got %d", len(mockCap.Alerts))
	}
}

func TestSubagentRunner_RunSubagentTask_RetriesAndFails(t *testing.T) {
	cfg := SentinelConfig{MaxRetries: 2}
	mockCap := &MockCAPLoop{}
	runner := NewSubagentRunner(cfg, "/tmp/mock", nil, mockCap)

	taskCalled := 0
	expectedErr := errors.New("task failed")
	task := func(ctx context.Context) error {
		taskCalled++
		return expectedErr
	}

	getStates := func() ([]rollback.ObjectState, error) {
		return nil, nil
	}

	err := runner.RunSubagentTask(context.Background(), "subagent-1", task, getStates)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Should try 1 (initial) + 2 (retries) = 3 times
	if taskCalled != 3 {
		t.Fatalf("expected task to be called 3 times, got %d", taskCalled)
	}

	// Should have sent exactly 1 CAP alert
	if len(mockCap.Alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(mockCap.Alerts))
	}
	if mockCap.Alerts[0].SubagentID != "subagent-1" {
		t.Errorf("expected alert for subagent-1, got %s", mockCap.Alerts[0].SubagentID)
	}
	if len(mockCap.Alerts[0].Violations) == 0 {
		t.Fatal("expected violations in alert, got 0")
	}
}

func TestSubagentRunner_RunSubagentTask_RetriesAndSucceeds(t *testing.T) {
	cfg := SentinelConfig{MaxRetries: 2}
	mockCap := &MockCAPLoop{}
	runner := NewSubagentRunner(cfg, "/tmp/mock", nil, mockCap)

	taskCalled := 0
	task := func(ctx context.Context) error {
		taskCalled++
		if taskCalled < 2 {
			return errors.New("temporary failure")
		}
		return nil
	}

	getStates := func() ([]rollback.ObjectState, error) {
		return nil, nil
	}

	err := runner.RunSubagentTask(context.Background(), "subagent-1", task, getStates)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Should try 1 (initial) + 1 (retry) = 2 times
	if taskCalled != 2 {
		t.Fatalf("expected task to be called 2 times, got %d", taskCalled)
	}
	if len(mockCap.Alerts) != 0 {
		t.Fatalf("expected 0 alerts, got %d", len(mockCap.Alerts))
	}
}
