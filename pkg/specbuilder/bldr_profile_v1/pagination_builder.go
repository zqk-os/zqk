package bldr_profile_v1

import (
	"github.com/lanceman/zqk/pkg/config"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/profile_builders"
)

// PaginationBuilder builds the pagination profile at version v1_0_0
// File: bldr_profile_v1/pagination_builder.go - version is encoded in package/directory name
type PaginationBuilder struct {
	*profile_builders.BaseProfileBuilder
}

// NewPaginationBuilder creates a new builder for pagination profile version v1_0_0
func NewPaginationBuilder() *PaginationBuilder {
	builder := &PaginationBuilder{
		BaseProfileBuilder: profile_builders.NewBaseProfileBuilder("pagination", "v1_0_0"),
	}

	// Configure the profile
	builder.
		SetSchemaVersion(SchemaVersionV1).
		SetKind("profile").
		SetType(config.ProfileTypeCLIContext).
		SetMetadata(config.ProfileMetadata{
			Name:        "pagination",
			Extends:     "base_profile",
			Description: "Pagination context: sets max page size to 5 items per page.\\nUseful for testing pagination behavior or limiting result sets.\\n",
		}).
		SetSpec(map[string]any{
			objects.FieldKeyFormat: "table",
			"quiet":                false,
			objects.FieldKeyStorage: map[string]any{
				"default_page_size": 5,
				"max_page_size":     5,
			},
			"verbose": false,
		})

	return builder
}

func init() {
	profile_builders.RegisterBuilder(NewPaginationBuilder())
}
