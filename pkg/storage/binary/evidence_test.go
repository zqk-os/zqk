// BLI-STARTER-COMMUNITY-021 / PRI-STARTER-COMMUNITY-048
package binary

import (
	"io"
	"testing"

	"github.com/zqk-os/zqk/pkg/specbuilder/registry"
)

func TestEvidenceNewBinaryStorageProvider(t *testing.T) {
	p := NewBinaryStorageProvider(&registry.FieldRegistry{Fields: map[string]registry.FieldMetadata{}})
	if p == nil {
		t.Fatal("nil provider")
	}
	if err := p.WriteObject(io.Discard, map[string]any{"x": 1}); err != nil {
		t.Fatal(err)
	}
}
