package storage

import (
	"errors"
	"testing"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

func TestIsObjectNotFound(t *testing.T) {
	t.Parallel()
	if IsObjectNotFound(nil) {
		t.Fatal("nil is not not-found")
	}
	if !IsObjectNotFound(ErrObjectNotFound) {
		t.Fatal("ErrObjectNotFound")
	}
	if !IsObjectNotFound(errfmt.Newf("failed to read object").Wrap(ErrObjectNotFound)) {
		t.Fatal("wrapped ErrObjectNotFound")
	}
	if !IsObjectNotFound(errors.New("object not found")) {
		t.Fatal("literal object not found")
	}
	if !IsObjectNotFound(errors.New("ID not found in index")) {
		t.Fatal("id not found")
	}
	if IsObjectNotFound(errors.New("permission denied")) {
		t.Fatal("unrelated error")
	}
}

func TestIsExpectedObjectGetMiss(t *testing.T) {
	t.Parallel()
	if !IsExpectedObjectGetMiss(ErrObjectNotFound) {
		t.Fatal("not-found")
	}
	if !IsExpectedObjectGetMiss(errors.New("could not infer kind from ID: AWAIT-1")) {
		t.Fatal("infer kind")
	}
	if !IsExpectedObjectGetMiss(errors.New("failed to read object: could not infer kind from ID: AFE-1")) {
		t.Fatal("wrapped infer kind")
	}
	if IsExpectedObjectGetMiss(errors.New("permission denied")) {
		t.Fatal("unrelated error")
	}
	if IsExpectedObjectGetMiss(nil) {
		t.Fatal("nil")
	}
}

func TestLogObjectReadFailure_nilLogger(t *testing.T) {
	t.Parallel()
	LogObjectReadFailure(nil, ErrObjectNotFound, "ATK-missing")
}
