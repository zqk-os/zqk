package tray

import (
	_ "embed"
	"encoding/json"
	"sync"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

//go:embed tray_config.schema.json
var traySchemaJSON []byte

const traySchemaURL = "https://zqk.dev/schemas/tray_config.schema.json"

var (
	traySchemaOnce sync.Once
	traySchema     *jsonschema.Schema
	traySchemaErr  error
)

func compiledTraySchema() (*jsonschema.Schema, error) {
	traySchemaOnce.Do(func() {
		traySchema, traySchemaErr = jsonschema.CompileString(traySchemaURL, string(traySchemaJSON))
	})
	return traySchema, traySchemaErr
}

// validateTrayConfig checks cfg against the embedded JSON Schema (draft-07).
// Canonical schema file also lives at .zqk/cli/specs/schemas/tray_config.schema.json (keep in sync).
func validateTrayConfig(cfg *Config) error {
	if cfg == nil {
		return errfmt.Errorf("tray config is nil")
	}
	schema, err := compiledTraySchema()
	if err != nil {
		return errfmt.Newf("compile tray JSON Schema").Wrap(err)
	}
	// Round-trip through JSON so numeric and field names match schema expectations.
	b, err := json.Marshal(cfg)
	if err != nil {
		return errfmt.Newf("tray config json encode").Wrap(err)
	}
	var doc any
	if err := json.Unmarshal(b, &doc); err != nil {
		return errfmt.Newf("tray config json decode").Wrap(err)
	}
	if err := schema.Validate(doc); err != nil {
		return errfmt.Newf("tray config schema validation failed").Wrap(err)
	}
	return nil
}
