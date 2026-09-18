package scheduler

import (
	"github.com/zqk-os/zqk/pkg/objects"
)

func (j *ScheduledJob) ToMap() map[string]any {
	return map[string]any{
		objects.FieldKeyID:          j.ID,
		objects.FieldKeyJobType:     j.JobType,
		objects.FieldKeyCategory:    j.Category,
		objects.FieldKeyTitle:       j.Title,
		objects.FieldKeyDescription: j.Description,
		objects.FieldKeyTriggerType: j.TriggerType,
		objects.FieldKeyEnabled:     j.Enabled,
		objects.FieldKeyPriority:    j.Priority,
	}
}
