package contextcmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/sessionpaging"
)

// NewContextCmd creates stranger-facing cold episodic snapshot/recall commands.
// TRACK: https://github.com/zqk-os/zqk/pull/1215 — cold episodic CLI surface.
func NewContextCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewContextCommandBuilder(), &cobra.Command{})
	cmd.AddCommand(newSnapshotCmd())
	cmd.AddCommand(newRecallCmd())
	return cmd
}

func newSnapshotCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewContextSnapshotCommandBuilder(), &cobra.Command{
		RunE: func(cmd *cobra.Command, _ []string) error {
			var flags clipkg.FlagBag
			sessionID := flags.String(cmd, "session-id")
			pageID := flags.String(cmd, "page-id")
			note := flags.String(cmd, "note")
			if err := flags.Err(); err != nil {
				return err
			}
			return runSnapshot(cmd, sessionID, pageID, note)
		},
	})
	_ = cmd.MarkFlagRequired("session-id")
	_ = cmd.MarkFlagRequired("page-id")
	return cmd
}

func newRecallCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewContextRecallCommandBuilder(), &cobra.Command{
		RunE: func(cmd *cobra.Command, _ []string) error {
			var flags clipkg.FlagBag
			sessionID := flags.String(cmd, "session-id")
			episodeID := flags.String(cmd, "episode-id")
			if err := flags.Err(); err != nil {
				return err
			}
			return runRecall(cmd, sessionID, episodeID)
		},
	})
	_ = cmd.MarkFlagRequired("session-id")
	_ = cmd.MarkFlagRequired("episode-id")
	return cmd
}

func runSnapshot(cmd *cobra.Command, sessionID, pageID, note string) error {
	root := paths.ResolveProjectRoot(".")
	hot, err := sessionpaging.NewStore(root)
	if err != nil {
		return err
	}
	cold, err := sessionpaging.NewColdStore(root)
	if err != nil {
		return err
	}
	payload, err := sessionpaging.EvictHotToCold(hot, cold, sessionID, pageID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(note) != "" {
		annotated := append(append([]byte{}, payload...), []byte("\n# note: "+note)...)
		if err := cold.Put(sessionID, pageID, annotated); err != nil {
			return err
		}
		payload = annotated
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "snapshot ok session=%s episode=%s bytes=%d\n", sessionID, pageID, len(payload))
	return err
}

func runRecall(cmd *cobra.Command, sessionID, episodeID string) error {
	root := paths.ResolveProjectRoot(".")
	cold, err := sessionpaging.NewColdStore(root)
	if err != nil {
		return err
	}
	payload, err := cold.Recall(sessionID, episodeID)
	if err != nil {
		return err
	}
	w := cmd.OutOrStdout()
	if _, err := w.Write(payload); err != nil {
		return err
	}
	if len(payload) == 0 || payload[len(payload)-1] != '\n' {
		_, _ = io.WriteString(w, "\n")
	}
	return nil
}
