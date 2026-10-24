package agentdelivery

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/federation"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/infrastructure/crypto"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/policy"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type recordingDeliverer struct {
	name   string
	result Result
	err    error
	called bool
	prompt Prompt
}

func (r *recordingDeliverer) Name() string {
	if r.name == "" {
		return "recording_deliverer"
	}
	return r.name
}

func (r *recordingDeliverer) Deliver(ctx context.Context, p Prompt) (Result, error) {
	r.called = true
	r.prompt = p
	if r.err != nil {
		return r.result, r.err
	}
	return r.result, nil
}

type mockFederationTransport struct {
	executeErr   error
	executeRes   json.RawMessage
	lastEndpoint string
	lastToolName string
	lastArgs     map[string]any
}

func (m *mockFederationTransport) SendHandshake(_ context.Context, _ string, _ federation.HandshakeRequest) (*federation.HandshakeResponse, error) {
	return nil, nil
}

func (m *mockFederationTransport) SendHeartbeat(_ context.Context, _ string, _ string) error {
	return nil
}

func (m *mockFederationTransport) ExecuteTool(_ context.Context, endpoint string, toolName string, arguments map[string]any) (json.RawMessage, error) {
	m.lastEndpoint = endpoint
	m.lastToolName = toolName
	m.lastArgs = arguments
	if m.executeErr != nil {
		return nil, m.executeErr
	}
	return m.executeRes, nil
}

func createTestSignedTDE(t *testing.T) *policy.TrustDomainEnvelope {
	t.Helper()
	signer, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatalf("GenerateKeypair failed: %v", err)
	}

	tde := &policy.TrustDomainEnvelope{
		ID:                "TDE-CYCLE163-MOCK",
		Scope:             "agent_orchestration",
		AllowedNamespaces: []string{"zqk:kernel"},
		MaxRuntimeSeconds: 300,
		MCPPermissions:    []string{"tools:execute"},
		Neurological: &policy.NeurologicalGovernance{
			Confidence:     0.98,
			DecisionBranch: "validation_branch",
		},
		Payload: map[string]string{"action": "deliver"},
	}

	if err := tde.Sign(signer); err != nil {
		t.Fatalf("Sign failed: %v", err)
	}
	if err := tde.Verify(); err != nil {
		t.Fatalf("Verify failed on newly signed envelope: %v", err)
	}
	return tde
}

// -------------------------------------------------------------------------
// Composite & Pipeline Deliverer Tests
// -------------------------------------------------------------------------

func TestCycle163_Composite_FailureHaltsSubsequentSteps(t *testing.T) {
	t.Parallel()
	step1 := &recordingDeliverer{name: "step-1", result: Result{DeliveredTo: "dest1"}}
	step2 := &recordingDeliverer{name: "step-2", err: errors.New("simulated pipeline error")}
	step3 := &recordingDeliverer{name: "step-3", result: Result{DeliveredTo: "dest3"}}

	composite := Composite{Steps: []Deliverer{step1, step2, step3}}
	res, err := composite.Deliver(context.Background(), Prompt{Markdown: []byte("pipeline test payload")})

	if err == nil {
		t.Fatal("expected composite to return error when step2 fails, got nil")
	}
	if !strings.Contains(err.Error(), "agentdelivery: step step-2") {
		t.Fatalf("expected step name in error message, got: %v", err)
	}
	if !step1.called {
		t.Fatal("expected step1 to be called")
	}
	if !step2.called {
		t.Fatal("expected step2 to be called")
	}
	if step3.called {
		t.Fatal("expected step3 NOT to be called after earlier step failure")
	}
	if res.DeliveredTo != "dest1" {
		t.Fatalf("expected last successful result from step1, got: %s", res.DeliveredTo)
	}
}

func TestCycle163_Composite_EmptyStepsReturnsZeroResult(t *testing.T) {
	t.Parallel()
	composite := Composite{Steps: []Deliverer{}}
	res, err := composite.Deliver(context.Background(), Prompt{Markdown: []byte("test")})
	if err != nil {
		t.Fatalf("expected nil error for empty composite, got: %v", err)
	}
	if res.DeliveredTo != "" || res.HTTPStatusCode != 0 {
		t.Fatalf("expected zero Result for empty composite, got: %+v", res)
	}
}

// -------------------------------------------------------------------------
// HTTP Deliverer Boundary & Status Code Translation Tests
// -------------------------------------------------------------------------

func TestCycle163_HTTPDeliverer_ConnectionFailure(t *testing.T) {
	t.Parallel()
	// Target an unbound port where connection will be refused
	d := HTTPDeliverer{
		URL:    "http://127.0.0.1:59998",
		Client: &http.Client{Timeout: 100 * time.Millisecond},
	}

	_, err := d.Deliver(context.Background(), Prompt{Markdown: []byte("payload")})
	if err == nil {
		t.Fatal("expected connection error, got nil")
	}
	if !strings.Contains(err.Error(), "agentdelivery: http post") {
		t.Fatalf("expected wrapped http post error, got: %v", err)
	}
}

func TestCycle163_HTTPDeliverer_StatusTranslations(t *testing.T) {
	t.Parallel()
	cases := []struct {
		statusCode  int
		expectError bool
	}{
		{http.StatusOK, false},
		{http.StatusCreated, false},
		{http.StatusAccepted, false},
		{http.StatusNoContent, false},
		{http.StatusBadRequest, true},
		{http.StatusUnauthorized, true},
		{http.StatusForbidden, true},
		{http.StatusNotFound, true},
		{http.StatusInternalServerError, true},
		{http.StatusServiceUnavailable, true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(http.StatusText(tc.statusCode), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.statusCode)
			}))
			t.Cleanup(srv.Close)

			deliverer := HTTPDeliverer{URL: srv.URL}
			res, err := deliverer.Deliver(context.Background(), Prompt{
				Markdown:  []byte("test-content"),
				SessionID: "sess-123",
			})

			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error for status %d, got nil", tc.statusCode)
				}
				if !strings.Contains(err.Error(), "agentdelivery: http status") {
					t.Fatalf("expected agentdelivery status error, got: %v", err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error for status %d: %v", tc.statusCode, err)
				}
			}

			if res.HTTPStatusCode != tc.statusCode {
				t.Fatalf("expected HTTPStatusCode %d, got %d", tc.statusCode, res.HTTPStatusCode)
			}
			if !strings.HasPrefix(res.DeliveredTo, "http:") {
				t.Fatalf("expected DeliveredTo to have http prefix, got: %s", res.DeliveredTo)
			}
		})
	}
}

func TestCycle163_HTTPDeliverer_URLValidationBoundaries(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name        string
		url         string
		expectedSub string
	}{
		{"control character in URL", "http://example.com/\x7f", "invalid URL"},
		{"missing host in http", "http://", "URL missing host"},
		{"missing host in https", "https://", "URL missing host"},
		{"custom scheme gopher", "gopher://example.com", "unsupported scheme"},
		{"custom scheme ws", "ws://example.com/socket", "unsupported scheme"},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			d := HTTPDeliverer{URL: tc.url}
			_, err := d.Deliver(context.Background(), Prompt{Markdown: []byte("body")})
			if err == nil {
				t.Fatalf("expected error for url %q, got nil", tc.url)
			}
			if !strings.Contains(err.Error(), tc.expectedSub) {
				t.Fatalf("expected error containing %q, got: %v", tc.expectedSub, err)
			}
		})
	}
}

func TestCycle163_HTTPDeliverer_SharedAPISpecClientSingleton(t *testing.T) {
	t.Parallel()
	client1 := sharedAPISpecHTTPClient()
	client2 := sharedAPISpecHTTPClient()
	if client1 == nil || client2 == nil {
		t.Fatal("expected shared client not to be nil")
	}
	if client1 != client2 {
		t.Fatal("expected sharedAPISpecHTTPClient to return the identical singleton instance")
	}

	resolved := httpClientOrDefault(nil)
	if resolved != client1 {
		t.Fatal("expected httpClientOrDefault(nil) to return sharedAPISpecHTTPClient singleton")
	}

	custom := &http.Client{}
	if httpClientOrDefault(custom) != custom {
		t.Fatal("expected httpClientOrDefault with non-nil client to return custom client")
	}
}

func TestCycle163_HTTPDeliverer_HeadersAndPayloadStructure(t *testing.T) {
	t.Parallel()
	var capturedContentType string
	var capturedAuth string
	var capturedBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedContentType = r.Header.Get("Content-Type")
		capturedAuth = r.Header.Get("Authorization")
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		capturedBody = bodyBytes
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	d := HTTPDeliverer{
		URL:         srv.URL,
		BearerToken: "sec-token-xyz-123",
	}

	prompt := Prompt{
		Markdown:  []byte("# Architectural Review\nEnsure all invariants hold."),
		SessionID: "session-abc-456",
		Format:    "agent-prompt",
	}

	res, err := d.Deliver(context.Background(), prompt)
	if err != nil {
		t.Fatalf("Deliver failed: %v", err)
	}

	if res.HTTPStatusCode != http.StatusOK {
		t.Fatalf("expected 200, got: %d", res.HTTPStatusCode)
	}
	if capturedContentType != "application/json" {
		t.Fatalf("expected application/json Content-Type, got: %s", capturedContentType)
	}
	if capturedAuth != "Bearer sec-token-xyz-123" {
		t.Fatalf("expected Bearer Authorization header, got: %s", capturedAuth)
	}

	var payload httpDeliveryPayload
	if err := json.Unmarshal(capturedBody, &payload); err != nil {
		t.Fatalf("failed to unmarshal delivery payload: %v", err)
	}

	if payload.Format != "agent-prompt" {
		t.Fatalf("expected format agent-prompt, got: %s", payload.Format)
	}
	if payload.ConvergenceSessionID != "session-abc-456" {
		t.Fatalf("expected session ID session-abc-456, got: %s", payload.ConvergenceSessionID)
	}
	if payload.Markdown != "# Architectural Review\nEnsure all invariants hold." {
		t.Fatalf("expected exact markdown context, got: %s", payload.Markdown)
	}
}

// -------------------------------------------------------------------------
// MCP Deliverer Transport & Remote Tool Invocation Tests
// -------------------------------------------------------------------------

func TestCycle163_MCPDeliverer_SuccessLifecycle(t *testing.T) {
	t.Parallel()
	mockTransport := &mockFederationTransport{
		executeRes: json.RawMessage(`{"status":"executed","receipt":"rcpt-001"}`),
	}

	deliverer := NewMCPDeliverer(mockTransport, "mesh.kernel.local:9091", "agent_dispatch_tool")

	if deliverer.Name() != "mcp" {
		t.Fatalf("expected name 'mcp', got: %s", deliverer.Name())
	}

	prompt := Prompt{
		Markdown:  []byte("## Context-Bound Task\nExecute bounded migration"),
		SessionID: "convergence-987",
		Format:    "agent-prompt",
	}

	res, err := deliverer.Deliver(context.Background(), prompt)
	if err != nil {
		t.Fatalf("unexpected Deliver error: %v", err)
	}

	if res.HTTPStatusCode != 200 {
		t.Fatalf("expected HTTPStatusCode 200, got: %d", res.HTTPStatusCode)
	}
	if res.DeliveredTo != "mcp://mesh.kernel.local:9091/agent_dispatch_tool" {
		t.Fatalf("unexpected DeliveredTo: %s", res.DeliveredTo)
	}

	if mockTransport.lastEndpoint != "mesh.kernel.local:9091" {
		t.Fatalf("expected endpoint mesh.kernel.local:9091, got: %s", mockTransport.lastEndpoint)
	}
	if mockTransport.lastToolName != "agent_dispatch_tool" {
		t.Fatalf("expected tool agent_dispatch_tool, got: %s", mockTransport.lastToolName)
	}
	if mockTransport.lastArgs["prompt"] != "## Context-Bound Task\nExecute bounded migration" {
		t.Fatalf("prompt argument mismatch: %v", mockTransport.lastArgs["prompt"])
	}
	if mockTransport.lastArgs[objects.FieldKeySessionID] != "convergence-987" {
		t.Fatalf("session_id argument mismatch: %v", mockTransport.lastArgs[objects.FieldKeySessionID])
	}
	if mockTransport.lastArgs[objects.FieldKeyFormat] != "agent-prompt" {
		t.Fatalf("format argument mismatch: %v", mockTransport.lastArgs[objects.FieldKeyFormat])
	}
}

func TestCycle163_MCPDeliverer_TransportExecutionFailure(t *testing.T) {
	t.Parallel()
	mockTransport := &mockFederationTransport{
		executeErr: errors.New("remote RPC socket disconnected"),
	}

	deliverer := NewMCPDeliverer(mockTransport, "remote-peer:8080", "remote_tool")
	_, err := deliverer.Deliver(context.Background(), Prompt{Markdown: []byte("work")})
	if err == nil {
		t.Fatal("expected error on transport failure, got nil")
	}
	if !strings.Contains(err.Error(), "remote execution failed") {
		t.Fatalf("expected wrapped 'remote execution failed' error, got: %v", err)
	}
}

func TestCycle163_MCPDeliverer_NilTransportError(t *testing.T) {
	t.Parallel()
	deliverer := NewMCPDeliverer(nil, "remote-peer:8080", "remote_tool")
	_, err := deliverer.Deliver(context.Background(), Prompt{Markdown: []byte("test")})
	if err == nil {
		t.Fatal("expected error when transport is nil, got nil")
	}
	if !strings.Contains(err.Error(), "mcp deliverer requires a transport") {
		t.Fatalf("expected transport required error, got: %v", err)
	}
}

// -------------------------------------------------------------------------
// Subagent Deliverer & Adapter Invocations
// -------------------------------------------------------------------------

func TestCycle163_SubagentDeliverer_NamesAndInvokerFunc(t *testing.T) {
	t.Parallel()
	// Test SubagentInvokerFunc Name
	fn := SubagentInvokerFunc(func(_ context.Context, _ SubagentInvocation) (Result, error) {
		return Result{DeliveredTo: "ok"}, nil
	})
	if fn.Name() != "subagent-func" {
		t.Fatalf("expected 'subagent-func', got: %s", fn.Name())
	}

	// Test SubagentDeliverer with nil Invoker
	nilDeliverer := &SubagentDeliverer{}
	if nilDeliverer.Name() != "subagent" {
		t.Fatalf("expected 'subagent' when invoker is nil, got: %s", nilDeliverer.Name())
	}

	// Test SubagentDeliverer with functional invoker
	funcDeliverer := NewSubagentDeliverer(fn)
	if funcDeliverer.Name() != "subagent:subagent-func" {
		t.Fatalf("expected 'subagent:subagent-func', got: %s", funcDeliverer.Name())
	}
}

func TestCycle163_SubagentDeliverer_NilInvokerDeliverError(t *testing.T) {
	t.Parallel()
	deliverer := &SubagentDeliverer{Invoker: nil}
	_, err := deliverer.Deliver(context.Background(), Prompt{Markdown: []byte("prompt text")})
	if err == nil {
		t.Fatal("expected error for nil invoker, got nil")
	}
	if !strings.Contains(err.Error(), "invoker adapter is not configured") {
		t.Fatalf("expected not configured error, got: %v", err)
	}
}

func TestCycle163_SubagentDeliverer_WhitespaceMarkdownParityViolation(t *testing.T) {
	t.Parallel()
	invoker := SubagentInvokerFunc(func(_ context.Context, _ SubagentInvocation) (Result, error) {
		return Result{DeliveredTo: "done"}, nil
	})
	deliverer := NewSubagentDeliverer(invoker)

	// Test whitespace-only markdown
	_, err := deliverer.Deliver(context.Background(), Prompt{Markdown: []byte("  \t\n  \r\n  ")})
	if err == nil {
		t.Fatal("expected context parity violation on whitespace-only markdown, got nil")
	}
	if !strings.Contains(err.Error(), "context parity violation: prompt markdown cannot be empty") {
		t.Fatalf("expected context parity violation, got: %v", err)
	}
}

func TestCycle163_SubagentDeliverer_ConfiguredDefaultsAndInvocationMapping(t *testing.T) {
	t.Parallel()
	var captured SubagentInvocation

	invoker := SubagentInvokerFunc(func(_ context.Context, invocation SubagentInvocation) (Result, error) {
		captured = invocation
		return Result{DeliveredTo: "custom:subagent", HTTPStatusCode: 200}, nil
	})

	deliverer := &SubagentDeliverer{
		Invoker:          invoker,
		DefaultTypeName:  "craftsman",
		DefaultRole:      "Principal Systems Architect",
		DefaultModel:     "gemini-2.5-pro",
		DefaultWorkspace: "branch",
	}

	prompt := Prompt{
		Markdown:  []byte("# Spec Verification\nExecute formal verification suite."),
		SessionID: "sess-cycle-163",
	}

	res, err := deliverer.Deliver(context.Background(), prompt)
	if err != nil {
		t.Fatalf("unexpected Deliver error: %v", err)
	}

	if res.DeliveredTo != "custom:subagent" {
		t.Fatalf("expected custom:subagent, got: %s", res.DeliveredTo)
	}
	if captured.TypeName != "craftsman" {
		t.Fatalf("expected typeName 'craftsman', got: %s", captured.TypeName)
	}
	if captured.Role != "Principal Systems Architect" {
		t.Fatalf("expected role 'Principal Systems Architect', got: %s", captured.Role)
	}
	if captured.Model != "gemini-2.5-pro" {
		t.Fatalf("expected model 'gemini-2.5-pro', got: %s", captured.Model)
	}
	if captured.Workspace != "branch" {
		t.Fatalf("expected workspace 'branch', got: %s", captured.Workspace)
	}
	if captured.SessionID != "sess-cycle-163" {
		t.Fatalf("expected sessionID 'sess-cycle-163', got: %s", captured.SessionID)
	}
	if captured.Prompt != "# Spec Verification\nExecute formal verification suite." {
		t.Fatalf("unexpected prompt: %s", captured.Prompt)
	}
}

func TestCycle163_SubagentDeliverer_InvokerErrorPropagation(t *testing.T) {
	t.Parallel()
	expectedErr := errors.New("subagent dispatch failed")
	invoker := SubagentInvokerFunc(func(_ context.Context, _ SubagentInvocation) (Result, error) {
		return Result{}, expectedErr
	})

	deliverer := NewSubagentDeliverer(invoker)
	_, err := deliverer.Deliver(context.Background(), Prompt{Markdown: []byte("valid instructions")})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected %v, got: %v", expectedErr, err)
	}
}

// -------------------------------------------------------------------------
// Noop Deliverer Tests
// -------------------------------------------------------------------------

func TestCycle163_NoopDeliverer_Semantics(t *testing.T) {
	t.Parallel()
	d := NoopDeliverer{}
	if d.Name() != "noop" {
		t.Fatalf("expected name 'noop', got: %s", d.Name())
	}

	res, err := d.Deliver(context.Background(), Prompt{Markdown: []byte("test")})
	if err != nil {
		t.Fatalf("unexpected error from NoopDeliverer: %v", err)
	}
	if res.DeliveredTo != "noop:skipped" {
		t.Fatalf("expected DeliveredTo 'noop:skipped', got: %s", res.DeliveredTo)
	}
}

// -------------------------------------------------------------------------
// TDE Enforcer Governance Verification Tests
// -------------------------------------------------------------------------

func TestCycle163_TDEEnforcer_SuccessAndDelegation(t *testing.T) {
	t.Parallel()
	tde := createTestSignedTDE(t)

	nextDeliverer := &recordingDeliverer{
		name:   "verified-target",
		result: Result{DeliveredTo: "secure://verified-destination", HTTPStatusCode: 200},
	}

	enforcer := &TDEEnforcer{Next: nextDeliverer}
	if enforcer.Name() != "tde_enforcer" {
		t.Fatalf("expected name 'tde_enforcer', got: %s", enforcer.Name())
	}

	prompt := Prompt{
		Markdown:  []byte("# Governance Controlled Context\nFollow POL-DEFAULT rules."),
		SessionID: "session-tde-001",
		TDE:       tde,
	}

	res, err := enforcer.Deliver(context.Background(), prompt)
	if err != nil {
		t.Fatalf("unexpected Deliver error on valid TDE: %v", err)
	}

	if !nextDeliverer.called {
		t.Fatal("expected downstream Deliverer to be called upon valid TDE verification")
	}
	if res.DeliveredTo != "secure://verified-destination" {
		t.Fatalf("expected result from downstream deliverer, got: %s", res.DeliveredTo)
	}
	if res.HTTPStatusCode != 200 {
		t.Fatalf("expected 200 status code, got: %d", res.HTTPStatusCode)
	}
}

func TestCycle163_TDEEnforcer_VerificationFailures(t *testing.T) {
	t.Parallel()
	nextDeliverer := &recordingDeliverer{name: "target"}
	enforcer := &TDEEnforcer{Next: nextDeliverer}

	// 1. Missing TDE
	_, err := enforcer.Deliver(context.Background(), Prompt{Markdown: []byte("test")})
	if err == nil {
		t.Fatal("expected error on missing TDE, got nil")
	}
	if !strings.Contains(err.Error(), "Neurological governance payload missing in TDE transport") {
		t.Fatalf("expected missing payload error, got: %v", err)
	}
	if nextDeliverer.called {
		t.Fatal("expected downstream deliverer NOT to be called when TDE is missing")
	}

	// 2. TDE with signature tampering
	tde := createTestSignedTDE(t)
	tde.Scope = "tampered_scope" // Alter content after signing
	_, err = enforcer.Deliver(context.Background(), Prompt{
		Markdown: []byte("test"),
		TDE:      tde,
	})
	if err == nil {
		t.Fatal("expected error on tampered TDE, got nil")
	}
	if !strings.Contains(err.Error(), "tde_enforcer: ABORT: TDE verification failed") {
		t.Fatalf("expected verification failed error, got: %v", err)
	}
}

// -------------------------------------------------------------------------
// Delivery Audit Logger Concurrency & Multi-Line Tests
// -------------------------------------------------------------------------

func TestCycle163_AppendDeliveryAuditJSONL_ConcurrentWrites(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	const workerCount = 5
	const writesPerWorker = 3
	var wg sync.WaitGroup

	for i := 0; i < workerCount; i++ {
		workerID := i
		wg.Add(1)
		builder := goroutinelabels.NewGoroutine("audit_test_worker", "concurrent audit log writer")
		builder.StartSimple(func() {
			defer wg.Done()
			for j := 0; j < writesPerWorker; j++ {
				rec := &DeliveryAuditRecord{
					ConvergenceSessionID: "sess-concurrent",
					Format:               "agent-prompt",
					DeliverMode:          "test",
					PrimaryDestination:   "test-dest",
					DeliveredTo:          []string{"audit-target"},
					MarkdownBytes:        workerID*10 + j,
				}
				AppendDeliveryAuditJSONL(dir, rec)
			}
		})
	}

	wg.Wait()

	path := AgentPromptDeliveriesJSONLPath(dir)
	content, err := fileutil.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read audit JSONL file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	expectedLines := workerCount * writesPerWorker
	if len(lines) != expectedLines {
		t.Fatalf("expected %d audit lines, got %d: %s", expectedLines, len(lines), string(content))
	}

	for idx, line := range lines {
		var r DeliveryAuditRecord
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("line %d is not valid JSON: %v", idx, err)
		}
		if r.EventType != EventTypeAgentPromptDelivery {
			t.Fatalf("line %d has unexpected event_type: %s", idx, r.EventType)
		}
		if r.ConvergenceSessionID != "sess-concurrent" {
			t.Fatalf("line %d has unexpected session ID: %s", idx, r.ConvergenceSessionID)
		}
		if r.Timestamp == "" {
			t.Fatalf("line %d missing timestamp", idx)
		}
	}
}

func TestCycle163_AgentPromptDeliveriesJSONLPath_RelativeAndAbsolute(t *testing.T) {
	t.Parallel()
	// Test empty root defaults to "."
	pEmpty := AgentPromptDeliveriesJSONLPath("")
	if !strings.HasPrefix(pEmpty, ".") {
		t.Fatalf("expected path to start with '.' when root is empty, got: %s", pEmpty)
	}

	// Test absolute path resolution
	pAbs := AgentPromptDeliveriesJSONLPath("/var/log/custom")
	expected := filepath.Join("/var/log/custom", paths.ProjectDataDir, paths.LogsDir, paths.SchedulerJobLogsSubdir, AgentPromptDeliveriesJSONLFile)
	if pAbs != expected {
		t.Fatalf("expected %s, got: %s", expected, pAbs)
	}
}
