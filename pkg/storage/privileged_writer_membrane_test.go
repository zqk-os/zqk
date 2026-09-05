package storage

import (
	"context"
	"testing"

	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestPrivilegedWriterLocalWriteAllowed(t *testing.T) {
	t.Setenv(zqkenv.IsDaemon(), "")
	t.Setenv(zqkenv.TestRoot(), "")
	t.Setenv(zqkenv.TestAllowCASFallthrough(), "1")

	if !privilegedWriterLocalWriteAllowed() {
		t.Error("expected local write to be allowed when ZQK_TEST_ALLOW_CAS_FALLTHROUGH=1")
	}

	t.Setenv(zqkenv.TestAllowCASFallthrough(), "0")
	if privilegedWriterLocalWriteAllowed() {
		t.Error("expected local write to not be allowed when ZQK_TEST_ALLOW_CAS_FALLTHROUGH=0")
	}

	t.Setenv(zqkenv.TestAllowCASFallthrough(), "")
	t.Setenv(zqkenv.TestRoot(), t.TempDir())
	if !privilegedWriterLocalWriteAllowed() {
		t.Error("expected local write when ZQK_TEST_ROOT is set (test harness auto-allow)")
	}

	t.Setenv(zqkenv.TestAllowCASFallthrough(), "0")
	if privilegedWriterLocalWriteAllowed() {
		t.Error("expected explicit ZQK_TEST_ALLOW_CAS_FALLTHROUGH=0 to override TestRoot auto-allow")
	}
}

func TestWriteCASThroughMembrane_Fallthrough(t *testing.T) {
	t.Setenv(zqkenv.TestAllowCASFallthrough(), "1")
	// Even if a studio PrivilegedWriter socket is live, test fallthrough must stay local.
	t.Setenv(zqkenv.PrivilegedWriterSocket(), DefaultPrivilegedWriterSocketPath())

	called := false
	localFn := func() error {
		called = true
		return nil
	}

	ctx := context.Background()
	err := writeCASThroughMembrane(ctx, "ID-123", "test_kind", []byte("data"), false, localFn)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !called {
		t.Error("expected localFn to be called")
	}
}

func TestWriteCASThroughMembrane_TestRootSkipsLiveDaemon(t *testing.T) {
	t.Setenv(zqkenv.TestAllowCASFallthrough(), "")
	t.Setenv(zqkenv.TestRoot(), t.TempDir())
	t.Setenv(zqkenv.PrivilegedWriterSocket(), DefaultPrivilegedWriterSocketPath())

	called := false
	err := writeCASThroughMembrane(context.Background(), "ID-123", "test_kind", []byte("data"), true, func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !called {
		t.Error("expected localFn when ZQK_TEST_ROOT is set even if PrivilegedWriter is dialable")
	}
}

func TestWriteCASThroughMembrane_DaemonArgvWithoutEnv(t *testing.T) {
	t.Setenv(zqkenv.IsDaemon(), "")
	t.Setenv(zqkenv.TestRoot(), "")
	t.Setenv(zqkenv.TestAllowCASFallthrough(), "0")
	t.Setenv(zqkenv.PrivilegedWriterSocket(), zqkenv.UnreachableTestSocketPath())
	setPrivilegedWriterCommandArgs(t, []string{"zqk-stable", "object", "daemon"})

	called := false
	err := writeCASThroughMembrane(context.Background(), "ID-123", "test_kind", []byte("data"), false, func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("expected argv role to write locally without IS_DAEMON, got %v", err)
	}
	if !called {
		t.Error("expected localFn from object daemon argv — Setenv order must not be required")
	}
}

func TestWriteCASThroughMembrane_DaemonWritesLocally(t *testing.T) {
	t.Setenv(zqkenv.TestRoot(), "")
	t.Setenv(zqkenv.TestAllowCASFallthrough(), "0")
	t.Setenv(zqkenv.IsDaemon(), "1")
	t.Setenv(zqkenv.PrivilegedWriterSocket(), zqkenv.UnreachableTestSocketPath())

	called := false
	err := writeCASThroughMembrane(context.Background(), "ID-123", "test_kind", []byte("data"), false, func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("expected daemon to write locally, got %v", err)
	}
	if !called {
		t.Error("expected localFn when IS_DAEMON=1 — self-dial is the EMFILE footgun")
	}
	if !privilegedWriterLocalWriteAllowed() {
		t.Error("expected local write allowed for the privileged-writer daemon process")
	}
}

func TestWriteCASThroughMembrane_NoFallthrough(t *testing.T) {
	t.Setenv(zqkenv.TestRoot(), "")
	t.Setenv(zqkenv.IsDaemon(), "")
	t.Setenv(zqkenv.TestAllowCASFallthrough(), "0")

	// Dialing will fail because socket doesn't exist, and fallthrough is disabled, so it should return an error
	called := false
	localFn := func() error {
		called = true
		return nil
	}

	ctx := context.Background()
	err := writeCASThroughMembrane(ctx, "ID-123", "test_kind", []byte("data"), false, localFn)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if called {
		t.Error("expected localFn not to be called")
	}
}
