package primaryorch

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestResolveAdapter_Antigravity(t *testing.T) {
	t.Parallel()
	b := Binding{
		Adapter: AdapterAntigravity,
		AgentID: "peer-agent-1",
	}
	ad, err := ResolveAdapter(b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ad.Name() != AdapterAntigravity {
		t.Fatalf("ad.Name() = %q, want %q", ad.Name(), AdapterAntigravity)
	}
	if _, ok := ad.(AntigravityAdapter); !ok {
		t.Fatalf("expected AntigravityAdapter, got %T", ad)
	}
}

func TestAntigravityAdapter_WakeSuccess(t *testing.T) {
	root := t.TempDir()
	logPath := filepath.Join(root, "mock_agentapi.log")

	t.Setenv("ANTIGRAVITY_LS_ADDRESS", "")
	t.Setenv("ANTIGRAVITY_CSRF_TOKEN", "")
	t.Setenv("ANTIGRAVITY_CONVERSATION_ID", "")
	t.Setenv("AGY_CONVERSATION_ID", "")
	t.Setenv("ANTIGRAVITY_PID", "")

	// Create mock agentapi executable
	mockScript := filepath.Join(root, "mock_agentapi.sh")
	scriptContent := "#!/bin/sh\n" +
		"echo \"ARGS: $@\" >> \"" + logPath + "\"\n" +
		"echo \"ANTIGRAVITY_LS_ADDRESS=$ANTIGRAVITY_LS_ADDRESS\" >> \"" + logPath + "\"\n" +
		"echo \"ANTIGRAVITY_CSRF_TOKEN=$ANTIGRAVITY_CSRF_TOKEN\" >> \"" + logPath + "\"\n" +
		"echo \"ANTIGRAVITY_CONVERSATION_ID=$ANTIGRAVITY_CONVERSATION_ID\" >> \"" + logPath + "\"\n" +
		"echo \"ANTIGRAVITY_PID=$ANTIGRAVITY_PID\" >> \"" + logPath + "\"\n" +
		"echo '{\"status\": \"ok\"}'\n"
	if err := fileutil.WriteFile(mockScript, []byte(scriptContent), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	b := Binding{
		AgentID:        "peer-agent-1",
		Adapter:        AdapterAntigravity,
		LSAddress:      "127.0.0.1:51753",
		CSRFToken:      "csrf-token-12345",
		ConversationID: "conv-1111-2222",
		PID:            4242,
	}

	adapter := AntigravityAdapter{
		AgentAPIBin: mockScript,
	}

	req := WakeRequest{
		TaskID:  "ATK-100",
		Persona: "coder",
		Message: "please continue working on task",
	}

	res, err := adapter.Wake(context.Background(), root, b, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.AgentID != "peer-agent-1" {
		t.Errorf("res.AgentID = %q, want peer-agent-1", res.AgentID)
	}
	if res.Adapter != AdapterAntigravity {
		t.Errorf("res.Adapter = %q, want %q", res.Adapter, AdapterAntigravity)
	}
	if res.DeliveredTo != "antigravity:conv-1111-2222" {
		t.Errorf("res.DeliveredTo = %q, want antigravity:conv-1111-2222", res.DeliveredTo)
	}

	rawLog, err := fileutil.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read mock log: %v", err)
	}
	logStr := string(rawLog)

	if !strings.Contains(logStr, "ARGS: send-message conv-1111-2222 please continue working on task") {
		t.Errorf("mock log missing expected args: %s", logStr)
	}
	if !strings.Contains(logStr, "ANTIGRAVITY_LS_ADDRESS=127.0.0.1:51753") {
		t.Errorf("mock log missing LS address: %s", logStr)
	}
	if !strings.Contains(logStr, "ANTIGRAVITY_CSRF_TOKEN=csrf-token-12345") {
		t.Errorf("mock log missing CSRF token: %s", logStr)
	}
	if !strings.Contains(logStr, "ANTIGRAVITY_CONVERSATION_ID=conv-1111-2222") {
		t.Errorf("mock log missing conversation ID: %s", logStr)
	}
	if !strings.Contains(logStr, "ANTIGRAVITY_PID=4242") {
		t.Errorf("mock log missing PID: %s", logStr)
	}
}

func TestAntigravityAdapter_EnvironmentPrecedence(t *testing.T) {
	root := t.TempDir()
	logPath := filepath.Join(root, "mock_env.log")

	mockScript := filepath.Join(root, "mock_env.sh")
	scriptContent := "#!/bin/sh\n" +
		"echo \"ANTIGRAVITY_LS_ADDRESS=$ANTIGRAVITY_LS_ADDRESS\" >> \"" + logPath + "\"\n" +
		"echo \"ANTIGRAVITY_CSRF_TOKEN=$ANTIGRAVITY_CSRF_TOKEN\" >> \"" + logPath + "\"\n" +
		"echo \"ANTIGRAVITY_CONVERSATION_ID=$ANTIGRAVITY_CONVERSATION_ID\" >> \"" + logPath + "\"\n"
	if err := fileutil.WriteFile(mockScript, []byte(scriptContent), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("ANTIGRAVITY_LS_ADDRESS", "env-host:9999")
	t.Setenv("ANTIGRAVITY_CSRF_TOKEN", "env-csrf-token-xyz")
	t.Setenv("ANTIGRAVITY_CONVERSATION_ID", "env-conv-999")

	b := Binding{
		AgentID:        "peer-agent-1",
		Adapter:        AdapterAntigravity,
		LSAddress:      "binding-host:1111",
		CSRFToken:      "binding-csrf-token",
		ConversationID: "binding-conv-111",
	}

	adapter := AntigravityAdapter{
		AgentAPIBin: mockScript,
	}

	_, err := adapter.Wake(context.Background(), root, b, WakeRequest{Message: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rawLog, err := fileutil.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	logStr := string(rawLog)

	if !strings.Contains(logStr, "ANTIGRAVITY_LS_ADDRESS=env-host:9999") {
		t.Errorf("expected env LS address to take precedence: %s", logStr)
	}
	if !strings.Contains(logStr, "ANTIGRAVITY_CSRF_TOKEN=env-csrf-token-xyz") {
		t.Errorf("expected env CSRF token to take precedence: %s", logStr)
	}
	if !strings.Contains(logStr, "ANTIGRAVITY_CONVERSATION_ID=env-conv-999") {
		t.Errorf("expected env conversation ID to take precedence: %s", logStr)
	}
}

func TestAntigravityAdapter_PeerSeatsFallback(t *testing.T) {
	root := t.TempDir()
	logPath := filepath.Join(root, "mock_peer.log")

	mockScript := filepath.Join(root, "mock_peer.sh")
	scriptContent := "#!/bin/sh\n" +
		"echo \"ARGS: $@\" >> \"" + logPath + "\"\n" +
		"echo \"ANTIGRAVITY_PID=$ANTIGRAVITY_PID\" >> \"" + logPath + "\"\n"
	if err := fileutil.WriteFile(mockScript, []byte(scriptContent), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("ANTIGRAVITY_LS_ADDRESS", "127.0.0.1:51753")
	t.Setenv("ANTIGRAVITY_CSRF_TOKEN", "csrf-val")
	t.Setenv("ANTIGRAVITY_CONVERSATION_ID", "")
	t.Setenv("AGY_CONVERSATION_ID", "")

	// Write .zqk/state/mesh/peer_seats.json
	peerSeatsDir := filepath.Join(root, paths.ProjectDataDir, paths.StateDir, paths.MeshStateSubdir)
	if err := fileutil.MkdirAll(peerSeatsDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	seatsData := map[string]any{
		"schema_version": "1",
		"seats": map[string]any{
			"peer-agent-1": map[string]any{
				"pid":          9876,
				"conversation": "conv-from-peer-seats",
				"wake":         "mcp",
			},
		},
	}
	rawSeats, err := json.Marshal(seatsData)
	if err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(paths.PeerSeatsPath(root), rawSeats, paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	b := Binding{
		AgentID: "peer-agent-1",
		Adapter: AdapterAntigravity,
	}

	adapter := AntigravityAdapter{
		AgentAPIBin: mockScript,
	}

	res, err := adapter.Wake(context.Background(), root, b, WakeRequest{Message: "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.DeliveredTo != "antigravity:conv-from-peer-seats" {
		t.Errorf("DeliveredTo = %q, want antigravity:conv-from-peer-seats", res.DeliveredTo)
	}

	rawLog, err := fileutil.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	logStr := string(rawLog)
	if !strings.Contains(logStr, "ARGS: send-message conv-from-peer-seats hello") {
		t.Errorf("unexpected args in log: %s", logStr)
	}
	if !strings.Contains(logStr, "ANTIGRAVITY_PID=9876") {
		t.Errorf("expected PID 9876 from peer seats, got log: %s", logStr)
	}
}

func TestAntigravityAdapter_MissingCredentials(t *testing.T) {
	root := t.TempDir()

	t.Run("missing_ls_address", func(t *testing.T) {
		t.Setenv("ANTIGRAVITY_LS_ADDRESS", "")
		t.Setenv("ANTIGRAVITY_CSRF_TOKEN", "some-csrf")
		t.Setenv("ANTIGRAVITY_CONVERSATION_ID", "some-conv")

		adapter := AntigravityAdapter{AgentAPIBin: "/bin/echo"}
		_, err := adapter.Wake(context.Background(), root, Binding{AgentID: "p1"}, WakeRequest{Message: "test"})
		if err == nil || !strings.Contains(err.Error(), "missing LS address") {
			t.Fatalf("expected missing LS address error, got: %v", err)
		}
	})

	t.Run("missing_csrf_token", func(t *testing.T) {
		t.Setenv("ANTIGRAVITY_LS_ADDRESS", "localhost:51753")
		t.Setenv("ANTIGRAVITY_CSRF_TOKEN", "")
		t.Setenv("ANTIGRAVITY_CONVERSATION_ID", "some-conv")

		adapter := AntigravityAdapter{AgentAPIBin: "/bin/echo"}
		_, err := adapter.Wake(context.Background(), root, Binding{AgentID: "p1"}, WakeRequest{Message: "test"})
		if err == nil || !strings.Contains(err.Error(), "missing CSRF token") {
			t.Fatalf("expected missing CSRF token error, got: %v", err)
		}
	})

	t.Run("missing_conversation_id", func(t *testing.T) {
		t.Setenv("ANTIGRAVITY_LS_ADDRESS", "localhost:51753")
		t.Setenv("ANTIGRAVITY_CSRF_TOKEN", "some-csrf")
		t.Setenv("ANTIGRAVITY_CONVERSATION_ID", "")
		t.Setenv("AGY_CONVERSATION_ID", "")

		adapter := AntigravityAdapter{AgentAPIBin: "/bin/echo"}
		_, err := adapter.Wake(context.Background(), root, Binding{AgentID: "p1"}, WakeRequest{Message: "test"})
		if err == nil || !strings.Contains(err.Error(), "missing conversation ID") {
			t.Fatalf("expected missing conversation ID error, got: %v", err)
		}
	})
}

func TestAntigravityAdapter_MissingAgentAPI(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ANTIGRAVITY_LS_ADDRESS", "localhost:51753")
	t.Setenv("ANTIGRAVITY_CSRF_TOKEN", "csrf-token")
	t.Setenv("ANTIGRAVITY_CONVERSATION_ID", "conv-id")
	t.Setenv("AGENTAPI", "/nonexistent/path/to/agentapi_binary_xyz")
	t.Setenv("ANTIGRAVITY_AGENTAPI_EXE", "/nonexistent/path/to/agy_binary_xyz")
	t.Setenv("PATH", t.TempDir()) // empty PATH
	t.Setenv("HOME", t.TempDir()) // empty HOME without .gemini

	adapter := AntigravityAdapter{}
	_, err := adapter.Wake(context.Background(), root, Binding{AgentID: "p1"}, WakeRequest{Message: "test"})
	if err == nil || !strings.Contains(err.Error(), "agentapi executable not found") {
		t.Fatalf("expected agentapi executable not found error, got: %v", err)
	}
}

func TestAntigravityAdapter_AgentAPIError(t *testing.T) {
	root := t.TempDir()

	mockScript := filepath.Join(root, "mock_err.sh")
	scriptContent := "#!/bin/sh\n" +
		"echo 'rpc error: code = Unauthenticated desc = unauthenticated: missing CSRF token' >&2\n" +
		"exit 1\n"
	if err := fileutil.WriteFile(mockScript, []byte(scriptContent), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	b := Binding{
		AgentID:        "peer-agent-1",
		Adapter:        AdapterAntigravity,
		LSAddress:      "127.0.0.1:51753",
		CSRFToken:      "csrf-val",
		ConversationID: "conv-val",
	}

	adapter := AntigravityAdapter{
		AgentAPIBin: mockScript,
	}

	_, err := adapter.Wake(context.Background(), root, b, WakeRequest{Message: "test"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unauthenticated: missing CSRF token") {
		t.Fatalf("expected unauthenticated error wrapped, got: %v", err)
	}
}
