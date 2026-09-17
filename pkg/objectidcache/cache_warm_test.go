package objectidcache

import (
	"testing"
)

func TestCacheWarm_NoOpNotifier(t *testing.T) {
	if noOpCacheProgressNotifier == nil {
		t.Fatal("expected non-nil noOpCacheProgressNotifier")
	}
	noOpCacheProgressNotifier.NotifyCacheProgress("kind", "step")
}
