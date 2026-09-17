package system

import (
	"testing"

	"github.com/lanceman/zqk/pkg/interactive"
)

func TestUpdateFieldFilter_IsFieldImmutable(t *testing.T) {
	t.Parallel()
	filter := interactive.NewUpdateFieldFilter()

	tests := []struct {
		name     string
		field    string
		expected bool
	}{
		{"id is immutable", "id", true},
		{"kind is immutable", "kind", true},
		{"created_at is immutable", "created_at", true},
		{"created_by is immutable", "created_by", true},
		{"schema_version is immutable", "schema_version", true},
		{"title is not immutable", "title", false},
		{"status is not immutable", "status", false},
		{"description is not immutable", "description", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filter.IsFieldImmutable(tt.field)
			if result != tt.expected {
				t.Errorf("IsFieldImmutable(%q) = %v, want %v", tt.field, result, tt.expected)
			}
		})
	}
}

func TestUpdateFieldFilter_IsFieldAutoManaged(t *testing.T) {
	t.Parallel()
	filter := interactive.NewUpdateFieldFilter()

	tests := []struct {
		name     string
		field    string
		expected bool
	}{
		{"updated_at is auto-managed", "updated_at", true},
		{"updated_by is auto-managed", "updated_by", true},
		{"created_at is not auto-managed", "created_at", false},
		{"title is not auto-managed", "title", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filter.IsFieldAutoManaged(tt.field)
			if result != tt.expected {
				t.Errorf("IsFieldAutoManaged(%q) = %v, want %v", tt.field, result, tt.expected)
			}
		})
	}
}

func TestUpdateFieldFilter_IsFieldUpdatable(t *testing.T) {
	t.Parallel()
	filter := interactive.NewUpdateFieldFilter()

	tests := []struct {
		name     string
		field    string
		expected bool
	}{
		{"id is not updatable", "id", false},
		{"kind is not updatable", "kind", false},
		{"created_at is not updatable", "created_at", false},
		{"title is updatable", "title", true},
		{"status is updatable", "status", true},
		{"updated_at is updatable (but auto-managed)", "updated_at", true},
		{"updated_by is updatable (but auto-managed)", "updated_by", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filter.IsFieldUpdatable(tt.field)
			if result != tt.expected {
				t.Errorf("IsFieldUpdatable(%q) = %v, want %v", tt.field, result, tt.expected)
			}
		})
	}
}
