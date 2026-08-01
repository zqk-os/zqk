package object

import (
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/validation"
	"github.com/spf13/cobra"
)

func TestApplyNamespaceBoundaries(t *testing.T) {
	tests := []struct {
		name       string
		federated  bool
		namespace  string
		filters    map[string]any
		wantFilter string
	}{
		{
			name:       "default adds local namespace",
			federated:  false,
			namespace:  "",
			filters:    make(map[string]any),
			wantFilter: validation.DefaultNamespaceKernel,
		},
		{
			name:       "federated skips isolation",
			federated:  true,
			namespace:  "",
			filters:    make(map[string]any),
			wantFilter: "",
		},
		{
			name:       "explicit namespace flag overrides",
			federated:  false,
			namespace:  "tenant:sandbox",
			filters:    make(map[string]any),
			wantFilter: "tenant:sandbox",
		},
		{
			name:       "existing filter skips default",
			federated:  false,
			namespace:  "",
			filters:    map[string]any{objects.FieldKeyNamespaceID: "existing:ns"},
			wantFilter: "existing:ns",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().Bool("federated", false, "")
			cmd.Flags().String("namespace", "", "")
			cmd.Flags().Set("federated", "false")
			if tt.federated {
				cmd.Flags().Set("federated", "true")
			}
			if tt.namespace != "" {
				cmd.Flags().Set("namespace", tt.namespace)
			}

			applyNamespaceBoundaries(cmd, tt.filters)

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
		})
	}
}
