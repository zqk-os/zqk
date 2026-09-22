package binary

import (
	"errors"
	"testing"

	"github.com/zqk-os/zqk/pkg/specbuilder/registry"
)

type errWriter struct {
	failAt int
	count  int
}

func (e *errWriter) Write(p []byte) (n int, err error) {
	e.count++
	if e.count >= e.failAt {
		return 0, errors.New("writer error")
	}
	return len(p), nil
}

func TestBinaryStorageProvider_ExtraCoverage(t *testing.T) {
	reg := &registry.FieldRegistry{
		Fields: map[string]registry.FieldMetadata{
			"F1": {ID: 10, Name: "field1"},
			"F2": {ID: 20, Name: "field2"},
		},
	}
	provider := NewBinaryStorageProvider(reg)

	// Object with no matching fields
	dataEmpty := map[string]any{"non_matching": "val"}
	ew := &errWriter{failAt: 1}
	if err := provider.WriteObject(ew, dataEmpty); err != nil {
		t.Fatalf("expected nil when no fields match, got %v", err)
	}

	// Error writing FieldID
	ewID := &errWriter{failAt: 1}
	data := map[string]any{"field1": "val1"}
	if err := provider.WriteObject(ewID, data); err == nil {
		t.Fatalf("expected error writing FieldID")
	}

	// Error writing Length
	ewLen := &errWriter{failAt: 2}
	if err := provider.WriteObject(ewLen, data); err == nil {
		t.Fatalf("expected error writing length")
	}

	// Error writing Payload
	ewPayload := &errWriter{failAt: 3}
	if err := provider.WriteObject(ewPayload, data); err == nil {
		t.Fatalf("expected error writing payload")
	}
}
