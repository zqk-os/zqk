package integrity

import (
	"errors"
	"fmt"
)

const (
	errMissingID     = "id is required"
	errEmptyTaskBody = "task body is required"
	errInvalidPrefix = "task id must start with %s, got %s"
	defaultATKPrefix = "ATK-"
	defaultBLIPrefix = "BLI-"
)

var (
	errNilTDE = errors.New("tde cannot be nil") //nolint:errname // standard library pattern
)

// TaskDefEnvelope describes a task definition envelope (TDE).
type TaskDefEnvelope struct {
	ID        string `json:"id"`
	Task      string `json:"task"`
	Kind      string `json:"kind,omitempty"`
	Status    string `json:"status,omitempty"`
	Title     string `json:"title,omitempty"`
	Purpose   string `json:"purpose,omitempty"`
	IsBlocked bool   `json:"is_blocked,omitempty"`
}

// ValidateTDE validates tde: requires non-nil, set id/task/ATK prefix, then applies defaults.
func ValidateTDE(tde *TaskDefEnvelope) error {
	if tde == nil {
		return errNilTDE
	}
	if tde.ID == "" {
		return errors.New(errMissingID)
	}
	if tde.Task == "" {
		return errors.New(errEmptyTaskBody)
	}
	if !validatePrefix(tde.ID, defaultATKPrefix) {
		return fmt.Errorf(errInvalidPrefix, defaultATKPrefix, prefixFromID(tde.ID))
	}
	applyDefault(&tde.Kind, "agent_task")
	applyDefault(&tde.Status, "pending")
	return nil
}

// ValidateBLI validates a BLI-style ID prefix.
func ValidateBLI(id, kind, status string) error {
	if id == "" {
		return errors.New(errMissingID)
	}
	if !validatePrefix(id, defaultBLIPrefix) {
		return fmt.Errorf(errInvalidPrefix, defaultBLIPrefix, prefixFromID(id))
	}
	return nil
}

func validatePrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func prefixFromID(id string) string {
	i := 0
	for i < len(id) && id[i] != '-' {
		i++
	}
	if i == len(id) {
		return ""
	}
	return id[:i+1]
}

// applyDefault sets *ptr to def when *ptr is empty.
func applyDefault(ptr *string, def string) {
	if *ptr == "" {
		*ptr = def
	}
}

// TDEWithDefaults adds sensible defaults to an incoming task envelope.
func TDEWithDefaults(tde *TaskDefEnvelope) *TaskDefEnvelope {
	if tde == nil {
		return &TaskDefEnvelope{Kind: "agent_task", Status: "pending"}
	}
	applyDefault(&tde.Kind, "agent_task")
	applyDefault(&tde.Status, "pending")
	return tde
}
