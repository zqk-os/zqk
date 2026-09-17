package security

import (
	"fmt"
	"sync"
	"testing"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

func TestNormalizeAndValidatePath_Success(t *testing.T) {
	tests := []struct {
		name      string
		unchecked string
		prefixes  []string
		want      string
	}{
		{
			name:      "clean child path under allowed prefix",
			unchecked: "/var/log/zqk/test.log",
			prefixes:  []string{"/var/log/zqk"},
			want:      "/var/log/zqk/test.log",
		},
		{
			name:      "exact match on allowed prefix directory itself",
			unchecked: "/var/log/zqk",
			prefixes:  []string{"/var/log/zqk"},
			want:      "/var/log/zqk",
		},
		{
			name:      "exact match on allowed prefix with trailing slash",
			unchecked: "/var/log/zqk/",
			prefixes:  []string{"/var/log/zqk"},
			want:      "/var/log/zqk",
		},
		{
			name:      "path with redundant slashes and dots",
			unchecked: "/var/log/zqk/./subdir/../test.log",
			prefixes:  []string{"/var/log/zqk"},
			want:      "/var/log/zqk/test.log",
		},
		{
			name:      "multiple prefixes with match",
			unchecked: "/opt/zqk/data/db.sqlite",
			prefixes:  []string{"/var/log", "/opt/zqk"},
			want:      "/opt/zqk/data/db.sqlite",
		},
		{
			name:      "root prefix allows everything",
			unchecked: "/etc/zqk/config.yaml",
			prefixes:  []string{"/"},
			want:      "/etc/zqk/config.yaml",
		},
		{
			name:      "no prefixes returns cleaned path",
			unchecked: "some/relative/path/../file.txt",
			prefixes:  nil,
			want:      "some/relative/file.txt",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeAndValidatePath(tc.unchecked, tc.prefixes...)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNormalizeAndValidatePath_TraversalBlocked(t *testing.T) {
	tests := []struct {
		name      string
		unchecked string
		prefixes  []string
	}{
		{
			name:      "dot dot traversal out of prefix",
			unchecked: "/var/log/zqk/../../etc/passwd",
			prefixes:  []string{"/var/log/zqk"},
		},
		{
			name:      "prefix string prefix collision without separator",
			unchecked: "/var/log/zqk-other/secret.txt",
			prefixes:  []string{"/var/log/zqk"},
		},
		{
			name:      "null byte injection in path",
			unchecked: "/var/log/zqk/valid.txt\x00/../../etc/shadow",
			prefixes:  []string{"/var/log/zqk"},
		},
		{
			name:      "root traversal",
			unchecked: "/",
			prefixes:  []string{"/var/log"},
		},
		{
			name:      "dot dot",
			unchecked: "..",
			prefixes:  nil,
		},
		{
			name:      "single dot",
			unchecked: ".",
			prefixes:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NormalizeAndValidatePath(tc.unchecked, tc.prefixes...)
			if err == nil {
				t.Errorf("expected error for traversal %q, got nil", tc.unchecked)
			}
			if err != ErrPathTraversalDetected {
				t.Errorf("expected ErrPathTraversalDetected, got %v", err)
			}
		})
	}
}

func TestSandbox_Operations(t *testing.T) {
	sb := NewSandbox("/workspace", "/tmp/zqk")

	// Verify roots
	roots := sb.Roots()
	if len(roots) != 2 {
		t.Fatalf("expected 2 roots, got %d", len(roots))
	}

	// Allowed paths
	allowedPaths := []string{
		"/workspace/src/main.go",
		"/workspace",
		"/tmp/zqk/cache/chunk.bin",
	}
	for _, p := range allowedPaths {
		if !sb.IsAllowed(p) {
			t.Errorf("expected path %q to be allowed by sandbox", p)
		}
		validated, err := sb.ValidatePath(p)
		if err != nil {
			t.Errorf("unexpected error validating %q: %v", p, err)
		}
		if validated == "" {
			t.Errorf("expected non-empty validated path for %q", p)
		}
	}

	// Denied paths
	deniedPaths := []string{
		"/etc/passwd",
		"/workspace/../../etc/shadow",
		"/workspace-other/file.txt",
		"/tmp/zqk/../../root/.ssh/id_rsa",
	}
	for _, p := range deniedPaths {
		if sb.IsAllowed(p) {
			t.Errorf("expected path %q to be denied by sandbox", p)
		}
		_, err := sb.ValidatePath(p)
		if err != ErrSandboxPathDenied {
			t.Errorf("expected ErrSandboxPathDenied for %q, got %v", p, err)
		}
	}

	// AddRoot
	sb.AddRoot("/var/data")
	if !sb.IsAllowed("/var/data/records.db") {
		t.Errorf("expected added root to be permitted")
	}

	// Empty sandbox denies all
	emptySb := NewSandbox()
	if emptySb.IsAllowed("/workspace/test") {
		t.Errorf("expected empty sandbox to deny all paths")
	}
}

func TestSandbox_ConcurrentAccess(t *testing.T) {
	sb := NewSandbox("/workspace")
	var wg sync.WaitGroup
	workers := 20
	iterations := 50

	for i := 0; i < workers; i++ {
		workerID := i
		goroutinelabels.NewGoroutine(fmt.Sprintf("sandbox_worker_%d", workerID), "concurrent sandbox access test").
			WithWaitGroup(&wg).
			StartSimple(func() {
				for j := 0; j < iterations; j++ {
					_ = sb.Roots()
					_ = sb.IsAllowed(fmt.Sprintf("/workspace/sub_%d/file_%d.go", workerID, j))
					_ = sb.IsAllowed(fmt.Sprintf("/etc/secret_%d", j))

					if j%10 == 0 {
						sb.AddRoot(fmt.Sprintf("/tmp/worker_%d", workerID))
					}
				}
			})
	}

	wg.Wait()
}
