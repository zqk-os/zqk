package object

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/validation"
)

func TestApplyNamespaceBoundaries(t *testing.T) {
	tests := []struct {
		name          string
		federated     bool
		allNamespaces bool
		namespace     string
		filters       map[string]any
		wantFilter    string
		wantMode      string
		wantIsolation bool
	}{
		{
			name:          "default adds local namespace",
			federated:     false,
			namespace:     "",
			filters:       make(map[string]any),
			wantFilter:    validation.DefaultNamespaceKernel,
			wantMode:      namespaceScopeModeDefault,
			wantIsolation: true,
		},
		{
			name:          "federated skips isolation",
			federated:     true,
			namespace:     "",
			filters:       make(map[string]any),
			wantFilter:    "",
			wantMode:      namespaceScopeModeFederated,
			wantIsolation: false,
		},
		{
			name:          "all-namespaces alias skips isolation",
			allNamespaces: true,
			namespace:     "",
			filters:       make(map[string]any),
			wantFilter:    "",
			wantMode:      namespaceScopeModeFederated,
			wantIsolation: false,
		},
		{
			name:          "explicit namespace flag overrides",
			federated:     false,
			namespace:     "tenant:sandbox",
			filters:       make(map[string]any),
			wantFilter:    "tenant:sandbox",
			wantMode:      namespaceScopeModeExplicit,
			wantIsolation: true,
		},
		{
			name:          "existing filter skips default",
			federated:     false,
			namespace:     "",
			filters:       map[string]any{objects.FieldKeyNamespaceID: "existing:ns"},
			wantFilter:    "existing:ns",
			wantMode:      namespaceScopeModeFilter,
			wantIsolation: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().Bool("federated", false, "")
			cmd.Flags().Bool("all-namespaces", false, "")
			cmd.Flags().String("namespace", "", "")
			_ = cmd.Flags().Set("federated", "false")
			_ = cmd.Flags().Set("all-namespaces", "false")
			if tt.federated {
				_ = cmd.Flags().Set("federated", "true")
			}
			if tt.allNamespaces {
				_ = cmd.Flags().Set("all-namespaces", "true")
			}
			if tt.namespace != "" {
				_ = cmd.Flags().Set("namespace", tt.namespace)
			}

			scope := applyNamespaceBoundaries(cmd, tt.filters)

			got, hasFilter := tt.filters[objects.FieldKeyNamespaceID]
			if tt.wantFilter == "" {
				if hasFilter {
					t.Errorf("expected no namespace filter, got %v", got)
				}
			} else {
				if !hasFilter {
					t.Errorf("expected namespace filter %s, got none", tt.wantFilter)
				} else if got.(string) != tt.wantFilter {
					t.Errorf("expected namespace filter %s, got %v", tt.wantFilter, got)
				}
			}
			if scope.Mode != tt.wantMode {
				t.Errorf("mode: got %q want %q", scope.Mode, tt.wantMode)
			}
			if scope.IsolationActive != tt.wantIsolation {
				t.Errorf("isolation: got %v want %v", scope.IsolationActive, tt.wantIsolation)
			}
			if tt.wantFilter != "" && scope.NamespaceScope != tt.wantFilter {
				t.Errorf("scope.NamespaceScope: got %q want %q", scope.NamespaceScope, tt.wantFilter)
			}
		})
	}
}

func TestAttachNamespaceScopeMeta(t *testing.T) {
	scope := NamespaceQueryScope{
		NamespaceScope:  validation.DefaultNamespaceKernel,
		Mode:            namespaceScopeModeDefault,
		IsolationActive: true,
	}
	hidden := 3
	scope.HiddenOutsideScope = &hidden
	meta := attachNamespaceScopeMeta(nil, scope)
	if meta[metaKeyNamespaceScope] != validation.DefaultNamespaceKernel {
		t.Fatalf("namespace_scope: %v", meta[metaKeyNamespaceScope])
	}
	if meta[metaKeyHiddenOutsideScope] != 3 {
		t.Fatalf("hidden_outside_scope: %v", meta[metaKeyHiddenOutsideScope])
	}
	line := formatNamespaceScopeTableLine(scope)
	if line == "" || line[:10] != "Namespace:" {
		t.Fatalf("table line: %q", line)
	}
}
