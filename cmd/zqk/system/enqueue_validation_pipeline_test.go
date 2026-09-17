package system

import "testing"

func TestRunEnqueueValidationForObjectViaPipeline_EmptyInputs(t *testing.T) {
	t.Parallel()

	if err := RunEnqueueValidationForObjectViaPipeline("", "OBJ-1", "backlog_item", "file.yaml"); err != nil {
		t.Fatalf("expected nil error for empty projectRoot, got %v", err)
	}
	if err := RunEnqueueValidationForObjectViaPipeline("test-root", "", "backlog_item", "file.yaml"); err != nil {
		t.Fatalf("expected nil error for empty objectID, got %v", err)
	}
}
