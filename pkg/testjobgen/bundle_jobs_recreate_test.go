package testjobgen

import (
	"context"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/scheduler"
	"github.com/zqk-os/zqk/pkg/storage"
)

type recreateProbeStore struct {
	storage.ObjectStorageProvider
	status string
}

func (s *recreateProbeStore) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	return map[string]any{objects.FieldKeyID: id, objects.FieldKeyStatus: s.status}, nil
}

func TestMustRecreateTestBundleJob(t *testing.T) {
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	cases := []struct {
		status string
		want   bool
	}{
		{scheduler.StatusActive, false},
		{"disabled", false},
		{"pending", false},
		{objects.ObjectStatusArchived, true},
		{objects.ObjectStatusError, true},
		{objects.ObjectStatusCompleted, true},
		{"complete", true},
		{"weird", true},
	}
	for _, tc := range cases {
		got := mustRecreateTestBundleJob(ctx, &recreateProbeStore{status: tc.status}, sec, "SCH-run-x")
		if got != tc.want {
			t.Fatalf("status=%q got %v want %v", tc.status, got, tc.want)
		}
	}
}
