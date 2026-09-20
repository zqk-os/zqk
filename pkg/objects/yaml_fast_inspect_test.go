package objects

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestExtractProperty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		data     string
		property string
		want     string
	}{
		{
			name:     "simple yaml",
			data:     "id: BLI-010\nkind: backlog_item\n",
			property: "id",
			want:     "BLI-010",
		},
		{
			name:     "first line without preceding newline",
			data:     "kind: backlog_item\nid: BLI-010\n",
			property: "kind",
			want:     "backlog_item",
		},
		{
			name:     "double quoted key and value",
			data:     "\"id\": \"BLI-010\"\n\"kind\": \"backlog_item\"\n",
			property: "id",
			want:     "BLI-010",
		},
		{
			name:     "single quoted key and value",
			data:     "'id': 'REQ-035'\n'kind': 'requirement'\n",
			property: "id",
			want:     "REQ-035",
		},
		{
			name:     "space before colon",
			data:     "id : BLI-010\nkind : backlog_item\n",
			property: "id",
			want:     "BLI-010",
		},
		{
			name:     "inline comment",
			data:     "id: BLI-001 # primary backlog item\nkind: backlog_item\n",
			property: "id",
			want:     "BLI-001",
		},
		{
			name:     "hash inside quoted value not stripped",
			data:     "title: \"Fix for issue #42 now\"\nid: BLI-042\n",
			property: "title",
			want:     "Fix for issue #42 now",
		},
		{
			name:     "created_at timestamp",
			data:     "id: BLI-001\ncreated_at: 2026-09-15T10:00:00Z\nstatus: in_progress\n",
			property: "created_at",
			want:     "2026-09-15T10:00:00Z",
		},
		{
			name: "top-level key preferred over indented nested key",
			data: `metadata:
  id: nested-id
  kind: nested-kind
id: top-level-id
kind: backlog_item
`,
			property: "id",
			want:     "top-level-id",
		},
		{
			name:     "json formatted",
			data:     `{"id": "BLI-100", "kind": "backlog_item", "status": "active"}`,
			property: "id",
			want:     "BLI-100",
		},
		{
			name:     "json with newline",
			data:     "{\n  \"id\": \"BLI-101\",\n  \"kind\": \"backlog_item\"\n}\n",
			property: "kind",
			want:     "backlog_item",
		},
		{
			name: "block scalar fallback",
			data: `id: BLI-050
description: |
  Line 1 of description
  Line 2 of description
kind: backlog_item
`,
			property: "description",
			want:     "Line 1 of description\nLine 2 of description\n",
		},
		{
			name:     "empty yaml",
			data:     "",
			property: "id",
			want:     "",
		},
		{
			name:     "missing property",
			data:     "id: BLI-010\nkind: backlog_item\n",
			property: "status",
			want:     "",
		},
		{
			name:     "null value",
			data:     "id: BLI-010\nstatus: null\n",
			property: "status",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractProperty([]byte(tt.data), tt.property)
			if got != tt.want {
				t.Errorf("ExtractProperty(%q, %q) = %q; want %q", tt.data, tt.property, got, tt.want)
			}
		})
	}
}

func TestExtractProperties(t *testing.T) {
	t.Parallel()

	data := []byte(`id: BLI-200
kind: backlog_item
created_at: "2026-09-15T12:00:00Z"
event_type: state_transition
status: open
`)

	props := ExtractProperties(data, "id", "kind", "created_at", "event_type", "status", "non_existent")

	if props["id"] != "BLI-200" {
		t.Errorf("expected id BLI-200, got %q", props["id"])
	}
	if props["kind"] != "backlog_item" {
		t.Errorf("expected kind backlog_item, got %q", props["kind"])
	}
	if props["created_at"] != "2026-09-15T12:00:00Z" {
		t.Errorf("expected created_at 2026-09-15T12:00:00Z, got %q", props["created_at"])
	}
	if props["event_type"] != "state_transition" {
		t.Errorf("expected event_type state_transition, got %q", props["event_type"])
	}
	if props["status"] != "open" {
		t.Errorf("expected status open, got %q", props["status"])
	}
	if props["non_existent"] != "" {
		t.Errorf("expected non_existent empty, got %q", props["non_existent"])
	}
}

func TestExtractPropertiesFromFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// 1. Small file test
	smallPath := filepath.Join(dir, "small.yaml")
	smallContent := "id: BLI-300\nkind: backlog_item\ncreated_at: 2026-09-15T14:00:00Z\n"
	if err := fileutil.WriteFile(smallPath, []byte(smallContent), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	id := ExtractPropertyFromFile(smallPath, "id")
	if id != "BLI-300" {
		t.Errorf("expected id BLI-300, got %q", id)
	}

	props := ExtractPropertiesFromFile(smallPath, "id", "kind", "created_at")
	if props["id"] != "BLI-300" || props["kind"] != "backlog_item" || props["created_at"] != "2026-09-15T14:00:00Z" {
		t.Errorf("unexpected props from small file: %+v", props)
	}

	// 2. Large file test (>4096 bytes) where property is near the end
	largePath := filepath.Join(dir, "large.yaml")
	var builder strings.Builder
	builder.WriteString("header: start\n")
	for i := 0; i < 200; i++ {
		builder.WriteString(fmt.Sprintf("comment_line_%03d: padding_large_file_content_to_exceed_buffer_limits\n", i))
	}
	builder.WriteString("id: BLI-400\nkind: backlog_item\n")
	largeContent := builder.String()
	if len(largeContent) <= defaultYAMLInspectReadLimit {
		t.Fatalf("expected test content > %d bytes, got %d", defaultYAMLInspectReadLimit, len(largeContent))
	}

	if err := fileutil.WriteFile(largePath, []byte(largeContent), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	largeID := ExtractPropertyFromFile(largePath, "id")
	if largeID != "BLI-400" {
		t.Errorf("expected id BLI-400 from large file, got %q", largeID)
	}

	largeProps := ExtractPropertiesFromFile(largePath, "id", "kind")
	if largeProps["id"] != "BLI-400" || largeProps["kind"] != "backlog_item" {
		t.Errorf("unexpected large file props: %+v", largeProps)
	}
}

func BenchmarkExtractProperty(b *testing.B) {
	data := []byte(`id: BLI-010
kind: backlog_item
created_at: 2026-09-15T10:00:00Z
title: Test Item
status: in_progress
`)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ExtractProperty(data, "id")
	}
}

func BenchmarkStandardYAMLUnmarshalMap(b *testing.B) {
	data := []byte(`id: BLI-010
kind: backlog_item
created_at: 2026-09-15T10:00:00Z
title: Test Item
status: in_progress
`)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var obj map[string]any
		_ = yaml.Unmarshal(data, &obj)
		_ = obj["id"]
	}
}
