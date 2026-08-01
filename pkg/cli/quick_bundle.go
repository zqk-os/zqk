package cli

import (
	"fmt"

	"io"

	"github.com/fatih/color"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"gopkg.in/yaml.v3"
)

type QuickBundleData struct {
	BacklogItem map[string]any   `yaml:"backlog_item" json:"backlog_item"`
	Criteria    []map[string]any `yaml:"criteria" json:"criteria"`
	AgentTasks  []map[string]any `yaml:"agent_tasks" json:"agent_tasks"`
}

type ObjectCreator func(kind string, data map[string]any) (string, error)

// ProcessQuickBundle contains the native logic to create and link bundle objects.
func ProcessQuickBundle(data []byte, createObj ObjectCreator, out io.Writer, logger *logging.EventLogger) error {
	var bundle QuickBundleData
	if err := yaml.Unmarshal(data, &bundle); err != nil {
		return errfmt.Errorf("failed to parse bundle: %w", err)
	}

	if bundle.BacklogItem == nil {
		return errfmt.Errorf("bundle must contain a backlog_item")
	}

	if out != nil {
		out.Write([]byte("Verifying bundle contents...\n"))
	}

	// Create BacklogItem
	bliData := bundle.BacklogItem
	bliData[objects.FieldKeyKind] = objects.KindBacklogItem

	bliID, err := createObj(objects.KindBacklogItem, bliData)
	if err != nil {
		return errfmt.Errorf("failed to create backlog item: %w", err)
	}
	if bliID == "" {
		return errfmt.Errorf("unable to verify creation: backlog item ID is empty")
	}
	if logger != nil {
		logging.FluentEvent(logger).Info("Backlog item created from bundle").ObjectID(bliID).Log()
	}
	if out != nil {
		out.Write([]byte(fmt.Sprintf("%s Backlog item created: %s\n", color.GreenString("✓"), bliID)))
	}

	// Create Criteria
	var criteriaIDs []string
	for i, critData := range bundle.Criteria {
		critData[objects.FieldKeyKind] = objects.KindCriteria
		appendRef(critData, "backlog_item_refs", bliID)

		critID, err := createObj(objects.KindCriteria, critData)
		if err != nil {
			return errfmt.Errorf("failed to create criteria %d: %w", i, err)
		}
		if critID == "" {
			return errfmt.Errorf("unable to verify creation: criteria ID is empty")
		}
		criteriaIDs = append(criteriaIDs, critID)
		if logger != nil {
			logging.FluentEvent(logger).Info("Criteria created from bundle").ObjectID(critID).Log()
		}
		if out != nil {
			out.Write([]byte(fmt.Sprintf("%s Criteria created: %s\n", color.GreenString("✓"), critID)))
		}
	}

	// Create AgentTasks
	for i, taskData := range bundle.AgentTasks {
		taskData[objects.FieldKeyKind] = objects.KindAgentTask
		appendRef(taskData, "backlog_item_refs", bliID)

		for _, cID := range criteriaIDs {
			appendRef(taskData, "related_object_refs", cID)
		}

		taskID, err := createObj(objects.KindAgentTask, taskData)
		if err != nil {
			return errfmt.Errorf("failed to create agent_task %d: %w", i, err)
		}
		if taskID == "" {
			return errfmt.Errorf("unable to verify creation: agent_task ID is empty")
		}
		if logger != nil {
			logging.FluentEvent(logger).Info("Agent task created from bundle").ObjectID(taskID).Log()
		}
		if out != nil {
			out.Write([]byte(fmt.Sprintf("%s Agent task created: %s\n", color.GreenString("✓"), taskID)))
		}
	}

	return nil
}

func appendRef(data map[string]any, field string, refID string) {
	if data[field] == nil {
		data[field] = []any{refID}
		return
	}
	switch v := data[field].(type) {
	case []any:
		data[field] = append(v, refID)
	case []string:
		var ifaces []any
		for _, s := range v {
			ifaces = append(ifaces, s)
		}
		data[field] = append(ifaces, refID)
	}
}
