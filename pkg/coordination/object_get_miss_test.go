package coordination

import (
	"errors"
	"testing"
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
	if !expectedObjectGetMiss(errors.New("unauthorized: missing token in ~/.zqk/credentials or ZQK_API_KEY")) {
		t.Fatal("missing token")
	}
}
