package crud

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

func TestLiveCASBlobUnreadable(t *testing.T) {
	t.Parallel()
	if LiveCASBlobUnreadable(nil) {
		t.Fatal("nil should be readable")
	}
	parseErr := errfmt.Newf(ConstStreamFailedToUnmarshalObjectAfterCasDiscovery).Wrap(errfmt.Errorf("yaml: line 46"))
	if !LiveCASBlobUnreadable(parseErr) {
		t.Fatal("expected unmarshal-after-CAS to be unreadable")
	}
	hashErr := errfmt.Errorf("CAS hash mismatch for BLI-X (kind: backlog_item): content hash mismatch: expected abc, got def")
	if !LiveCASBlobUnreadable(hashErr) {
		t.Fatal("expected content hash mismatch to be unreadable")
	}
	if LiveCASBlobUnreadable(errfmt.Errorf("object not found")) {
		t.Fatal("not-found is not an unreadable live blob")
	}
	yamlErr := errfmt.Errorf("failed to parse YAML: yaml: line 2: mapping values are not allowed in this context")
	if !LiveCASBlobUnreadable(yamlErr) {
		t.Fatal("expected YAML parse failure to be an unreadable live blob")
	}
	streamErr := errfmt.Newf("STREAM_FAILED_TO_UNMARSHAL_OBJECT_AFTER_CAS_DISCOVERY").Wrap(errfmt.Errorf("yaml: line 2"))
	if !LiveCASBlobUnreadable(streamErr) {
		t.Fatal("expected STREAM unmarshal token to be an unreadable live blob")
	}
}
