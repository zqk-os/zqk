package reports

import "github.com/zqk-os/zqk/pkg/objects"

const (
	quickKindQuestion  = objects.KindQuestion
	quickKindMilestone = objects.KindMilestone
	quickKindBacklog   = objects.KindBacklogItem
	quickKindCodeRef   = objects.KindCodeReference
)

const (
	quickSchemaV2         = objects.DefaultSchemaVersion
	quickStatusResolved   = "resolved"
	quickStatusDeferred   = "deferred"
	quickStatusError      = "error"
	quickStatusComplete   = "complete"
	quickStatusArchived   = "archived"
	quickStatusInProgress = "in_progress"
	quickStatusOpen       = "open"
	quickStatusValidated  = "validated"
	quickStatusExploring  = "exploring"
)

const (
	quickPriorityP0 = "P0"
	quickFormatJSON = "json"
)
