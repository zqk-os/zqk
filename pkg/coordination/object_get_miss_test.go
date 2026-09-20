package coordination

import (
	"errors"
	"fmt"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestExpectedObjectGetMiss(t *testing.T) {
	t.Parallel()
	if !expectedObjectGetMiss(errors.New("could not infer kind from ID: AWAIT-1")) {
		t.Fatal("infer kind")
	}
	if !expectedObjectGetMiss(errors.New("failed to read object: object not found")) {
		t.Fatal("not found")
	}
	if expectedObjectGetMiss(errors.New("permission denied")) {
		t.Fatal("unrelated")
	}
	if expectedObjectGetMiss(nil) {
		t.Fatal("nil")
	}
	if !expectedObjectGetMiss(errors.New("unauthorized: account ACC-system not found in account index")) {
		t.Fatal("acc-system")
	}
	expectedAuthErr := fmt.Sprintf("unauthorized: missing token in ~/%s/credentials or %s", paths.ProjectDataDir, zqkenv.APIKey())
	if !expectedObjectGetMiss(errors.New(expectedAuthErr)) {
		t.Fatal("missing token")
	}
}
