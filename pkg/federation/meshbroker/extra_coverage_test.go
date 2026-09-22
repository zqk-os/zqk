// BLI-STARTER-COMMUNITY-038 / PRI-STARTER-COMMUNITY-038 coverage elevation
package meshbroker

import (
	"context"
	"errors"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/federation"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

type brokerStore struct {
	storage.NoopObjectStorage
	lists     []*storage.QueryResult
	listErr   []error
	i         int
	objs      map[string]map[string]any
	createErr error
}

func (s *brokerStore) List(context.Context, *storage.SecurityContext, *storage.StorageContext, storage.ListFilter) (*storage.QueryResult, error) {
	idx := s.i
	s.i++
	if idx < len(s.listErr) && s.listErr[idx] != nil {
		return nil, s.listErr[idx]
	}
	if idx < len(s.lists) && s.lists[idx] != nil {
		return s.lists[idx], nil
	}
	return &storage.QueryResult{}, nil
}

func (s *brokerStore) Read(_ context.Context, _ *storage.SecurityContext, id string) (map[string]any, error) {
	if obj, ok := s.objs[id]; ok {
		return obj, nil
	}
	return nil, storage.ErrObjectNotFound
}

func (s *brokerStore) Create(_ context.Context, _ *storage.SecurityContext, obj map[string]any) error {
	if s.createErr != nil {
		return s.createErr
	}
	return nil
}

func TestExtraSkillAndComputeBroker(t *testing.T) {
	ctx := context.Background()
	b := NewSkillBroker(&brokerStore{listErr: []error{errors.New("list"), errors.New("list")}}, nil, t.TempDir())
	if _, err := b.EnsureSkill(ctx, "skill-1"); err == nil {
		t.Fatal("expected missing skill")
	}

	existing := &brokerStore{
		lists: []*storage.QueryResult{{
			Objects: []map[string]any{{
				objects.FieldKeyResourceRef:       "skill-1",
				objects.FieldKeyProviderKernelRef: "KER-1",
				objects.FieldKeyTokenID:           "TOK-1",
			}},
		}},
		objs: map[string]map[string]any{
			"KER-1": {objects.FieldKeyEndpoint: "http://peer"},
		},
	}
	got, err := NewSkillBroker(existing, nil, t.TempDir()).EnsureSkill(ctx, "skill-1")
	if err != nil || got == nil || got.Endpoint != "http://peer" || got.Token != "TOK-1" {
		t.Fatalf("existing lease = %+v %v", got, err)
	}
	secured := pkgctx.WithSecurityContext(ctx, pkgctx.NewSystemSecurityContext())
	existing.i = 0
	got, err = NewSkillBroker(existing, nil, t.TempDir()).EnsureSkill(secured, "skill-1")
	if err != nil || got == nil {
		t.Fatalf("secured lease = %+v %v", got, err)
	}

	noKernel := &brokerStore{
		lists: []*storage.QueryResult{
			{},
			{Objects: []map[string]any{{objects.FieldKeyProviderKernelRef: "KER-missing"}}},
		},
	}
	if _, err := NewSkillBroker(noKernel, nil, t.TempDir()).EnsureSkill(ctx, "skill-2"); err == nil {
		t.Fatal("expected missing kernel")
	}

	createFail := &brokerStore{
		lists: []*storage.QueryResult{
			{},
			{Objects: []map[string]any{{objects.FieldKeyProviderKernelRef: "KER-1"}}},
		},
		objs:      map[string]map[string]any{"KER-1": {objects.FieldKeyEndpoint: "ep"}},
		createErr: errors.New("create"),
	}
	if _, err := NewSkillBroker(createFail, nil, t.TempDir()).EnsureSkill(ctx, "skill-3"); err == nil {
		t.Fatal("expected create failure")
	}

	okStore := &brokerStore{
		lists: []*storage.QueryResult{
			{},
			{Objects: []map[string]any{{objects.FieldKeyProviderKernelRef: "KER-1"}}},
		},
		objs: map[string]map[string]any{"KER-1": {objects.FieldKeyEndpoint: "ep"}},
	}
	leased, err := NewSkillBroker(okStore, nil, t.TempDir()).EnsureSkill(ctx, "skill-4")
	if err != nil || leased == nil || leased.ID != "skill-4" || leased.Token == "" {
		t.Fatalf("new lease = %+v %v", leased, err)
	}

	cb := NewComputeBroker(&brokerStore{listErr: []error{errors.New("list")}})
	if _, err := cb.FindCapabilityPod(ctx, "gpu"); err == nil {
		t.Fatal("expected missing pod")
	}
	cbOK := NewComputeBroker(&brokerStore{lists: []*storage.QueryResult{{
		Objects: []map[string]any{{objects.FieldKeyID: "pod-1"}},
	}}})
	pod, err := cbOK.FindCapabilityPod(ctx, "gpu")
	if err != nil || pod[objects.FieldKeyID] != "pod-1" {
		t.Fatalf("pod = %+v %v", pod, err)
	}

	tok, err := newFederationLeaseToken()
	if err != nil || tok == "" {
		t.Fatalf("token = %q %v", tok, err)
	}

	tr := NewMCPClientTransport("")
	if err := tr.SendHeartbeat(ctx, "ep", "k"); err != nil {
		t.Fatal(err)
	}
	if err := tr.Close(); err != nil {
		t.Fatal(err)
	}
	pc := &pooledClient{}
	pc.close()
	if _, err := tr.ExecuteTool(ctx, "tcp://", "object_list", nil); err == nil {
		t.Fatal("expected empty tcp dial failure")
	}
	hsCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if _, err := tr.SendHandshake(hsCtx, t.TempDir(), federation.HandshakeRequest{KernelID: "k"}); err == nil {
		t.Fatal("expected handshake CLI failure")
	}

	orphan := &brokerStore{lists: []*storage.QueryResult{{
		Objects: []map[string]any{{
			objects.FieldKeyResourceRef:       "skill-x",
			objects.FieldKeyProviderKernelRef: "KER-gone",
			objects.FieldKeyTokenID:           "TOK",
		}},
	}}}
	remote, err := NewSkillBroker(orphan, nil, t.TempDir()).EnsureSkill(ctx, "skill-x")
	if err != nil || remote.Endpoint != "" {
		t.Fatalf("orphan lease = %+v %v", remote, err)
	}
}
