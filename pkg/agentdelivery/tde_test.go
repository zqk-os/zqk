package agentdelivery

import (
	"context"
	"testing"

	"github.com/lanceman/zqk/pkg/policy"
)

type mockDeliverer struct {
	called bool
}

func (m *mockDeliverer) Name() string { return "mock" }

func (m *mockDeliverer) Deliver(ctx context.Context, p Prompt) (Result, error) {
	m.called = true
	return Result{}, nil
}

func TestTDEEnforcer_MissingTDE(t *testing.T) {
	mock := &mockDeliverer{}
	enforcer := &TDEEnforcer{Next: mock}

	prompt := Prompt{Markdown: []byte("test")}
	_, err := enforcer.Deliver(context.Background(), prompt)
	if err == nil {
		t.Fatal("expected error for missing TDE")
	}
	if mock.called {
		t.Fatal("expected mock to not be called")
	}
}

func TestTDEEnforcer_InvalidTDE(t *testing.T) {
	mock := &mockDeliverer{}
	enforcer := &TDEEnforcer{Next: mock}

	prompt := Prompt{
		Markdown: []byte("test"),
		TDE:      &policy.TrustDomainEnvelope{MCPPermissions: []string{}}, // empty, will fail verify
	}
	_, err := enforcer.Deliver(context.Background(), prompt)
	if err == nil {
		t.Fatal("expected error for invalid TDE")
	}
	if mock.called {
		t.Fatal("expected mock to not be called")
	}
}
