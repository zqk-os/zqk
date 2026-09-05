package audit

import (
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/bldr_instance_v1"
)

func TestResolveActor(t *testing.T) {
	t.Parallel()
	if got := ResolveActor(nil, "explicit"); got != "explicit" {
		t.Fatalf("explicit: %q", got)
	}
	sec := &pkgctx.SecurityContext{AccountID: "ACC-1"}
	if got := ResolveActor(sec, ""); got != "ACC-1" {
		t.Fatalf("sec: %q", got)
	}
	if got := ResolveActor(nil, ""); got != pkgctx.SystemAccountID {
		t.Fatalf("system: %q", got)
	}
}

func TestBufferCheckMap(t *testing.T) {
	t.Parallel()
	m := BufferCheckMap(&EventOptions{EventType: "object_creation", Severity: "low", TargetID: "BLI-1"})
	if m[FieldEventType] != "object_creation" || m[FieldTargetID] != "BLI-1" {
		t.Fatalf("%v", m)
	}
}

func TestPopulateEvent(t *testing.T) {
	t.Parallel()
	builder := bldr_instance_v1.NewAuditEventInstanceBuilder("2.0.0")
	PopulateEvent(builder, "AUD-1", "/proj", "ACC-1", "2026-01-01T00:00:00Z", &EventOptions{
		EventType: EventTypeObjectCreation,
		Operation: "create",
		Severity:  SeverityLow,
		TargetID:  "BLI-1",
		SessionID: "SESS-1",
	})
	got, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if objects.GetString(got, FieldID) != "AUD-1" {
		t.Fatalf("id=%v", got[FieldID])
	}
	if objects.GetString(got, FieldEventType) != EventTypeObjectCreation {
		t.Fatalf("event_type=%v", got[FieldEventType])
	}
	if objects.GetString(got, FieldSessionID) != "SESS-1" {
		t.Fatalf("session=%v", got[FieldSessionID])
	}
}
