package utility

import (
	"fmt"
	"sync/atomic"

	"github.com/lanceman/zqk/pkg/cli"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/coordination"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

// ScenarioBuilderEventSubscriber subscribes to scenario builder operational events
// for progress tracking and event logging
type ScenarioBuilderEventSubscriber struct {
	id      string
	active  atomic.Bool
	logger  logging.Logger
	profile string
}

// NewScenarioBuilderEventSubscriber creates a new scenario builder event subscriber
func NewScenarioBuilderEventSubscriber(profile string) *ScenarioBuilderEventSubscriber {
	// Load scenario_builder profile spec to get logging configuration
	profileLoader := cli.NewProfileLoader("")
	scenarioBuilderProfile, err := profileLoader.LoadProfile(ScenarioBuilderProfileName)

	var loggingCtx *pkgctx.LoggingContext
	logLevel := logging.InfoLevel // Default to InfoLevel for scenario builder

	// If profile spec loaded successfully, extract logging level
	if err == nil && scenarioBuilderProfile != nil {
		if loggingConfig, ok := scenarioBuilderProfile.ResolvedSpec["logging"].(map[string]any); ok {
			if levelStr, ok := loggingConfig["level"].(string); ok {
				switch levelStr {
				case string(pkgctx.ProfileDebug):
					logLevel = logging.DebugLevel
				case "info":
					logLevel = logging.InfoLevel
				case "warn":
					logLevel = logging.WarnLevel
				case "error":
					logLevel = logging.ErrorLevel
				}
			}
		}
	}

	// Create LoggingContext based on the base profile, then override with level from spec
	switch profile {
	case ScenarioBuilderProfileName:
		// Use human profile but with InfoLevel from scenario_builder spec
		loggingCtx = pkgctx.NewHumanLoggingContext().WithLevel(int(logLevel))
	case string(pkgctx.ProfileMCP):
		loggingCtx = pkgctx.NewLoggingContext(pkgctx.ProfileMCP).WithLevel(int(logLevel))
	case string(pkgctx.ProfileSystem):
		loggingCtx = pkgctx.NewSystemLoggingContext().WithLevel(int(logLevel))
	case string(pkgctx.ProfileAIAgent):
		loggingCtx = pkgctx.NewLoggingContext(pkgctx.ProfileAIAgent).WithLevel(int(logLevel))
	case string(pkgctx.ProfileDebug):
		loggingCtx = pkgctx.NewLoggingContext(pkgctx.ProfileDebug).WithLevel(int(logLevel))
	case string(pkgctx.ProfileHuman), "":
		// Use level from scenario_builder profile spec (InfoLevel) instead of human default (ErrorLevel)
		loggingCtx = pkgctx.NewHumanLoggingContext().WithLevel(int(logLevel))
	default:
		// Default to human profile with level from spec
		loggingCtx = pkgctx.NewHumanLoggingContext().WithLevel(int(logLevel))
	}

	// Use system context as fallback - this function doesn't have access to command context
	// In practice, this is called during initialization before command context exists
	s := &ScenarioBuilderEventSubscriber{
		id:      "scenario-builder-event-subscriber",
		profile: profile,
		logger:  logging.GetLoggerFromLoggingContext(pkgctx.NewSystemContext(), loggingCtx),
	}
	s.active.Store(true)
	return s
}

// ID returns the subscriber ID
func (s *ScenarioBuilderEventSubscriber) ID() string {
	return s.id
}

// HandleEvent processes scenario builder operational events
func (s *ScenarioBuilderEventSubscriber) HandleEvent(event *coordination.OperationalEvent) error {
	// Only handle scenario_builder events
	if event.OperationType != ScenarioBuilderProfileName {
		return nil // Not a scenario builder event, ignore
	}

	// Extract metadata
	kind := ""
	count := 0
	message := ""
	if event.Metadata != nil {
		if k, ok := event.Metadata[objects.FieldKeyKind].(string); ok {
			kind = k
		}
		if c, ok := event.Metadata["count"].(int); ok {
			count = c
		} else if cFloat, ok := event.Metadata["count"].(float64); ok {
			count = int(cFloat)
		}
		if m, ok := event.Metadata[scenarioBuilderMetadataEvent].(string); ok {
			message = m
		}
	}

	// Debug: verify we're receiving events (temporary - remove after debugging)
	if message == emptyValue {
		// Fallback: try to extract from operation type or status
		message = fmt.Sprintf("Scenario builder %s", event.Status)
	}

	// Log events based on status
	switch event.Status {
	case scenarioBuilderStatusProgress:
		if kind != emptyValue && count > 0 {
			logging.Fluent(s.logger).Info(fmt.Sprintf("Creating %d %s objects...", count, kind)).
				String("operation_type", event.OperationType).
				String("status", event.Status).
				Kind(kind).
				Count(count).
				Log()
		} else if message != emptyValue {
			// Always log progress messages (they're important for user feedback)
			// Use logger.Info which respects the logging level from the profile
			logging.Fluent(s.logger).Info(message).
				String("operation_type", event.OperationType).
				String("status", event.Status).
				Log()
		} else {
			// Fallback: log something even if message is empty
			logging.Fluent(s.logger).Info(fmt.Sprintf("Scenario builder progress: %s", event.Status)).
				String("operation_type", event.OperationType).
				String("status", event.Status).
				Log()
		}
	case scenarioBuilderStatusError:
		errorMsg := message
		if errorMsg == emptyValue {
			errorMsg = "Scenario builder error"
		}
		var err error
		if event.Error != emptyValue {
			err = errfmt.Errorf("%s: %s", errorMsg, event.Error)
		} else {
			err = errfmt.Errorf("%s", errorMsg)
		}
		logging.Fluent(s.logger).Error(errorMsg, err).
			String("operation_type", event.OperationType).
			String("status", event.Status).
			Kind(kind).
			Log()
	case scenarioBuilderStatusWarning:
		warningMsg := message
		if warningMsg == emptyValue {
			warningMsg = "Scenario builder warning"
		}
		logging.Fluent(s.logger).Warn(warningMsg).
			String("operation_type", event.OperationType).
			String("status", event.Status).
			Kind(kind).
			Log()
	case scenarioBuilderStatusComplete:
		logging.Fluent(s.logger).Info("Scenario builder completed").
			String("operation_type", event.OperationType).
			String("status", event.Status).
			Log()
	}

	return nil
}

// EventTypes returns the event types this subscriber is interested in
// Empty slice means subscribe to all operational events
// We filter by OperationType in HandleEvent instead
func (s *ScenarioBuilderEventSubscriber) EventTypes() []string {
	// Subscribe to all operational events, filter by OperationType in HandleEvent
	return []string{}
}

// IsActive returns whether the subscriber is still active
func (s *ScenarioBuilderEventSubscriber) IsActive() bool {
	return s.active.Load()
}

// Deactivate deactivates the subscriber
func (s *ScenarioBuilderEventSubscriber) Deactivate() {
	s.active.Store(false)
}
