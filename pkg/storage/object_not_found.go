package storage

import (
	"errors"
	"strings"

	"github.com/lanceman/zqk/pkg/logging"
)

const (
	logMsgObjectNotFound     = "Object not found"
	logMsgFailedToReadObject = "Failed to read object"
)

// IsObjectNotFound reports whether err is a missing-object miss (not a crash).
func IsObjectNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrObjectNotFound) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "object not found") || strings.Contains(msg, "id not found")
}

// IsExpectedObjectGetMiss reports a client get miss: missing object or an ID
// that is not a kernel kind (AFE-/AWAIT- feed ids). Not a storage crash.
func IsExpectedObjectGetMiss(err error) bool {
	if IsObjectNotFound(err) {
		return true
	}
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "could not infer kind")
}

// LogObjectReadFailure logs a Read miss as Warn and other read failures as Error.
// Used by object get (CLI and internal) so both paths share one severity rule.
func LogObjectReadFailure(el *logging.EventLogger, err error, id string) {
	if el == nil {
		return
	}
	if IsExpectedObjectGetMiss(err) {
		logging.FluentEvent(el).Warn(logMsgObjectNotFound).
			WithError(err).
			ObjectID(id).
			Log()
		return
	}
	logging.FluentEvent(el).Error(logMsgFailedToReadObject, err).
		ObjectID(id).
		Log()
}
