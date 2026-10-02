package feed

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/mcp"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// requireProjectRoot ensures the project root can be resolved, returning a standardized error otherwise.
func requireProjectRoot(proc *cli.Processor) (string, error) {
	if proc == nil {
		return "", errfmt.Errorf("processor is nil")
	}
	root := proc.ProjectRoot()
	if root == "" {
		return "", errfmt.Errorf("project root not found")
	}
	return root, nil
}

// withFeedRoot wraps cli.WithProcessor and extracts the verified project root and a fresh FlagBag.
func withFeedRoot(fn func(cmd *cobra.Command, proc *cli.Processor, root string, flags *clipkg.FlagBag) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
			root, err := requireProjectRoot(proc)
			if err != nil {
				return err
			}
			var flags clipkg.FlagBag
			return fn(cmd, proc, root, &flags)
		})(cmd, args)
	}
}

// validateSeatAndPersona verifies that agentID and personaRef are non-empty and distinct.
func validateSeatAndPersona(agentID, personaRef string) error {
	if personaRef == "" {
		return errfmt.Errorf("--persona-ref is required (kernel persona PER-*)")
	}
	if agentID == "" {
		return errfmt.Errorf("--agent-id is required (unique swarm seat; not the persona id)")
	}
	if strings.EqualFold(agentID, personaRef) {
		return errfmt.Errorf("--agent-id must differ from --persona-ref (seat vs kernel persona)")
	}
	return nil
}

// validatePersonaRef reads and verifies that the given personaRef resolves to a valid persona object.
func validatePersonaRef(proc *cli.Processor, personaRef string) (map[string]any, error) {
	return resolveAndValidatePersona(proc, personaRef, "")
}

// resolveAndValidatePersona reads and verifies that the given personaRef resolves to a valid persona object.
// If opName is non-empty, error messages are formatted as "feed <opName>: resolve persona <ref>".
func resolveAndValidatePersona(proc *cli.Processor, personaRef, opName string) (map[string]any, error) {
	if proc == nil {
		return nil, errfmt.Errorf("processor is nil")
	}
	persona, err := proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), personaRef)
	if err != nil {
		if opName != "" {
			return nil, errfmt.Newf("feed %s: resolve persona %s", opName, personaRef).Wrap(err)
		}
		return nil, errfmt.Newf("resolve persona %s", personaRef).Wrap(err)
	}
	kind, _ := persona[objects.FieldKeyKind].(string)
	if !strings.EqualFold(strings.TrimSpace(kind), "persona") {
		return nil, errfmt.Errorf("--persona-ref %q is kind %q (want persona)", personaRef, kind)
	}
	return persona, nil
}

// validateUsablePersonaStatus verifies the persona status is active/approved/implemented and not archived/error/rejected/deprecated.
func validateUsablePersonaStatus(persona map[string]any, personaRef string) error {
	status, _ := persona[objects.FieldKeyStatus].(string)
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "approved", "implemented", "active":
		return nil
	case "archived", "error", "rejected", "deprecated":
		return errfmt.Errorf("persona %s has status %q (not usable for mesh stamp)", personaRef, status)
	default:
		if strings.TrimSpace(status) == "" {
			return errfmt.Errorf("persona %s has empty status", personaRef)
		}
		return nil
	}
}

// newPersonaFeedResult initializes a feed result populated with persona and agent IDs.
func newPersonaFeedResult(cmd *cobra.Command, res agentfeed.AppendEventResult, personaRef, agentID string) map[string]any {
	out := feedResult(cmd, res, nil)
	out[objects.FieldKeyPersonaRef] = personaRef
	out[objects.FieldKeyAgentID] = agentID
	return out
}

// dispatchPeerWake performs the shared MCP IPC probe and peer wake dispatch.
func dispatchPeerWake(
	ctx context.Context,
	logger logging.Logger,
	root string,
	message string,
	res agentfeed.AppendEventResult,
	agentID string,
	toAgentID string,
	noWake bool,
	opName string,
) (*agentfeed.PeerWakeResult, int) {
	var wake agentfeed.PeerWakeResult
	var wakePtr *agentfeed.PeerWakeResult
	mcpSubscribers := -1
	mcpSubscribersProbed := false
	mcpIPCDelivered := false

	if !zqkenv.IsCommunityEdition && !noWake && agentfeed.ShouldWakePeer(res.DeliveryMode) {
		if res.DeliveryMode == datacell.DeliveryModeNotify {
			probe := mcp.ProbeFeedSteerMCPWake(ctx, "", message, agentID, res.EventID, logger)
			mcpIPCDelivered = probe.IPCDelivered
			mcpSubscribersProbed = probe.SubscribersProbed
			mcpSubscribers = probe.SubscriberCount
			if probe.PublishErr == nil {
				logging.Fluent(logger).Info("feed " + opName + " MCP daemon IPC wake delivered").
					String("event_id", res.EventID).
					Log()
			} else {
				logging.Fluent(logger).Warn("feed " + opName + " MCP daemon IPC publish failed; continuing with wake script").
					String("error", probe.PublishErr.Error()).
					Log()
			}
			if probe.QueryErr != nil {
				logging.Fluent(logger).Warn("feed " + opName + " MCP subscriber count query failed").
					String("error", probe.QueryErr.Error()).
					Log()
			} else if probe.SubscribersProbed {
				logging.Fluent(logger).Info("feed " + opName + " MCP subscriber count").
					Int("subscriber_count", probe.SubscriberCount).
					Log()
			}
		}

		wake = agentfeed.WakePeerOpts(ctx, agentfeed.WakePeerOptions{
			ProjectRoot:          root,
			Message:              message,
			InReplyTo:            res.EventID,
			FromAgentID:          agentID,
			DeliveryMode:         res.DeliveryMode,
			ToAgentID:            toAgentID,
			MCPIPCDelivered:      mcpIPCDelivered,
			MCPSubscribersProbed: mcpSubscribersProbed,
			MCPSubscriberCount:   mcpSubscribers,
		})
		wakePtr = &wake
		if unrepaired, reason, detail := agentfeed.IsUnrepairedWake(wake); unrepaired {
			_ = agentfeed.AlertUnrepairedWake(res.EventID, detail, reason)
			logging.Fluent(logger).Warn("feed " + opName + " peer wake unrepaired (event still appended; human alerted)").
				String("reason", string(reason)).
				String("detail", detail).
				String("transport", wake.Transport).
				Path(wake.Script).
				Log()
		} else if wake.Attempted {
			logging.Fluent(logger).Info("feed " + opName + " peer wake attempted").
				Path(wake.Script).
				String("transport", wake.Transport).
				String("delivery_receipt", fmt.Sprintf("%v", wake.DeliveryReceipt)).
				Bool("live", wake.Live).
				Log()
		}
	} else if zqkenv.IsCommunityEdition && !noWake && agentfeed.ShouldWakePeer(res.DeliveryMode) {
		logging.Fluent(logger).Info("feed " + opName + ": community edition skips Studio mesh wake membrane").
			String("delivery_mode", res.DeliveryMode).
			Log()
	}

	return wakePtr, mcpSubscribers
}

// formatPeerWakeOutput formats the final CLI output map for steer/wake results.
func formatPeerWakeOutput(
	cmd *cobra.Command,
	res agentfeed.AppendEventResult,
	wakePtr *agentfeed.PeerWakeResult,
	awaitID string,
	mcpSubscribers int,
) error {
	out := feedResult(cmd, res, wakePtr)
	if awaitID != "" {
		out["peer_ack_await_id"] = awaitID
		out["await_peer_ack"] = true
	}
	if mcpSubscribers >= 0 {
		out["mcp_subscribers"] = mcpSubscribers
	}
	if wakePtr != nil && wakePtr.Transport != "" {
		out["peer_wake_transport"] = wakePtr.Transport
	}
	return cli.FormatOutput(cmd, out)
}
