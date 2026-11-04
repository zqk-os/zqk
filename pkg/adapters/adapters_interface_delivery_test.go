package adapters_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/adapters"
	"github.com/zqk-os/zqk/pkg/adapters/antigravity"
	"github.com/zqk-os/zqk/pkg/adapters/gemini"
	"github.com/zqk-os/zqk/pkg/adapters/golang"
	"github.com/zqk-os/zqk/pkg/adapters/macos"
	"github.com/zqk-os/zqk/pkg/adapters/ollama"
	"github.com/zqk-os/zqk/pkg/adapters/openai"
	"github.com/zqk-os/zqk/pkg/adapters/qwen"
	"github.com/zqk-os/zqk/pkg/adapters/sync"
	"github.com/zqk-os/zqk/pkg/llm"
	"github.com/zqk-os/zqk/pkg/swarm"
)

// mockCompletionGate verifies CompletionGate interface adherence.
type mockCompletionGate struct {
	vendor    string
	files     []string
	verifyErr error
}

func (m *mockCompletionGate) Vendor() string { return m.vendor }

func (m *mockCompletionGate) WrittenFiles(history []swarm.ToolCallRecord, root string) []string {
	return m.files
}

func (m *mockCompletionGate) Verify(ctx context.Context, root string, history []swarm.ToolCallRecord) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if m.verifyErr != nil {
		return "verification failed", m.verifyErr
	}
	return "verification passed", nil
}

// mockWorktreeBuildCheck verifies WorktreeBuildCheck interface adherence.
type mockWorktreeBuildCheck struct {
	vendor string
	err    error
}

func (m *mockWorktreeBuildCheck) Vendor() string { return m.vendor }

func (m *mockWorktreeBuildCheck) Verify(ctx context.Context, worktreeRoot string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return m.err
}

// mockMessageDelivery verifies MessageDelivery interface adherence.
type mockMessageDelivery struct {
	vendor     string
	deliverErr error
	delivered  []string
}

func (m *mockMessageDelivery) Vendor() string { return m.vendor }

func (m *mockMessageDelivery) Deliver(ctx context.Context, msg string, msgID string, renderDetails map[string]string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if m.deliverErr != nil {
		return m.deliverErr
	}
	m.delivered = append(m.delivered, fmt.Sprintf("%s:%s", msgID, msg))
	return nil
}

func TestCompletionGateInterface(t *testing.T) {
	var gate adapters.CompletionGate = &mockCompletionGate{
		vendor: "mock-toolchain",
		files:  []string{"main.go", "main_test.go"},
	}

	if gate.Vendor() != "mock-toolchain" {
		t.Fatalf("unexpected vendor: %s", gate.Vendor())
	}

	files := gate.WrittenFiles(nil, "/workspace")
	if len(files) != 2 || files[0] != "main.go" {
		t.Fatalf("unexpected written files: %v", files)
	}

	// Normal verification
	feedback, err := gate.Verify(context.Background(), "/workspace", nil)
	if err != nil || feedback != "verification passed" {
		t.Fatalf("unexpected verify result: %v, %s", err, feedback)
	}

	// Cancelled context verification
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = gate.Verify(cancelledCtx, "/workspace", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	// Error path
	failingGate := &mockCompletionGate{
		vendor:    "failing-gate",
		verifyErr: errors.New("build error"),
	}
	feedback, err = failingGate.Verify(context.Background(), "/workspace", nil)
	if err == nil || !strings.Contains(feedback, "failed") {
		t.Fatalf("expected failure, got feedback=%q err=%v", feedback, err)
	}
}

func TestWorktreeBuildCheckInterface(t *testing.T) {
	var check adapters.WorktreeBuildCheck = &mockWorktreeBuildCheck{
		vendor: "mock-build",
	}

	if check.Vendor() != "mock-build" {
		t.Fatalf("unexpected vendor: %s", check.Vendor())
	}

	if err := check.Verify(context.Background(), "/repo"); err != nil {
		t.Fatalf("unexpected verify error: %v", err)
	}

	// Context cancellation check
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := check.Verify(cancelledCtx, "/repo"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	// Failure check
	failingCheck := &mockWorktreeBuildCheck{
		vendor: "failing-build",
		err:    errors.New("compilation failed"),
	}
	if err := failingCheck.Verify(context.Background(), "/repo"); err == nil {
		t.Fatal("expected compilation error")
	}
}

func TestMessageDeliveryInterface(t *testing.T) {
	var delivery adapters.MessageDelivery = &mockMessageDelivery{
		vendor: "test-delivery",
	}

	if delivery.Vendor() != "test-delivery" {
		t.Fatalf("unexpected vendor: %s", delivery.Vendor())
	}

	ctx := context.Background()
	if err := delivery.Deliver(ctx, "hello world", "msg-001", map[string]string{"type": "chat"}); err != nil {
		t.Fatalf("failed delivery: %v", err)
	}

	// Context cancellation
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := delivery.Deliver(cancelledCtx, "fail", "msg-002", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	// Failure check
	errDelivery := &mockMessageDelivery{
		vendor:     "err-delivery",
		deliverErr: errors.New("channel closed"),
	}
	if err := errDelivery.Deliver(ctx, "hello", "msg-003", nil); err == nil {
		t.Fatal("expected delivery error")
	}
}

func TestResolve_DeliveryMechanicsAndChannels(t *testing.T) {
	// 1. Antigravity path resolution
	tmp := t.TempDir()
	convDir := filepath.Join(tmp, "brain", "my-conv")
	transcriptPath := filepath.Join(convDir, ".system_generated", "logs", "transcript.jsonl")
	if err := os.MkdirAll(filepath.Dir(transcriptPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(transcriptPath, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	delivery := adapters.Resolve(transcriptPath)
	if delivery.Vendor() != antigravity.VendorID {
		t.Fatalf("expected %s, got %s", antigravity.VendorID, delivery.Vendor())
	}

	// Deliver message into resolved antigravity inbox
	msgID := "test-msg-123"
	err := delivery.Deliver(context.Background(), "test prompt message", msgID, map[string]string{
		"agent": "worker-1",
	})
	if err != nil {
		t.Fatalf("Deliver failed on resolved antigravity adapter: %v", err)
	}

	// Verify file was written to disk
	expectedMsgFile := filepath.Join(convDir, ".system_generated", "messages", msgID+".json")
	if _, err := os.Stat(expectedMsgFile); err != nil {
		t.Fatalf("expected message file to exist at %s: %v", expectedMsgFile, err)
	}

	// 2. Fallback resolution for non-antigravity path
	fallbackDelivery := adapters.Resolve("/some/arbitrary/path/transcript.log")
	if fallbackDelivery.Vendor() != macos.VendorID {
		t.Fatalf("expected %s fallback, got %s", macos.VendorID, fallbackDelivery.Vendor())
	}

	// Empty string fallback
	emptyFallback := adapters.Resolve("")
	if emptyFallback.Vendor() != macos.VendorID {
		t.Fatalf("expected %s fallback for empty path, got %s", macos.VendorID, emptyFallback.Vendor())
	}
}

func TestProviderSpecificAdapters_Compliance(t *testing.T) {
	// Gemini Provider Adapter
	geminiAdapter := &gemini.Adapter{}
	if geminiAdapter.ID() != "gemini" {
		t.Errorf("unexpected gemini ID: %s", geminiAdapter.ID())
	}
	if !geminiAdapter.Match("gemini", "") {
		t.Error("gemini adapter should match provider=gemini")
	}
	if !geminiAdapter.IsPublicCloud("https://generativelanguage.googleapis.com") {
		t.Error("gemini should be public cloud")
	}
	_ = geminiAdapter.Detect(&llm.Config{})
	var filledGemini llm.Config
	geminiAdapter.FillDefaults(&filledGemini)
	if filledGemini.Provider != "gemini" {
		t.Errorf("expected provider gemini, got %s", filledGemini.Provider)
	}

	// Qwen Provider Adapter
	qwenAdapter := &qwen.Adapter{}
	if qwenAdapter.ID() != "qwen" {
		t.Errorf("unexpected qwen ID: %s", qwenAdapter.ID())
	}
	if !qwenAdapter.Match("qwen", "") {
		t.Error("qwen adapter should match provider=qwen")
	}
	var filledQwen llm.Config
	qwenAdapter.FillDefaults(&filledQwen)
	if filledQwen.Provider != "qwen" {
		t.Errorf("expected provider qwen, got %s", filledQwen.Provider)
	}

	// OpenAI Provider Adapter
	openaiAdapter := &openai.Adapter{}
	if openaiAdapter.ID() != "openai" {
		t.Errorf("unexpected openai ID: %s", openaiAdapter.ID())
	}
	if !openaiAdapter.Match("openai", "") {
		t.Error("openai adapter should match provider=openai")
	}
	if !openaiAdapter.IsPublicCloud("https://api.openai.com/v1") {
		t.Error("openai should be public cloud")
	}
	_ = openaiAdapter.Detect(&llm.Config{})
	var filledOpenAI llm.Config
	openaiAdapter.FillDefaults(&filledOpenAI)
	if filledOpenAI.Provider != "openai" {
		t.Errorf("expected provider openai, got %s", filledOpenAI.Provider)
	}

	// Ollama Provider Adapter
	ollamaAdapter := &ollama.Adapter{}
	if ollamaAdapter.ID() != "ollama" {
		t.Errorf("unexpected ollama ID: %s", ollamaAdapter.ID())
	}
	if !ollamaAdapter.Match("ollama", "") {
		t.Error("ollama adapter should match provider=ollama")
	}
	_ = ollamaAdapter.Detect(&llm.Config{})
	var filledOllama llm.Config
	ollamaAdapter.FillDefaults(&filledOllama)
	if filledOllama.Provider != "ollama" {
		t.Errorf("expected provider ollama, got %s", filledOllama.Provider)
	}

	// Golang Completion Gate & Worktree
	goGate := &golang.CompletionGate{}
	if goGate.Vendor() != "golang" {
		t.Errorf("unexpected golang gate vendor: %s", goGate.Vendor())
	}
	goBuild := &golang.WorktreeBuildCheck{}
	if goBuild.Vendor() != "golang" {
		t.Errorf("unexpected golang build vendor: %s", goBuild.Vendor())
	}

	// MacOS Delivery Adapter
	macAdapter := &macos.ClipboardPasteAdapter{}
	if macAdapter.Vendor() != macos.VendorID {
		t.Errorf("unexpected macos vendor: %s", macAdapter.Vendor())
	}

	// Sync Engine
	syncEngine := sync.NewSyncEngine(nil, nil, nil)
	if syncEngine == nil {
		t.Fatal("expected non-nil SyncEngine")
	}
}

func TestStreamingAndNegativeBoundaries(t *testing.T) {
	// Test timeout and cancellation propagation in worktree build
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancelled

	tmp := t.TempDir()
	goMod := filepath.Join(tmp, "go.mod")
	_ = os.WriteFile(goMod, []byte("module example.com/testmod\n\ngo 1.24\n"), 0o600)
	mainGo := filepath.Join(tmp, "cmd", "zqk", "main.go")
	_ = os.MkdirAll(filepath.Dir(mainGo), 0o755)
	_ = os.WriteFile(mainGo, []byte("package main\nfunc main() {}\n"), 0o600)

	build := &golang.WorktreeBuildCheck{}
	err := build.Verify(ctx, tmp)
	if err == nil {
		t.Fatal("expected build error with cancelled context")
	}

	gate := &golang.CompletionGate{}

	// Test malformed payload handling
	malformedHistory := []swarm.ToolCallRecord{
		{Name: "write_file", Arguments: `{invalid-json`},
		{Name: "unknown_tool", Arguments: `{}`},
	}
	written := gate.WrittenFiles(malformedHistory, t.TempDir())
	if len(written) != 0 {
		t.Fatalf("expected 0 written files for malformed history, got %d", len(written))
	}
}
