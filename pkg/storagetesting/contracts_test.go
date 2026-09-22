package storagetesting

import (
	"errors"
	"testing"
)

type dummyFactory struct {
	auditBuf any
}

func (d *dummyFactory) CreateFileStorage(testRoot string) (any, error) {
	if testRoot == "" {
		return nil, errors.New("empty test root")
	}
	return testRoot, nil
}

func (d *dummyFactory) GetAuditBuffer() any {
	return d.auditBuf
}

type dummyConfigurator struct {
	configured bool
}

func (c *dummyConfigurator) ConfigureTestGlobals(t *testing.T, opts *GlobalHookOptions) {
	if opts != nil && opts.NoopStorageMetrics {
		c.configured = true
	}
}

func TestContracts(t *testing.T) {
	var factory IsolationFactory = &dummyFactory{auditBuf: "test-buffer"}
	res, err := factory.CreateFileStorage("/tmp/test")
	if err != nil || res != "/tmp/test" {
		t.Fatalf("unexpected result: %v, %v", res, err)
	}

	_, err = factory.CreateFileStorage("")
	if err == nil {
		t.Fatalf("expected error for empty root")
	}

	buf := factory.GetAuditBuffer()
	if buf != "test-buffer" {
		t.Fatalf("expected 'test-buffer', got %v", buf)
	}

	opts := &GlobalHookOptions{NoopStorageMetrics: true}
	var configurator GlobalHookConfigurator = &dummyConfigurator{}
	configurator.ConfigureTestGlobals(t, opts)

	if !configurator.(*dummyConfigurator).configured {
		t.Fatalf("expected configurator to be configured")
	}
}
