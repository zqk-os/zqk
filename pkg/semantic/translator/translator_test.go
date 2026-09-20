package translator_test

import (
	"encoding/json"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/ontology"
	"github.com/zqk-os/zqk/pkg/semantic/translator"
)

type dummyTranslator struct{}

func (d *dummyTranslator) SupportedFormat() string {
	return "dummy-json"
}

func (d *dummyTranslator) TranslateToInternal(className string, externalData []byte) (map[string]any, error) {
	var out map[string]any
	if err := json.Unmarshal(externalData, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (d *dummyTranslator) TranslateToExternal(className string, internalData map[string]any) ([]byte, error) {
	return json.Marshal(internalData)
}

func TestEngine_TranslateAndValidate(t *testing.T) {
	mgr := ontology.NewManager()

	o := ontology.Ontology{
		ID:      "core-1.0",
		Version: "1.0",
		Classes: map[string]ontology.Class{
			"Widget": {
				Name: "Widget",
				Properties: map[string]ontology.Property{
					objects.FieldKeyName: {Name: "name", Type: "string", Required: true},
					"price":              {Name: "price", Type: "int", Required: false},
				},
			},
		},
	}
	_ = mgr.Register(o)

	engine := translator.NewEngine(mgr)
	_ = engine.RegisterTranslator(&dummyTranslator{})

	// Valid data
	validData := []byte(`{"name": "SuperWidget", "price": 10}`)
	result, err := engine.TranslateAndValidate("dummy-json", "Widget", validData)
	if err != nil {
		t.Errorf("Expected success, got err: %v", err)
	}
	if name, ok := result[objects.FieldKeyName].(string); !ok || name != "SuperWidget" {
		t.Errorf("Expected SuperWidget, got %v", result[objects.FieldKeyName])
	}

	// Invalid data (missing required 'name' property)
	invalidData := []byte(`{"price": 10}`)
	_, err = engine.TranslateAndValidate("dummy-json", "Widget", invalidData)
	if err != translator.ErrValidationFailed {
		t.Errorf("Expected ErrValidationFailed, got: %v", err)
	}

	// Unsupported format
	_, err = engine.TranslateAndValidate("yaml", "Widget", validData)
	if err != translator.ErrUnsupportedFormat {
		t.Errorf("Expected ErrUnsupportedFormat, got: %v", err)
	}
}
