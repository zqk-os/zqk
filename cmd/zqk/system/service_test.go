package system_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/spf13/cobra"
	systemcmd "github.com/zqk-os/zqk/cmd/zqk/system"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/service"
)

func executeCommand(root *cobra.Command, args ...string) (string, error) {
	buf := new(bytes.Buffer)
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), buf)
	root.SetContext(ctx)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

func setupMockServiceManager(mock *service.MockAdapter) func() {
	prevFactory := func() *systemcmd.ServiceManager {
		return systemcmd.NewServiceManager(service.WithAdapter(mock))
	}
	systemcmd.SetDefaultServiceManagerFactory(prevFactory)
	return func() {
		systemcmd.SetDefaultServiceManagerFactory(nil)
	}
}

func TestSystemServiceList(t *testing.T) {
	cmd := systemcmd.NewServiceCmd()
	out, err := executeCommand(cmd, "list")
	if err != nil {
		t.Fatalf("expected list to succeed, got error: %v", err)
	}
	if !bytes.Contains([]byte(out), []byte("memgraph")) {
		t.Errorf("expected output to contain memgraph, got: %s", out)
	}
}

func TestSystemServiceMockLifecycle(t *testing.T) {
	mock := service.NewMockAdapter("mock-adapter", true)
	// Pre-install a service spec for memgraph so mock knows about it
	ctx := context.Background()
	err := mock.Install(ctx, service.ServiceSpec{
		ID:         "memgraph",
		Executable: "/usr/local/bin/memgraph",
	})
	if err != nil {
		t.Fatalf("failed to install mock service: %v", err)
	}

	cleanup := setupMockServiceManager(mock)
	defer cleanup()

	// 1. Initial status: should be stopped
	statusCmd := systemcmd.NewServiceCmd()
	out, err := executeCommand(statusCmd, "status", "memgraph")
	if err != nil {
		t.Fatalf("expected status command to succeed, got: %v", err)
	}
	if !bytes.Contains([]byte(out), []byte("not running")) && !bytes.Contains([]byte(out), []byte("stopped")) {
		t.Errorf("expected status to report not running, got: %s", out)
	}

	// 2. Start service
	startCmd := systemcmd.NewServiceCmd()
	out, err = executeCommand(startCmd, "start", "memgraph")
	if err != nil {
		t.Fatalf("expected start command to succeed, got: %v", err)
	}
	if !bytes.Contains([]byte(out), []byte("started successfully")) {
		t.Errorf("expected output to confirm service start, got: %s", out)
	}
	if !mock.IsRunning("memgraph") {
		t.Fatalf("expected mock adapter to report service is running")
	}

	// 3. Status after start: should be running
	statusCmd2 := systemcmd.NewServiceCmd()
	out, err = executeCommand(statusCmd2, "status", "memgraph")
	if err != nil {
		t.Fatalf("expected status command to succeed, got: %v", err)
	}
	if !bytes.Contains([]byte(out), []byte("is running")) {
		t.Errorf("expected status to report running, got: %s", out)
	}

	// 4. Stop service
	stopCmd := systemcmd.NewServiceCmd()
	out, err = executeCommand(stopCmd, "stop", "memgraph")
	if err != nil {
		t.Fatalf("expected stop command to succeed, got: %v", err)
	}
	if !bytes.Contains([]byte(out), []byte("stopped successfully")) {
		t.Errorf("expected output to confirm service stop, got: %s", out)
	}
	if mock.IsRunning("memgraph") {
		t.Fatalf("expected mock adapter to report service is stopped")
	}

	// 5. Status after stop: should be stopped
	statusCmd3 := systemcmd.NewServiceCmd()
	out, err = executeCommand(statusCmd3, "status", "memgraph")
	if err != nil {
		t.Fatalf("expected status command to succeed, got: %v", err)
	}
	if !bytes.Contains([]byte(out), []byte("not running")) && !bytes.Contains([]byte(out), []byte("stopped")) {
		t.Errorf("expected status to report not running, got: %s", out)
	}
}

func TestSystemServiceJsonFormatting(t *testing.T) {
	mock := service.NewMockAdapter("mock-adapter", true)
	ctx := context.Background()
	_ = mock.Install(ctx, service.ServiceSpec{
		ID:         "memgraph",
		Executable: "/usr/local/bin/memgraph",
	})

	cleanup := setupMockServiceManager(mock)
	defer cleanup()

	cmd := systemcmd.NewServiceCmd()
	out, err := executeCommand(cmd, "list", "--format", "json")
	if err != nil {
		t.Fatalf("expected list json to succeed, got error: %v", err)
	}
	if !bytes.Contains([]byte(out), []byte(`"services"`)) {
		t.Errorf("expected json output with services key, got: %s", out)
	}
}

func TestSystemServiceErrorHandling(t *testing.T) {
	mock := service.NewMockAdapter("mock-adapter", true)
	cleanup := setupMockServiceManager(mock)
	defer cleanup()

	// Stop non-existent service
	stopCmd := systemcmd.NewServiceCmd()
	_, err := executeCommand(stopCmd, "stop", "nonexistent-svc")
	if err == nil {
		t.Errorf("expected error when stopping non-existent service, got nil")
	}

	// Status non-existent service should report stopped
	statusCmd := systemcmd.NewServiceCmd()
	out, err := executeCommand(statusCmd, "status", "nonexistent-svc")
	if err != nil {
		t.Errorf("expected status for unknown service to succeed with stopped, got: %v", err)
	}
	if !bytes.Contains([]byte(out), []byte("not running")) {
		t.Errorf("expected status output to report not running, got: %s", out)
	}
}

func TestSystemStartShutdownCommands(t *testing.T) {
	startCmd := systemcmd.NewStartCmd()
	if startCmd == nil {
		t.Fatal("expected NewStartCmd to return non-nil command")
	}

	shutdownCmd := systemcmd.NewShutdownCmd()
	if shutdownCmd == nil {
		t.Fatal("expected NewShutdownCmd to return non-nil command")
	}
}

