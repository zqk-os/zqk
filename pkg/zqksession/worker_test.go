package zqksession

import (
	"context"
	"github.com/zqk-os/zqk/pkg/config"

	"errors"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

type captureStorage struct {
	storage.ObjectStorageProvider
	created   []map[string]any
	createErr error
	readable  map[string]map[string]any
}

func (s *captureStorage) Read(_ context.Context, _ *storage.SecurityContext, id string) (map[string]any, error) {
	if obj, ok := s.readable[id]; ok {
		return obj, nil
	}
	return nil, errors.New("not found")
}

func (s *captureStorage) Create(
	_ context.Context,
	_ *storage.SecurityContext,
	obj map[string]any,
) error {
	if s.createErr != nil {
		return s.createErr
	}
	s.created = append(s.created, obj)
	return nil
}

func TestSessionConstants(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		got  string
		want string
	}{
		"CLI session":    {got: SessionTypeCLI, want: "cli"},
		"worker session": {got: SessionTypeAgentWorker, want: "agent_worker"},
		"active":         {got: StatusActive, want: "active"},
		"completed":      {got: StatusCompleted, want: "completed"},
		"error":          {got: StatusError, want: "error"},
		"agentx":         {got: ExecutorTypeAgentX, want: "agentx"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if test.got != test.want {
				t.Fatalf("got %q, want %q", test.got, test.want)
			}
		})
	}
}

func TestStartWorkerSessionRequiresAgentID(t *testing.T) {
	t.Parallel()

	sessionID, err := StartWorkerSession(
		context.Background(),
		WorkerSessionInput{ProjectRoot: t.TempDir(), AgentID: " \t "},
		&captureStorage{},
	)
	if err == nil {
		t.Fatal("expected AgentID validation error")
	}
	if sessionID != EmptyValue {
		t.Fatalf("session ID = %q, want empty", sessionID)
	}
}

func TestStartWorkerSessionCreatesFreshTypedSession(t *testing.T) {
	t.Parallel()

	sp := &captureStorage{
		readable: map[string]map[string]any{
			"ZQK-parent": {objects.FieldKeyID: "ZQK-parent"},
		},
	}
	input := WorkerSessionInput{
		ProjectRoot:        t.TempDir(),
		AccountID:          "ACC-test",
		Title:              "worker test",
		ParentSessionID:    "ZQK-parent",
		AgentID:            "agent-seat-1",
		PersonaRef:         "PER-test",
		Provider:           "cursor",
		ProviderProfileRef: "VPR-test",
		ModelID:            "test-model",
	}

	firstID, err := StartWorkerSession(context.Background(), input, sp)
	if err != nil {
		t.Fatalf("first StartWorkerSession: %v", err)
	}
	secondID, err := StartWorkerSession(context.Background(), input, sp)
	if err != nil {
		t.Fatalf("second StartWorkerSession: %v", err)
	}
	if firstID == EmptyValue || secondID == EmptyValue || firstID == secondID {
		t.Fatalf("expected fresh non-empty IDs, got %q and %q", firstID, secondID)
	}
	if len(sp.created) != 2 {
		t.Fatalf("created %d sessions, want 2", len(sp.created))
	}

	created := sp.created[0]
	expected := map[string]string{
		objects.FieldKeySessionType:        SessionTypeAgentWorker,
		objects.FieldKeyStatus:             StatusActive,
		objects.FieldKeyAgentID:            input.AgentID,
		objects.FieldKeyPersonaRef:         input.PersonaRef,
		objects.FieldKeyExecutorType:       ExecutorTypeAgentX,
		objects.FieldKeyProvider:           input.Provider,
		objects.FieldKeyProviderProfileRef: input.ProviderProfileRef,
		objects.FieldKeyModelID:            input.ModelID,
	}
	for field, want := range expected {
		if got, _ := created[field].(string); got != want {
			t.Errorf("%s = %q, want %q", field, got, want)
		}
	}
	if _, ok := created[objects.FieldKeyParentSessionRef]; ok && storage.StreamStorageEnabledForKind(objects.KindZqkSession) {
		t.Fatalf("parent_session_ref must be omitted when zqk_session is stream-backed, got %#v", created[objects.FieldKeyParentSessionRef])
	}
}

func TestStartWorkerSessionAttachesFileBackedParent(t *testing.T) {
	cfg := config.Get()
	orig := cfg.System.StreamStorageEnabled
	disabled := false
	cfg.System.StreamStorageEnabled = &disabled
	t.Cleanup(func() {
		cfg.System.StreamStorageEnabled = orig
	})

	sp := &captureStorage{
		readable: map[string]map[string]any{
			"ZQK-parent": {objects.FieldKeyID: "ZQK-parent"},
		},
	}
	id, err := StartWorkerSession(context.Background(), WorkerSessionInput{
		ProjectRoot:     t.TempDir(),
		AgentID:         "agent-seat-1",
		ParentSessionID: "ZQK-parent",
	}, sp)
	if err != nil {
		t.Fatalf("StartWorkerSession: %v", err)
	}
	if id == EmptyValue {
		t.Fatal("expected worker session id")
	}
	if got, _ := sp.created[0][objects.FieldKeyParentSessionRef].(string); got != "ZQK-parent" {
		t.Fatalf("parent_session_ref = %q, want ZQK-parent when session kind is file/CAS", got)
	}
}

func TestStartWorkerSessionOmitsStreamBackedReadableParent(t *testing.T) {
	if !storage.StreamStorageEnabledForKind(objects.KindZqkSession) {
		t.Skip("zqk_session is not stream-backed in this environment")
	}

	sp := &captureStorage{
		readable: map[string]map[string]any{
			"ZQK-stream-parent": {objects.FieldKeyID: "ZQK-stream-parent"},
		},
	}
	id, err := StartWorkerSession(context.Background(), WorkerSessionInput{
		ProjectRoot:     t.TempDir(),
		AgentID:         "agent-seat-1",
		ParentSessionID: "ZQK-stream-parent",
	}, sp)
	if err != nil {
		t.Fatalf("StartWorkerSession: %v", err)
	}
	if id == EmptyValue {
		t.Fatal("expected worker session id")
	}
	if _, ok := sp.created[0][objects.FieldKeyParentSessionRef]; ok {
		t.Fatalf("parent_session_ref should be omitted for stream-backed parents, got %#v", sp.created[0][objects.FieldKeyParentSessionRef])
	}
}

func TestStartWorkerSessionOmitsUnreadableParent(t *testing.T) {
	t.Parallel()

	sp := &captureStorage{}
	id, err := StartWorkerSession(context.Background(), WorkerSessionInput{
		ProjectRoot:     t.TempDir(),
		AgentID:         "agent-seat-1",
		ParentSessionID: "ZQK-ghost-stream",
	}, sp)
	if err != nil {
		t.Fatalf("StartWorkerSession: %v", err)
	}
	if id == EmptyValue {
		t.Fatal("expected worker session id")
	}
	if len(sp.created) != 1 {
		t.Fatalf("created %d sessions, want 1", len(sp.created))
	}
	if _, ok := sp.created[0][objects.FieldKeyParentSessionRef]; ok {
		t.Fatalf("parent_session_ref should be omitted when parent is unreadable, got %#v", sp.created[0][objects.FieldKeyParentSessionRef])
	}
}

func TestStartWorkerSessionReturnsCreateError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("create failed")
	sessionID, err := StartWorkerSession(
		context.Background(),
		WorkerSessionInput{ProjectRoot: t.TempDir(), AgentID: "agent-seat-1"},
		&captureStorage{createErr: wantErr},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want wrapped %v", err, wantErr)
	}
	if sessionID != EmptyValue {
		t.Fatalf("session ID = %q, want empty", sessionID)
	}
}

func TestSessionContext(t *testing.T) {
	t.Parallel()

	const sessionID = "ZQK-test"
	var nilContext context.Context
	ctx := WithID(nilContext, sessionID)
	if got := GetIDFromContext(ctx); got != sessionID {
		t.Fatalf("GetIDFromContext() = %q, want %q", got, sessionID)
	}
	if got := GetIDFromContext(nilContext); got != EmptyValue {
		t.Fatalf("GetIDFromContext(nil) = %q, want empty", got)
	}
}
