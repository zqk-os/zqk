package mcp

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

const (
	markdownMIMEType = "text/markdown"
	// Generous relative to the real runtime (~4s under -race); this only has to
	// fail the test instead of hanging if the registry deadlocks.
	concurrentRegistrationTimeout = 60 * time.Second
)

func discoveredResourceURI(worker, index int) string {
	return fmt.Sprintf("file://discovered/%d-%d.md", worker, index)
}

// Resource discovery walks the tree inside ParallelExecutor while sibling tasks
// register resources. An unguarded read of server.resources crashed the daemon
// with "fatal error: concurrent map read and map write"; -race catches it here.
func TestIsResourceAlreadyRegistered_ConcurrentWithRegistration(t *testing.T) {
	const goroutines = 8
	const iterations = 200

	server := NewServer()

	var wg sync.WaitGroup
	for w := range goroutines {
		worker := w
		wg.Add(2)
		goroutinelabels.NewGoroutine("mcp_resource_register", "registering discovered resources").
			StartSimple(func() {
				defer wg.Done()
				for i := range iterations {
					server.RegisterResource(discoveredResourceURI(worker, i), "name", "description", markdownMIMEType)
				}
			})
		goroutinelabels.NewGoroutine("mcp_resource_lookup", "probing resource registry").
			StartSimple(func() {
				defer wg.Done()
				for i := range iterations {
					isResourceAlreadyRegistered(server, discoveredResourceURI(worker, i))
				}
			})
	}

	waitDone := make(chan struct{})
	goroutinelabels.NewGoroutine("mcp_resource_race_join", "joining resource race workers").
		StartSimple(func() {
			wg.Wait()
			close(waitDone)
		})
	select {
	case <-waitDone:
	case <-time.After(concurrentRegistrationTimeout):
		t.Fatalf("registration workers did not finish within %s", concurrentRegistrationTimeout)
	}

	probe := "file://discovered/probe.md"
	if isResourceAlreadyRegistered(server, probe) {
		t.Fatalf("unregistered resource %q reported as registered", probe)
	}
	server.RegisterResource(probe, "probe", "probe resource", markdownMIMEType)
	if !isResourceAlreadyRegistered(server, probe) {
		t.Fatalf("registered resource %q reported as missing", probe)
	}
}
