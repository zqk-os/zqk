// BLI-STARTER-COMMUNITY-041 / PRI-STARTER-COMMUNITY-041 coverage elevation
package engine

import (
	"context"
	"testing"

	"github.com/zqk-os/zqk/pkg/bridge"
)

type extraTranslator struct{}

func (extraTranslator) Translate(_ context.Context, input []byte) (map[string]any, error) {
	return map[string]any{"len": len(input)}, nil
}

func (extraTranslator) GetSupportedFormats() []string {
	return []string{"json"}
}

var _ bridge.SemanticTranslator = extraTranslator{}

func TestExtraTranslateRegisteredFormat(t *testing.T) {
	eng := NewTranslationEngine()
	eng.RegisterTranslator("json", extraTranslator{})
	got, err := eng.Translate(context.Background(), "json", []byte(`{"a":1}`))
	if err != nil || got["len"] != 7 {
		t.Fatalf("translate = %#v %v", got, err)
	}
	if _, err := eng.Translate(context.Background(), "yaml", nil); err == nil {
		t.Fatal("expected missing translator")
	}
}
