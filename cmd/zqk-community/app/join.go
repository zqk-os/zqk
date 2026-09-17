package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/federation"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/spf13/cobra"
)

func NewJoinCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewJoinCommandBuilder()
	cli.BindAsyncProgress(cmd, runJoin)
	return cmd
}

func runJoin(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		peerURL := args[0]
		alias, _ := cmd.Flags().GetString("alias")
		interactive, _ := cmd.Flags().GetBool("interactive")

		var err error
		_ = err

		logger := proc.Logger()
		cyan := color.New(color.FgCyan).SprintFunc()
		green := color.New(color.FgGreen).SprintFunc()
		yellow := color.New(color.FgYellow).SprintFunc()

		// 1. RESOLVE LOCAL IDENTITY
		fmt.Fprintf(cmd.OutOrStdout(), "%s Initializing Sovereign Identity...\n", cyan("➤"))
		idManager := federation.NewIdentityManager(proc.ProjectRoot())
		kernelID, err := idManager.GetKernelID()
		if err != nil {
			return errfmt.Newf("failed to resolve kernel identity").Wrap(err)
		}
		publicKey, _ := idManager.GetPublicKey()

		// 2. IDENTIFY & HANDSHAKE
		fmt.Fprintf(cmd.OutOrStdout(), "%s Connecting to %s...\n", cyan("➤"), peerURL)
		time.Sleep(500 * time.Millisecond) // UX beat

		// Prepare local capabilities (hardcoded for now, future: dynamic discovery)
		caps := []federation.Capability{
			{Kind: "skill", ID: "storage", Name: "Object Storage"},
			{Kind: "skill", ID: "compute", Name: "Goroutine Pool"},
		}

		// Use hardened handshaker with default local transport
		handshaker := federation.NewMCPHandshaker(kernelID, publicKey, nil)

		req := federation.HandshakeRequest{
			ProtocolVersion: "1.0.0",
			Capabilities:    caps,
			Timestamp:       time.Now(),
		}

		resp, err := handshaker.Initiate(proc.OperationContext(), peerURL, req)
		if err != nil {
			return errfmt.Newf("failed to join peer").Wrap(err)
		}

		if !resp.Accepted {
			return errfmt.Errorf("peer declined handshake: %s", resp.Message)
		}

		// 3. VERIFY & DISCOVER
		fmt.Fprintf(cmd.OutOrStdout(), "%s Peer identified: %s\n", green("✔"), cyan(resp.KernelID))
		fmt.Fprintf(cmd.OutOrStdout(), "%s Trust Level: %s\n", yellow("ℹ"), cyan("Verified (FHP v1.0)"))

		fmt.Fprintf(cmd.OutOrStdout(), "\n%s Advertised Capabilities:\n", yellow("✦"))
		for _, c := range resp.Capabilities {
			fmt.Fprintf(cmd.OutOrStdout(), "  - %s (%s): %s\n", cyan(c.ID), yellow(c.Kind), c.Name)
		}

		// Suggest alias if not provided
		if alias == EmptyValue {
			alias = strings.ToLower(resp.KernelID)
			alias = strings.TrimPrefix(alias, "rem-")
			alias = strings.TrimPrefix(alias, "ker-")
			// Handle nested prefixes
			parts := strings.Split(alias, "-")
			if len(parts) > 1 {
				alias = parts[len(parts)-1]
			}
		}

		// 4. CONFIRM (Interactive)
		if interactive {
			fmt.Fprintf(cmd.OutOrStdout(), "\nJoin this peer as %s? [Y/n]: ", cyan(alias))
			var input string
			_ , _ = fmt.Scanln(&input)
			if input != "" && !strings.EqualFold(input, "y") && !strings.EqualFold(input, "yes") {
				fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
				return nil
			}
		}

		// 5. REGISTER
		fmt.Fprintf(cmd.OutOrStdout(), "%s Registering %s in local kernel...\n", cyan("➤"), cyan(resp.KernelID))

		remoteState := &federation.RemoteKernelState{
			ID:            resp.KernelID,
			Endpoint:      peerURL,
			PublicKey:     resp.PublicKey,
			Capabilities:  resp.Capabilities,
			LastHandshake: time.Now(),
		}

		// Use architectural ToMap for registration
		peerObj := remoteState.ToMap()
		if alias != EmptyValue {
			peerObj[objects.FieldKeyTitle] = fmt.Sprintf("Peer: %s (%s)", resp.KernelID, alias)
		}

		// Use synchronous creation
		syncCtx := storage.WithSyncCreateForKind(proc.OperationContext(), objects.KindRemoteKernel)
		peerID := peerObj[objects.FieldKeyID].(string)
		if err := proc.Storage().Create(syncCtx, proc.SecurityContext(), peerObj); err != nil {
			// Treat as Upsert: if Create fails, attempt Update
			if updateErr := proc.Storage().Update(syncCtx, proc.SecurityContext(), peerID, peerObj); updateErr != nil {
				return errfmt.Newf("failed to update registered peer").Wrap(updateErr)
			}
		}

		fmt.Fprintf(cmd.OutOrStdout(), "%s Success! Welcome to the Sovereign Mesh.\n", green("✔"))
		logging.FluentEvent(logger).Info("Joined Sovereign Mesh").
			String("peer_url", peerURL).
			String("peer_id", resp.KernelID).
			String("alias", alias).
			Log()

		return nil
	})(cmd, args)
}
