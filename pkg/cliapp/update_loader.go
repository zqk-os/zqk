package cli

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
)

// LoadUpdatesFromFile loads updates from a file path using DataLoader.
func (dl *DataLoader) LoadUpdatesFromFile(filePath string) (map[string]any, error) {
	if filePath == "" {
		return nil, nil
	}
	data, _, err := dl.LoadFromFile(filePath)
	return data, err
}

// LoadUpdatesFromData loads updates from an inline data string using DataLoader.
func (dl *DataLoader) LoadUpdatesFromData(dataStr string) (map[string]any, error) {
	if dataStr == "" {
		return nil, nil
	}
	data, _, err := dl.LoadFromString(dataStr)
	return data, err
}

// ApplyAutoStatusFlag checks --auto-status and derives the next lifecycle status if applicable.
func ApplyAutoStatusFlag(cmd *cobra.Command, currentObj map[string]any, updates map[string]any) error {
	var flagsBag clipkg.FlagBag
	autoStatus := flagsBag.Bool(cmd, "auto-status")
	if flagsBag.Err() != nil || !autoStatus {
		return flagsBag.Err()
	}
	if currentObj == nil {
		return errfmt.Errorf("--auto-status requires an existing object")
	}
	if _, hasStatus := updates[objects.FieldKeyStatus]; hasStatus {
		return errfmt.Errorf("--auto-status cannot be combined with an explicit status update")
	}
	kind, _ := currentObj[objects.FieldKeyKind].(string)
	hasTrait, err := objects.KindHasTrait(kind, "auto_status_transitionable")
	if err != nil {
		return errfmt.Newf("failed to evaluate auto-status trait for kind %q", kind).Wrap(err)
	}
	if !hasTrait {
		return errfmt.Errorf("--auto-status not supported for kind %q (missing auto_status_transitionable trait)", kind)
	}
	currentStatus, _ := currentObj[objects.FieldKeyStatus].(string)
	next, err := objects.NextProgressLifecycleStatus(kind, currentStatus)
	if err != nil {
		return err
	}
	updates[objects.FieldKeyStatus] = next
	return nil
}
