// materialize_agent_chat_channel.go: materialize datacell lite file from CAS agent_feed.
// Command structure from spec: .zqk/cli/specs/system/materialize_agent_chat_channel_command.yaml
package system

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

// NewMaterializeAgentChatChannelCmd wires command builder from spec.
func NewMaterializeAgentChatChannelCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewMaterializeAgentChatChannelCommandBuilder()
	cmd.RunE = runMaterializeAgentChatChannel
	return cmd
}

func runMaterializeAgentChatChannel(cmd *cobra.Command, _ []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		var err error
		_ = err
		root := proc.ProjectRoot()
		if root == emptyValue {
			return errfmt.Errorf("project root not found")
		}
		feedID, err := cmd.Flags().GetString("feed-id")
		if err != nil {
			return err
		}
		if feedID == "" {
			return errfmt.Errorf("--feed-id is required (AGF-*)")
		}
		dryRun, err := cmd.Flags().GetBool("dry-run")
		if err != nil {
			return err
		}

		ctx := proc.OperationContext()
		obj, readErr := proc.Storage().Read(ctx, proc.SecurityContext(), feedID)
		if readErr != nil {
			return errfmt.Newf("read agent_feed").Wrap(readErr)
		}

		cfg, matErr := materializeAgentChatChannelLiteFromObject(obj, time.Now())
		if matErr != nil {
			return errfmt.Newf("materialize lite fields").Wrap(matErr)
		}

		if dryRun {
			return cli.FormatOutput(cmd, map[string]any{
				objects.FieldKeyStatus: "dry_run",
				"feed_id":              cfg.FeedID,
				"preview":              cfg,
			})
		}

		if wErr := datacell.WriteAgentChatChannelConfig(root, cfg); wErr != nil {
			return errfmt.Newf("write agent_chat_channel.json").Wrap(wErr)
		}
		if eErr := datacell.EnsureAgentChatChannelEventsDirForConfig(root, cfg); eErr != nil {
			return errfmt.Newf("ensure events dir").Wrap(eErr)
		}

		logger := logging.GetLoggerFromProfile(proc.Context().Profile)
		detail := "agent_chat_channel feed_id=" + cfg.FeedID
		_ = datacell.EnqueueStewardMaintenance(ctx, root, datacell.ProfileLightFile,
			datacell.MaintenanceOp{Name: datacell.MaintenanceOpRefreshSummary, Detail: detail}, logger)

		outPath := datacell.RuntimeOrganismMembraneReadPaths(root).AgentChatChannelConfigPath()
		logging.Fluent(logger).Info("materialized agent chat channel lite file").
			Path(outPath).
			String("feed_id", cfg.FeedID).
			Log()

		return cli.FormatOutput(cmd, map[string]any{
			objects.FieldKeyStatus: "success",
			"feed_id":              cfg.FeedID,
			"wrote":                outPath,
		})
	})(cmd, nil)
}

func materializeAgentChatChannelLiteFromObject(obj map[string]any, materializedAt time.Time) (datacell.AgentChatChannelConfig, error) {
	if obj == nil {
		return datacell.AgentChatChannelConfig{}, errfmt.Errorf("agent_feed object is nil")
	}
	kind, _ := obj[objects.FieldKeyKind].(string)
	if kind != objects.KindAgentFeed {
		return datacell.AgentChatChannelConfig{}, errfmt.Errorf("expected kind %s, got %q", objects.KindAgentFeed, kind)
	}
	id, _ := obj[objects.FieldKeyID].(string)
	if id == "" {
		return datacell.AgentChatChannelConfig{}, errfmt.Errorf("agent_feed missing id")
	}
	return datacell.AgentChatChannelConfig{
		SchemaVersion:           datacell.AgentChatChannelSchemaVersion,
		Enabled:                 objectMapBool(obj, objects.FieldKeyEnabled),
		DeliveryMode:            objectMapString(obj, objects.FieldKeyDeliveryMode),
		Note:                    objectMapString(obj, objects.FieldKeyNote),
		FeedID:                  id,
		MaterializedAt:          materializedAt.UTC().Format(time.RFC3339),
		ContractSchemaVersion:   objectMapString(obj, objects.FieldKeyContractSchemaVersion),
		ConfigPathOverride:      objectMapString(obj, objects.FieldKeyConfigPathOverride),
		EventsJSONLPathOverride: objectMapString(obj, objects.FieldKeyEventsJsonlPathOverride),
		ProbeToolAllowlist:      objectMapStringSlice(obj, objects.FieldKeyProbeToolAllowlist),
		ProbeCommandSubstrings:  objectMapStringSlice(obj, objects.FieldKeyProbeCommandSubstrings),
	}, nil
}

func objectMapStringSlice(obj map[string]any, key string) []string {
	v, ok := obj[key]
	if !ok || v == nil {
		return nil
	}
	switch x := v.(type) {
	case []string:
		out := make([]string, 0, len(x))
		for _, s := range x {
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			switch t := e.(type) {
			case string:
				s := strings.TrimSpace(t)
				if s != "" {
					out = append(out, s)
				}
			default:
				s := strings.TrimSpace(fmt.Sprint(t))
				if s != "" {
					out = append(out, s)
				}
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	default:
		return nil
	}
}

func objectMapString(obj map[string]any, key string) string {
	v, ok := obj[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func objectMapBool(obj map[string]any, key string) bool {
	v, ok := obj[key]
	if !ok || v == nil {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}
