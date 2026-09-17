package storage

import "github.com/lanceman/zqk/pkg/logging"

// StorageLog starts a pooled fluent log builder for POL-CODE-007 storage runtime logs (same backing
// type as [scheduler.SLog]). Prefer StorageLog(logger).Warn(msg).Field().Log() over variadic
// Logger.Warn(msg, logging.String(...), ...) when multiple fields attach to one event.
func StorageLog(logger logging.Logger) *logging.FluentRoot {
	return logging.Fluent(logger)
}
