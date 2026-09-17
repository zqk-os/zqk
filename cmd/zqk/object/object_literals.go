package object

import "github.com/lanceman/zqk/pkg/objects"

const (
	objectFormatJSON  = "json"
	objectFormatYAML  = "yaml"
	objectFormatTable = "table"
	objectSchemaV2    = objects.DefaultSchemaVersion

	objectProfileQuiet = "quiet"

	objectKindLifecycle    = objects.KindLifecycle
	objectKindObjectSpec   = objects.KindObjectSpec
	objectKindPriorityPlan = objects.KindPriorityPlan

	objectStatusInProgress = "in_progress"
	objectStatusActive     = "active"
	objectStatusComplete   = "complete"
	objectStatusArchived   = "archived"
	objectStatusClosed     = "closed"
	objectStatusCancelled  = "cancelled"
	objectStatusExploring  = "exploring"
	objectStatusValidated  = "validated"
	objectStatusPlanned    = "planned"
	objectStatusNotStarted = "not_started"
	objectStatusDraft      = "draft"
	objectStatusFinal      = "final"
)
