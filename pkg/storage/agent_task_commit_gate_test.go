package storage

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestAgentTaskWorkDoneRequiresCommit(t *testing.T) {
	t.Parallel()
	if !agentTaskWorkDoneRequiresCommit(objects.ObjectStatusImplemented) {
		t.Fatal("implemented is work-done and must require commit_hash")
	}
	if agentTaskWorkDoneRequiresCommit(objects.ObjectStatusArchived) {
		t.Fatal("archived is abandon; leftover ATKs have no commit_hash")
	}
	if agentTaskWorkDoneRequiresCommit(objects.ObjectStatusError) {
		t.Fatal("error is halted, not work-done")
	}
	if agentTaskWorkDoneRequiresCommit(objects.ObjectStatusInProgress) {
		t.Fatal("in_progress is not work-done")
	}
	if agentTaskWorkDoneRequiresCommit(objects.ObjectStatusApproved) {
		t.Fatal("approved is not work-done")
	}
}
