package bldr_config_v1

// ConfigYAMLVersionV1 is the embedded "version" field value in internal config YAML for v1 builders.
// Config *_builder.go files in this package are generated from docs/process/_internal/configs/*_config.yaml
// by pkg/specbuilder/config_builders/codegen.go. Do not edit them by hand; run:
//
//	zqk system generate-config-builders --overwrite
//
// Codegen emits ConfigYAMLVersionV1 in SetConfig literals when the YAML top-level version matches this value.
const ConfigYAMLVersionV1 = "1.0.0"
