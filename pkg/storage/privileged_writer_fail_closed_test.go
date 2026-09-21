package storage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/kernelcas"
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestPrivilegedWriterFailClosed_Create(t *testing.T) {
	testRoot, f, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	t.Setenv(zqkenv.TestAllowCASFallthrough().Name(), "0")
	// Do not accidentally dial a live LaunchAgent socket on the machine.
	t.Setenv(zqkenv.PrivilegedWriterSocket().Name(), filepath.Join(t.TempDir(), "no-privileged-writer.sock"))
	defer fileutil.RemoveAll(testRoot)

	// Create an object
	obj := map[string]any{
		objects.FieldKeyKind:        "backlog_item",
		objects.FieldKeyID:          "BLI-1234567890123456789-abcdef12",
		objects.FieldKeyStatus:      objects.ObjectStatusExploring,
		objects.FieldKeyTitle:       "Test BLI",
		objects.FieldKeyDescription: "Substantive description for privileged writer fail closed test.",
	}

	ctx := kernelcas.WithCommit(WithCLIOperation(pkgctx.WithPromoteOnCreate(context.Background())))
	secCtx := pkgctx.NewSecurityContext("system", []string{"admin"}, []string{"bypass_policy"})

	err := f.Create(ctx, secCtx, obj)
	if err == nil {
		t.Fatalf("Expected Create to fail-closed when PrivilegedWriter daemon is down, but it succeeded")
	}

	t.Logf("Error type: %T, err: %v\nStack trace: %+v", err, err, err)

	if !strings.Contains(err.Error(), "membrane is fail-closed") {
		t.Fatalf("Expected PrivilegedWriter fail-closed error, got: %v", err)
	}
}

func TestPrivilegedWriterFailClosed_Update(t *testing.T) {
	testRoot, f, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	// Start with membrane OPEN for tests (default is 1 for ZQK_TEST_ALLOW_CAS_FALLTHROUGH)
	t.Setenv(zqkenv.TestAllowCASFallthrough().Name(), "1")
	defer fileutil.RemoveAll(testRoot)

	// Create an object
	obj := map[string]any{
		objects.FieldKeyKind:        "backlog_item",
		objects.FieldKeyID:          "BLI-1234567890123456789-abcdef13",
		objects.FieldKeyStatus:      objects.ObjectStatusValidated,
		objects.FieldKeyTitle:       "Test BLI",
		objects.FieldKeyDescription: "Substantive description for privileged writer fail closed test.",
	}

	ctx := kernelcas.WithCommit(WithCLIOperation(pkgctx.WithPromoteOnCreate(context.Background())))
	secCtx := pkgctx.NewSecurityContext("system", []string{"admin"}, []string{"bypass_policy"})

	err := f.Create(ctx, secCtx, obj)
	if err != nil {
		t.Fatalf("Failed to create object: %v", err)
	}

	// Now close the membrane
	t.Setenv(zqkenv.TestAllowCASFallthrough().Name(), "0")
	t.Setenv(zqkenv.PrivilegedWriterSocket().Name(), filepath.Join(t.TempDir(), "no-privileged-writer.sock"))

	// Update the object
	updates := map[string]any{
		objects.FieldKeyTitle: "Updated BLI",
	}
	err = f.Update(ctx, secCtx, "BLI-1234567890123456789-abcdef13", updates)
	if err == nil {
		t.Fatalf("Expected Update to fail-closed when PrivilegedWriter daemon is down, but it succeeded")
	}

	if !strings.Contains(err.Error(), "membrane is fail-closed") {
		t.Fatalf("Expected PrivilegedWriter fail-closed error, got: %v", err)
	}
}

func TestPrivilegedWriterFailClosed_UpdateIDChange(t *testing.T) {
	testRoot, f, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	t.Setenv(zqkenv.TestAllowCASFallthrough().Name(), "1")
	defer fileutil.RemoveAll(testRoot)

	obj := map[string]any{
		objects.FieldKeyKind:        "backlog_item",
		objects.FieldKeyID:          "BLI-1234567890123456789-abcdef14",
		objects.FieldKeyStatus:      objects.ObjectStatusValidated,
		objects.FieldKeyTitle:       "Test BLI",
		objects.FieldKeyDescription: "Substantive description for privileged writer fail closed test.",
	}

	ctx := kernelcas.WithCommit(WithCLIOperation(pkgctx.WithPromoteOnCreate(context.Background())))
	secCtx := pkgctx.NewSecurityContext("system", []string{"admin"}, []string{"bypass_policy"})

	err := f.Create(ctx, secCtx, obj)
	if err != nil {
		t.Fatalf("Failed to create object: %v", err)
	}

	// Now close the membrane
	t.Setenv(zqkenv.TestAllowCASFallthrough().Name(), "0")
	t.Setenv(zqkenv.PrivilegedWriterSocket().Name(), filepath.Join(t.TempDir(), "no-privileged-writer.sock"))

	// Update the object ID
	updates := map[string]any{
		objects.FieldKeyID: "BLI-1234567890123456789-abcdef15",
	}
	err = f.Update(ctx, secCtx, "BLI-1234567890123456789-abcdef14", updates)
	if err == nil {
		t.Fatalf("Expected Update (ID Change) to fail-closed when PrivilegedWriter daemon is down, but it succeeded")
	}

	if !strings.Contains(err.Error(), "membrane is fail-closed") {
		t.Fatalf("Expected PrivilegedWriter fail-closed error, got: %v", err)
	}
}

func TestPrivilegedWriterFailClosed_Delete(t *testing.T) {
	testRoot, f, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	t.Setenv(zqkenv.TestAllowCASFallthrough().Name(), "1")
	defer fileutil.RemoveAll(testRoot)

	obj := map[string]any{
		objects.FieldKeyKind:        "backlog_item",
		objects.FieldKeyID:          "BLI-1234567890123456789-abcdef16",
		objects.FieldKeyStatus:      objects.ObjectStatusValidated,
		objects.FieldKeyTitle:       "Test BLI",
		objects.FieldKeyDescription: "Substantive description for privileged writer fail closed test.",
	}

	ctx := kernelcas.WithCommit(WithCLIOperation(pkgctx.WithPromoteOnCreate(context.Background())))
	secCtx := pkgctx.NewSecurityContext("system", []string{"admin"}, []string{"bypass_policy"})

	err := f.Create(ctx, secCtx, obj)
	if err != nil {
		t.Fatalf("Failed to create object: %v", err)
	}

	// Now close the membrane
	t.Setenv(zqkenv.TestAllowCASFallthrough().Name(), "0")
	t.Setenv(zqkenv.PrivilegedWriterSocket().Name(), filepath.Join(t.TempDir(), "no-privileged-writer.sock"))

	// Core-kind delete refuses first; satisfy that membrane so this test
	// can observe the privileged-writer fail-closed (do not weaken core-delete).
	err = f.Delete(WithTestHardDelete(ctx), secCtx, "BLI-1234567890123456789-abcdef16", false)
	if err == nil {
		t.Fatalf("Expected Delete to fail-closed when PrivilegedWriter daemon is down, but it succeeded")
	}

	if !strings.Contains(err.Error(), "membrane is fail-closed") {
		t.Fatalf("Expected PrivilegedWriter fail-closed error, got: %v", err)
	}
}
