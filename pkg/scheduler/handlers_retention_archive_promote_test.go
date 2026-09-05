package scheduler

import (
	"context"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

type retentionArchiveCandidateStore struct {
	objs map[string]map[string]any
}

func (s *retentionArchiveCandidateStore) Read(_ context.Context, _ *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if s == nil || s.objs == nil {
		return nil, nil
	}
	return s.objs[id], nil
}

func TestRetentionArchiveCandidate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	store := &retentionArchiveCandidateStore{objs: map[string]map[string]any{
		"PRI-COMPLETE": {
			objects.FieldKeyID:     "PRI-COMPLETE",
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyStatus: objects.ObjectStatusComplete,
		},
		"PRI-ACTIVE": {
			objects.FieldKeyID:     "PRI-ACTIVE",
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyStatus: objects.ObjectStatusActive,
		},
		"PRI-ARCHIVED": {
			objects.FieldKeyID:     "PRI-ARCHIVED",
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyStatus: objects.ObjectStatusArchived,
		},
	}}

	cases := []struct {
		name string
		kind string
		obj  map[string]any
		want bool
	}{
		{
			name: "complete plan eligible",
			kind: objects.KindPriorityPlan,
			obj: map[string]any{
				objects.FieldKeyID:     "PRI-COMPLETE",
				objects.FieldKeyStatus: objects.ObjectStatusComplete,
			},
			want: true,
		},
		{
			name: "active plan not eligible",
			kind: objects.KindPriorityPlan,
			obj: map[string]any{
				objects.FieldKeyID:     "PRI-ACTIVE",
				objects.FieldKeyStatus: objects.ObjectStatusActive,
			},
			want: false,
		},
		{
			name: "orphan BLI eligible",
			kind: objects.KindBacklogItem,
			obj: map[string]any{
				objects.FieldKeyID:     "BLI-ORPHAN",
				objects.FieldKeyStatus: objects.ObjectStatusComplete,
			},
			want: true,
		},
		{
			name: "BLI under complete plan skipped",
			kind: objects.KindBacklogItem,
			obj: map[string]any{
				objects.FieldKeyID:              "BLI-1",
				objects.FieldKeyStatus:          objects.ObjectStatusComplete,
				objects.FieldKeyPriorityPlanRef: "PRI-COMPLETE",
			},
			want: false,
		},
		{
			name: "BLI under archived plan eligible",
			kind: objects.KindBacklogItem,
			obj: map[string]any{
				objects.FieldKeyID:              "BLI-2",
				objects.FieldKeyStatus:          objects.ObjectStatusComplete,
				objects.FieldKeyPriorityPlanRef: "PRI-ARCHIVED",
			},
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := retentionArchiveCandidate(ctx, sec, store, tc.kind, tc.obj, objects.ObjectStatusArchived)
			if got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
