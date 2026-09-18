package bldr_profile_v1

import "github.com/zqk-os/zqk/pkg/specbuilder/profile_builders"

// SchemaVersionV1 is the schema_version string for v1_0_0 profile builders in this package.
// Profile builder *.go files are generated; pkg/specbuilder/profile_builders/codegen.go emits
// SetSchemaVersion(SchemaVersionV1) when YAML schema_version matches this value.
const SchemaVersionV1 = profile_builders.DefaultProfileSchemaVersion
