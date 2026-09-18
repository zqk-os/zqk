package storage

import (
	"context"
	"fmt"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"

	"github.com/zqk-os/zqk/pkg/validation"
)

func TestCheckCriteriaVerificationOutcomeAuthority(t *testing.T) {
	sys := pkgctx.NewSystemSecurityContext()
	dev := pkgctx.NewSecurityContext("account:voa-unit", []string{"developer"}, []string{"read:*", "write:*"})

	t.Run("non-criteria kind ignores outcome statuses", func(t *testing.T) {
		if err := checkCriteriaVerificationOutcomeAuthority(objects.KindBacklogItem, dev, objects.ObjectStatusValidated); err != nil {
			t.Fatalf("unexpected: %v", err)
		}
	})

	t.Run("criteria non-outcome status allowed for non-system", func(t *testing.T) {
		if err := checkCriteriaVerificationOutcomeAuthority(objects.KindCriteria, dev, objects.ObjectStatusInProgress); err != nil {
			t.Fatalf("unexpected: %v", err)
		}
	})

	t.Run("criteria validated denied for non-system", func(t *testing.T) {
		if err := checkCriteriaVerificationOutcomeAuthority(objects.KindCriteria, dev, objects.ObjectStatusValidated); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("criteria complete denied for non-system", func(t *testing.T) {
		if err := checkCriteriaVerificationOutcomeAuthority(objects.KindCriteria, dev, criteriaStatusComplete); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("criteria outcome allowed for system", func(t *testing.T) {
		if err := checkCriteriaVerificationOutcomeAuthority(objects.KindCriteria, sys, objects.ObjectStatusValidated); err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if err := checkCriteriaVerificationOutcomeAuthority(objects.KindCriteria, sys, criteriaStatusComplete); err != nil {
			t.Fatalf("unexpected: %v", err)
		}
	})

}

func TestCheckVerificationOutcomeAuthority(t *testing.T) {
	sys := pkgctx.NewSystemSecurityContext()
	dev := pkgctx.NewSecurityContext("account:voa-umbrella", []string{"developer"}, []string{"read:*", "write:*"})

	t.Run("criteria outcome via umbrella", func(t *testing.T) {
		if err := checkVerificationOutcomeAuthority(objects.KindCriteria, dev, objects.ObjectStatusValidated); err == nil {
			t.Fatal("expected error")
		}
		if err := checkVerificationOutcomeAuthority(objects.KindCriteria, sys, objects.ObjectStatusValidated); err != nil {
			t.Fatalf("unexpected: %v", err)
		}
	})

	t.Run("convergence_session error denied for non-system", func(t *testing.T) {
		if err := checkVerificationOutcomeAuthority(objects.KindConvergenceSession, dev, objects.ObjectStatusError); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("convergence_session completed allowed for non-system", func(t *testing.T) {
		if err := checkVerificationOutcomeAuthority(objects.KindConvergenceSession, dev, objects.ObjectStatusCompleted); err != nil {
			t.Fatalf("unexpected: %v", err)
		}
	})

	t.Run("convergence_session error allowed for system", func(t *testing.T) {
		if err := checkVerificationOutcomeAuthority(objects.KindConvergenceSession, sys, objects.ObjectStatusError); err != nil {
			t.Fatalf("unexpected: %v", err)
		}
	})

}

func TestCriteriaVerificationOutcomeAuthority_FileStorage(t *testing.T) {
	testRoot := setupCriteriaTestRoot(t)
	fos, err := NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("NewFileObjectStorageForTest: %v", err)
	}

	defer func() { _ = fos.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(testRoot, fos)
		if err := RunProjectTestTeardown(opts); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	ctx := context.Background()
	cliCtx := pkgctx.WithPromoteOnCreate(WithTestHardDelete(ctx))
	sys := pkgctx.NewSystemSecurityContext()
	dev := pkgctx.NewSecurityContext("account:voa-dev", []string{"developer"}, []string{"read:*", "write:*"})

	critID := fmt.Sprintf("CRIT-voa%d", time.Now().UnixNano())
	initial := map[string]any{
		objects.FieldKeyID:            critID,
		objects.FieldKeyKind:          objects.KindCriteria,
		objects.FieldKeyTitle:         "voa " + critID,
		objects.FieldKeyDescription:   "Substantive description for criteria verification authority test.",
		objects.FieldKeyCategory:      "functional",
		objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusAwaitingVerification,
	}
	if err := fos.Create(cliCtx, sys, initial); err != nil {
		t.Fatalf("Create: %v", err)
	}

	t.Run("developer cannot set validated", func(t *testing.T) {
		err := fos.Update(ctx, dev, critID, map[string]any{objects.FieldKeyStatus: objects.ObjectStatusValidated})
		if err == nil {
			t.Fatal("expected permission error")
		}
	})

	t.Run("developer cannot set complete", func(t *testing.T) {
		err := fos.Update(ctx, dev, critID, map[string]any{objects.FieldKeyStatus: criteriaStatusComplete})
		if err == nil {
			t.Fatal("expected permission error")
		}
	})

	t.Run("developer can set in_progress", func(t *testing.T) {
		if err := fos.Update(ctx, dev, critID, map[string]any{objects.FieldKeyStatus: objects.ObjectStatusInProgress}); err != nil {
			t.Fatalf("Update in_progress: %v", err)
		}
	})

	t.Run("system can set validated", func(t *testing.T) {
		if err := fos.Update(ctx, sys, critID, map[string]any{objects.FieldKeyStatus: objects.ObjectStatusValidated}); err != nil {
			t.Fatalf("Update validated as system: %v", err)
		}
	})

}
