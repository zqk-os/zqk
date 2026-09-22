// BLI-STARTER-COMMUNITY-031 / PRI-STARTER-COMMUNITY-031 coverage elevation
package testservices

import (
	"context"
	"testing"
	"time"
)

func TestNewServiceManagerAndMemGraphConfig(t *testing.T) {
	t.Parallel()
	sm := NewServiceManager()
	if sm == nil {
		t.Fatal("nil manager")
	}
	sm.log("quiet %s", "ok")
	cfg := GetMemGraphConfig()
	if cfg.Name != "memgraph" || cfg.ContainerName == "" || cfg.HealthCheck.Type != "port" {
		t.Fatalf("%+v", cfg)
	}
}

func TestHostPortsAlreadyPublished_Empty(t *testing.T) {
	t.Parallel()
	sm := NewServiceManager()
	if sm.hostPortsAlreadyPublished(ServiceConfig{}) {
		t.Fatal("empty ports")
	}
}

func TestCheckHealth_CommandAndDefault(t *testing.T) {
	t.Parallel()
	sm := NewServiceManager()
	ctx := context.Background()
	ok, err := sm.checkCommandHealth(ctx, nil)
	if err == nil || ok {
		t.Fatal("empty command")
	}
	ok, err = sm.checkCommandHealth(ctx, []string{"true"})
	if err != nil || !ok {
		t.Fatalf("true: %v %v", ok, err)
	}
	ok, err = sm.checkHealth(ctx, ServiceConfig{
		HealthCheck: HealthCheck{Type: "command", Command: []string{"false"}},
	})
	if err != nil || ok {
		t.Fatalf("false cmd: %v %v", ok, err)
	}
	ok, err = sm.checkHealth(ctx, ServiceConfig{
		ContainerName: "zqk-test-coverage-missing",
		HealthCheck:   HealthCheck{Type: "unknown"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = ok
}

func TestWaitForHealthy_Cancelled(t *testing.T) {
	t.Parallel()
	sm := NewServiceManager()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := sm.waitForHealthy(ctx, ServiceConfig{
		StartTimeout: time.Second,
		HealthCheck:  HealthCheck{Type: "command", Command: []string{"false"}, Interval: time.Millisecond},
	})
	if err == nil {
		t.Fatal("expected cancel")
	}
}

func TestSetupAndCleanup_GraphDisabled(t *testing.T) {
	ctx := context.Background()
	ts, err := SetupTestServices(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if ts == nil {
		t.Fatal("nil services")
	}
	if err := ts.Cleanup(); err != nil {
		t.Fatal(err)
	}
}

func TestStopService_NotRunning(t *testing.T) {
	t.Parallel()
	sm := NewServiceManager()
	if err := sm.StopService(context.Background(), "zqk-test-coverage-not-running"); err != nil {
		t.Fatal(err)
	}
	_ = sm.IsServiceRunning("zqk-test-coverage-not-running")
}
