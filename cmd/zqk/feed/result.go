package feed

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/objects"
)

// feedResult builds a concise, format-friendly command result.
// Table/json/yaml get status + identifiers only. Verbose adds event_path and peer_wake detail.
func feedResult(cmd *cobra.Command, res agentfeed.AppendEventResult, wake *agentfeed.PeerWakeResult) map[string]any {
	out := map[string]any{
		objects.FieldKeyStatus:       "success",
		agentfeed.JSONFieldFeedID:    res.FeedID,
		objects.FieldKeyDeliveryMode: res.DeliveryMode,
		agentfeed.JSONFieldEventID:   res.EventID,
	}
	verbose := cli.IsVerbose(cmd)
	if verbose {
		out[agentfeed.JSONFieldEventPath] = res.EventPath
		if res.Event != nil {
			if msg, ok := res.Event[agentfeed.JSONFieldMessage].(string); ok && strings.TrimSpace(msg) != "" {
				out[agentfeed.JSONFieldMessage] = truncateRunes(msg, 120)
			}
		}
	}
	if wake != nil {
		switch {
		case wake.Error != "":
			out["peer_wake"] = "failed"
			out["peer_wake_unrepaired"] = true
			if verbose {
				out["peer_wake_error"] = wake.Error
			}
		case wake.Attempted:
			out["peer_wake"] = "attempted"
			if wake.DeliveryReceipt {
				out["delivery_receipt"] = true
				out["delivery_event_id"] = wake.DeliveryEventID
			}
			if wake.IdeBridgeQueued {
				out["ide_bridge_queued"] = true
			}
			if !wake.Live {
				out["peer_wake_unrepaired"] = true
				out["peer_wake_live"] = false
				if verbose && wake.Transport != "" {
					out["peer_wake_transport"] = wake.Transport
				}
			} else {
				out["peer_wake_live"] = true
			}
		case wake.Skipped != "":
			out["peer_wake"] = "skipped"
			out["peer_wake_unrepaired"] = true
			if verbose {
				out["peer_wake_reason"] = wake.Skipped
			}
		}
	}
	return out
}

func truncateRunes(s string, max int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= max {
		return string(r)
	}
	return string(r[:max]) + "…"
}
