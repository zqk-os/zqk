package contextcmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/sessionpaging"
)

// NewContextCmd creates stranger-facing cold episodic snapshot/recall commands.
// TRACK: REQ-1795568387221360000-3729129a — cold episodic CLI surface.
func NewContextCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Cold episodic context snapshot and recall",
		Long: `Snapshot hot session pages into the cold episodic store, and recall
facts after soft-limit eviction (CONTEXT_PAGING_DESIGN.md v0.2).

Examples:
  zqk context snapshot --session-id agent-a --page-id page-0
  zqk context recall --session-id agent-a --episode-id page-0
`,
	}
	cmd.AddCommand(newSnapshotCmd())
	cmd.AddCommand(newRecallCmd())
	return cmd
}

func newSnapshotCmd() *cobra.Command {
	var sessionID, pageID, note string
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Evict a hot session page into cold episodic storage",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSnapshot(cmd, sessionID, pageID, note)
		},
	}
	cmd.Flags().StringVar(&sessionID, "session-id", "", "Session id owning the hot page")
	cmd.Flags().StringVar(&pageID, "page-id", "", "Hot page id to snapshot into cold storage")
	cmd.Flags().StringVar(&note, "note", "", "Optional note appended to the cold payload")
	_ = cmd.MarkFlagRequired("session-id")
	_ = cmd.MarkFlagRequired("page-id")
	return cmd
}

func newRecallCmd() *cobra.Command {
	var sessionID, episodeID string
	cmd := &cobra.Command{
		Use:   "recall",
		Short: "Recall a cold episodic fact by session and episode id",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runRecall(cmd, sessionID, episodeID)
		},
	}
	cmd.Flags().StringVar(&sessionID, "session-id", "", "Session id that owns the cold episode")
	cmd.Flags().StringVar(&episodeID, "episode-id", "", "Cold episode id (often the former hot page id)")
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
