package binary

import (
	"bytes"
	"testing"

	"github.com/zqk-os/zqk/pkg/specbuilder/registry"
)

func TestBinaryStorageWrite(t *testing.T) {
	reg := &registry.FieldRegistry{
		Fields: map[string]registry.FieldMetadata{
			"TST-001": {Name: "testField", Type: "string", ProfileCode: "TST-001", Description: "Test field"},
		},
	}
	provider := NewBinaryStorageProvider(reg)

	buf := new(bytes.Buffer)
	data := map[string]any{"testField": "hello"}

	if err := provider.WriteObject(buf, data); err != nil {
		t.Fatal(err)
	}

	if buf.Len() == 0 {
		t.Fatal("buffer is empty")
	}
}
