package cli

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
)

func TestIsNumericCASCommandStem(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want bool
	}{
		{"1785199714348041000_411cd659", true},
		{"1785199714348041000-411cd659", true},
		{"start_here", false},
		{"SYSTEM_ORCHESTRATE_BATCH", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsNumericCASCommandStem(tc.in); got != tc.want {
			t.Fatalf("IsNumericCASCommandStem(%q)=%v want %v", tc.in, got, tc.want)
		}
	}
}

func TestResolveCommandBuilderName_PrefersUseOverNumericCSPEC(t *testing.T) {
	t.Parallel()
	spec := map[string]any{
		objects.FieldKeyID:   "CSPEC-1785199714348041000-411cd659",
		objects.FieldKeyKind: "command_spec",
		objects.FieldKeyUse:  "start-here",
	}
	got := ResolveCommandBuilderName(spec, "/tmp/70793d0aa49c.yaml")
	if got != "start_here" {
		t.Fatalf("got %q want start_here", got)
	}
}

func TestResolveCommandBuilderName_NestedDNAUsesPathStem(t *testing.T) {
	t.Parallel()
	spec := map[string]any{objects.FieldKeyName: "install"}
	got := ResolveCommandBuilderName(spec, filepath.Join("/repo", paths.CLICommandSpecsDir, "scheduler/service/install_command.yaml"))
	if got != "scheduler_service_install" {
		t.Fatalf("got %q want scheduler_service_install", got)
	}
	// Shallow group DNA must path-qualify so object/list and scheduler/list do not collide.
	got = ResolveCommandBuilderName(map[string]any{objects.FieldKeyName: "list"}, filepath.Join("/repo", paths.CLICommandSpecsDir, "scheduler/list_command.yaml"))
	if got != "scheduler_list" {
		t.Fatalf("got %q want scheduler_list", got)
	}
	got = ResolveCommandBuilderName(map[string]any{objects.FieldKeyName: "list"}, filepath.Join("/repo", paths.CLICommandSpecsDir, "object/list_command.yaml"))
	if got != "object_list" {
		t.Fatalf("got %q want object_list", got)
	}
	got = ResolveCommandBuilderName(map[string]any{objects.FieldKeyName: "count"}, filepath.Join("/repo", paths.CLICommandSpecsDir, "object/count_command.yaml"))
	if got != "object_count" {
		t.Fatalf("got %q want object_count", got)
	}
}

func TestIsProcessCommandSpecCAS(t *testing.T) {
	t.Parallel()
	if !IsProcessCommandSpecCAS(map[string]any{objects.FieldKeyKind: "command_spec", objects.FieldKeyID: "CSPEC-1"}) {
		t.Fatal("expected CAS true")
	}
	if IsProcessCommandSpecCAS(map[string]any{objects.FieldKeyName: "start-here"}) {
		t.Fatal("DNA yaml should not be CAS")
	}
}
