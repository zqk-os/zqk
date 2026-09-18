package mcp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/observer"
)

// fakeStorageProvider records Create calls for tests.
type fakeStorageProvider struct {
	mu      sync.Mutex
	creates []createCall
	err     error // if set, Create returns this error
}

type createCall struct {
	ctx    context.Context
	secCtx *pkgctx.SecurityContext
	obj    map[string]any
}

func (f *fakeStorageProvider) List(ctx context.Context, secCtx *pkgctx.SecurityContext, _ *pkgctx.StorageContext, filter any) (any, error) {
	return nil, nil
}

func (f *fakeStorageProvider) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	f.mu.Lock()
	f.creates = append(f.creates, createCall{ctx: ctx, secCtx: secCtx, obj: obj})
	err := f.err
	f.mu.Unlock()
	return err
}

func (f *fakeStorageProvider) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	return nil, fmt.Errorf("Read not implemented")
}

func (f *fakeStorageProvider) getCreates() []createCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]createCall(nil), f.creates...)
}

func TestRegisterAgentConnectionAccountCreation_nilServer(t *testing.T) {
	RegisterAgentConnectionAccountCreation(nil) // must not panic
}

func TestRegisterAgentConnectionAccountCreation_noStorage_noCreate(t *testing.T) {
	server := NewServer()
	// no SetStorageProvider
	RegisterAgentConnectionAccountCreation(server)
	ctx := context.Background()
	observer.NotifyAgentConnection(ctx, observer.AgentConnectionInfo{
		ClientID: "no-storage-client", ClientName: "NoStorage", Version: "1.0",
	})
	time.Sleep(50 * time.Millisecond)
	// Callback runs but should not call Create (storage is nil)
	// Nothing to assert except no panic
}

func waitForCreates(t *testing.T, fake *fakeStorageProvider, before int) []createCall {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		creates := fake.getCreates()
		newCreates := creates[before:]
		if len(newCreates) > 0 {
			return newCreates
		}
		time.Sleep(10 * time.Millisecond)
	}
	creates := fake.getCreates()
	return creates[before:]
}

func TestRegisterAgentConnectionAccountCreation_createsAccount(t *testing.T) {
	fake := &fakeStorageProvider{}
	server := NewServer()
	server.storageProvider = fake
	server.secCtx = pkgctx.NewSystemSecurityContext()

	before := len(fake.getCreates())
	RegisterAgentConnectionAccountCreation(server)
	ctx := context.Background()
	observer.NotifyAgentConnection(ctx, observer.AgentConnectionInfo{
		ClientID: "test-agent-01", ClientName: "Test Agent", Version: "1.0",
	})

	newCreates := waitForCreates(t, fake, before)
	if len(newCreates) != 1 {
		t.Fatalf("expected 1 new Create call, got %d", len(newCreates))
	}
	obj := newCreates[0].obj
	if obj[objects.FieldKeyID] != "account:test-agent-01" {
		t.Errorf("obj[id] = %v, want account:test-agent-01", obj[objects.FieldKeyID])
	}
	if obj[objects.FieldKeyKind] != "account" {
		t.Errorf("obj[kind] = %v, want account", obj[objects.FieldKeyKind])
	}
	if obj[objects.FieldKeyStatus] != "active" {
		t.Errorf("obj[status] = %v, want active", obj[objects.FieldKeyStatus])
	}
	// roles may be []string or []interface{} depending on builder
	switch r := obj[objects.FieldKeyRoles].(type) {
	case []string:
		if len(r) != 1 || r[0] != "developer" {
			t.Errorf("obj[roles] = %v, want [developer]", r)
		}
	case []interface{}:
		if len(r) != 1 || r[0] != "developer" {
			t.Errorf("obj[roles] = %v, want [developer]", r)
		}
	default:
		t.Errorf("obj[roles] = %T %v, want [developer]", obj[objects.FieldKeyRoles], obj[objects.FieldKeyRoles])
	}
	if obj[objects.FieldKeyUsername] != "test-agent-01" {
		t.Errorf("obj[username] = %v, want test-agent-01", obj[objects.FieldKeyUsername])
	}
	if obj[objects.FieldKeyDisplayName] != "Test Agent" {
		t.Errorf("obj[display_name] = %v, want Test Agent", obj[objects.FieldKeyDisplayName])
	}
}

func TestRegisterAgentConnectionAccountCreation_sanitizesUsername(t *testing.T) {
	fake := &fakeStorageProvider{}
	server := NewServer()
	server.storageProvider = fake
	server.secCtx = pkgctx.NewSystemSecurityContext()

	before := len(fake.getCreates())
	RegisterAgentConnectionAccountCreation(server)
	ctx := context.Background()
	observer.NotifyAgentConnection(ctx, observer.AgentConnectionInfo{
		ClientID: "client.with.dots", ClientName: "Dots", Version: "1.0",
	})

	newCreates := waitForCreates(t, fake, before)
	if len(newCreates) != 1 {
		t.Fatalf("expected 1 new Create call, got %d", len(newCreates))
	}
	obj := newCreates[0].obj
	// sanitizeUsername replaces non [a-zA-Z0-9_-] with _
	wantID := "account:client_with_dots"
	if obj[objects.FieldKeyID] != wantID {
		t.Errorf("obj[id] = %v, want %s", obj[objects.FieldKeyID], wantID)
	}
	if obj[objects.FieldKeyUsername] != "client_with_dots" {
		t.Errorf("obj[username] = %v, want client_with_dots", obj[objects.FieldKeyUsername])
	}
}

func TestRegisterAgentConnectionAccountCreation_emptyClientID_usesAgent(t *testing.T) {
	fake := &fakeStorageProvider{}
	server := NewServer()
	server.storageProvider = fake
	server.secCtx = pkgctx.NewSystemSecurityContext()

	before := len(fake.getCreates())
	RegisterAgentConnectionAccountCreation(server)
	ctx := context.Background()
	observer.NotifyAgentConnection(ctx, observer.AgentConnectionInfo{
		ClientID: "---", ClientName: "Fallback", Version: "1.0",
	})

	newCreates := waitForCreates(t, fake, before)
	// Find the create for account:agent (our callback with sanitized empty -> "agent")
	var obj map[string]any
	for _, c := range newCreates {
		if c.obj[objects.FieldKeyID] == "account:agent" {
			obj = c.obj
			break
		}
	}
	if obj == nil {
		t.Fatalf("no Create with id account:agent in %d new creates", len(newCreates))
	}
	if obj[objects.FieldKeyUsername] != "agent" {
		t.Errorf("obj[username] = %v, want agent", obj[objects.FieldKeyUsername])
	}
}

func TestAccountCreatedNotificationMessage_BLI860(t *testing.T) {
	// BLI-860: new agent must receive "Account created: account:{agent_name}, please re-initialize"
	got := accountCreatedNotificationMessage("account:my-agent")
	want := "Account created: account:my-agent, please re-initialize"
	if got != want {
		t.Errorf("accountCreatedNotificationMessage(%q) = %q, want %q", "account:my-agent", got, want)
	}
}

func TestRegisterAgentConnectionAccountCreation_alreadyExists_idempotent(t *testing.T) {
	fake := &fakeStorageProvider{}
	fake.err = errors.New("object already exists")
	server := NewServer()
	server.storageProvider = fake
	server.secCtx = pkgctx.NewSystemSecurityContext()

	before := len(fake.getCreates())
	RegisterAgentConnectionAccountCreation(server)
	ctx := context.Background()
	observer.NotifyAgentConnection(ctx, observer.AgentConnectionInfo{
		ClientID: "dup-agent", ClientName: "Dup", Version: "1.0",
	})

	newCreates := waitForCreates(t, fake, before)
	if len(newCreates) != 1 {
		t.Fatalf("expected 1 new Create call (then treat already exists as success), got %d", len(newCreates))
	}
	// Callback should not panic; "already exists" is treated as success
}
