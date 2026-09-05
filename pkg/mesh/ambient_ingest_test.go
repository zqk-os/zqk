package mesh

import (
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/ambient"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/stretchr/testify/assert"
)

func TestAmbientIngestService(t *testing.T) {
	projectRoot := "/tmp/mock-project"
	secCtx := pkgctx.NewSecurityContext(pkgctx.SystemAccountID, []string{"system"}, []string{"*"})

	service := NewAmbientIngestService(projectRoot, secCtx)
	assert.NotNil(t, service)

	// Test doc_entry mapping (.md)
	eventMD := ambient.Event{
		Type:      ambient.EventTypeFilesystem,
		Payload:   map[string]any{"file": "docs/architecture/README.md"},
		Timestamp: time.Now(),
	}
	kindMD, uriMD := service.MapToSystemObject(eventMD)
	assert.Equal(t, "doc_entry", kindMD)
	assert.Equal(t, "docs/architecture/README.md", uriMD)

	// Test technical_spec mapping (.yaml and .json)
	eventYAML := ambient.Event{
		Type:      ambient.EventTypeFilesystem,
		Payload:   map[string]any{"file": "docs/process/technical_spec/tspec-001.yaml"},
		Timestamp: time.Now(),
	}
	kindYAML, uriYAML := service.MapToSystemObject(eventYAML)
	assert.Equal(t, "technical_spec", kindYAML)
	assert.Equal(t, "docs/process/technical_spec/tspec-001.yaml", uriYAML)

	eventJSON := ambient.Event{
		Type:      ambient.EventTypeFilesystem,
		Payload:   map[string]any{"file": "specs/config.json"},
		Timestamp: time.Now(),
	}
	kindJSON, uriJSON := service.MapToSystemObject(eventJSON)
	assert.Equal(t, "technical_spec", kindJSON)
	assert.Equal(t, "specs/config.json", uriJSON)

	// Test fallback mapping
	eventOther := ambient.Event{
		Type:      ambient.EventTypeGit,
		Payload:   map[string]any{"action": "commit"},
		Timestamp: time.Now(),
	}
	kindOther, uriOther := service.MapToSystemObject(eventOther)
	assert.Equal(t, "audit_event", kindOther)
	assert.Equal(t, "", uriOther)

	// Test Hub Binding
	hub := ambient.NewEventHub()
	service.BindToHub(hub)
	assert.Equal(t, "active", hub.Status())
}
