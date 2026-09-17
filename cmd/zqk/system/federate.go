package system

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/federation"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/spf13/cobra"
)

// NewFederateCmd creates the system federate command group
func NewFederateCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSystemFederateCommandBuilder()

	cmd.AddCommand(NewHandshakeCmd())
	cmd.AddCommand(NewInitiateCmd())
	cmd.AddCommand(NewFederateStatusCmd())

	return cmd
}

// NewFederateStatusCmd creates the system federate status command
func NewFederateStatusCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSystemFederateStatusCommandBuilder()
	cmd.RunE = runFederateStatus
	return cmd
}

func runFederateStatus(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		var err error
		_ = err
		ctx := proc.OperationContext()
		sp := proc.Storage()
		secCtx := proc.SecurityContext()

		filter := storage.ListFilter{Kind: objects.KindRemoteKernel, Limit: 0}
		result, err := sp.List(ctx, secCtx, proc.StorageContext(), filter)
		if err != nil {
			return errfmt.Newf("failed to list federated nodes").Wrap(err)
		}

		if len(result.Objects) == 0 {
			_ = cli.WriteOutput(cmd, []byte("No federated connections established. Your kernel is currently autonomous and sovereign.\n"))
			return nil
		}

		out := "ZQK Federated Sovereign Mesh Status\n"
		out += "==================================\n\n"

		for _, obj := range result.Objects {
			id, _ := obj[objects.FieldKeyID].(string)
			title, _ := obj[objects.FieldKeyTitle].(string)
			endpoint, _ := obj[objects.FieldKeyEndpoint].(string)
			trust, _ := obj[objects.FieldKeyTrustLevel].(string)
			caps, _ := obj[objects.FieldKeyCapabilities].([]any)
			last, _ := obj[objects.FieldKeyLastHeartbeat].(string)

			out += fmt.Sprintf("Node: %s (%s)\n", title, id)
			out += fmt.Sprintf("  Endpoint:   %s\n", endpoint)
			out += fmt.Sprintf("  Trust:      %s\n", trust)
			out += fmt.Sprintf("  Last Sync:  %s\n", last)
			out += fmt.Sprintf("  Capabilities: %d shared\n", len(caps))
			out += "\n"
		}

		return cli.WriteOutput(cmd, []byte(out))
	})(cmd, args)
}

// NewInitiateCmd creates the system federate initiate command
func NewInitiateCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSystemFederateInitiateCommandBuilder()
	cmd.RunE = runInitiate
	return cmd
}

func runInitiate(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		var err error
		_ = err
		ctx := proc.OperationContext()
		remoteEndpoint := args[0]

		// 1. Resolve local identity
		idManager := federation.NewIdentityManager(proc.ProjectRoot())
		kernelID, err := idManager.GetKernelID()
		if err != nil {
			return err
		}
		publicKey, _ := idManager.GetPublicKey()

		// 2. Prepare local handshake request
		req := federation.HandshakeRequest{
			ProtocolVersion: "1.0",
			KernelID:        kernelID,
			Namespace:       "zqk:kernel",
			PublicKey:       publicKey,
			Timestamp:       time.Now(),
		}

		// 3. Execute Initiate
		handshaker := federation.NewMCPHandshaker(kernelID, publicKey, nil)
		resp, err := handshaker.Initiate(ctx, remoteEndpoint, req)
		if err != nil {
			return err
		}

		if !resp.Accepted {
			return errfmt.Errorf("federation handshake declined: %s", resp.Message)
		}

		// 4. Persist the remote kernel object from response
		sp := proc.Storage()
		secCtx := proc.SecurityContext()

		remoteState := &federation.RemoteKernelState{
			ID:            resp.KernelID,
			Endpoint:      remoteEndpoint,
			PublicKey:     resp.PublicKey,
			Capabilities:  resp.Capabilities,
			LastHandshake: time.Now(),
		}

		// Use synchronous creation for immediate visibility in the mesh
		syncCtx := storage.WithSyncCreateForKind(ctx, objects.KindRemoteKernel)
		peerObj := remoteState.ToMap()
		peerID := peerObj[objects.FieldKeyID].(string)

		if err := sp.Create(syncCtx, secCtx, peerObj); err != nil {
			// Treat as Upsert: if Create fails, attempt Update
			if updateErr := sp.Update(syncCtx, secCtx, peerID, peerObj); updateErr != nil {
				return errfmt.Newf("failed to persist federated node").Wrap(err)
			}
		}

		_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("✅ Federated session established with: %s\nRemote ID: %s\nCapabilities: %d shared\n",
			remoteEndpoint, resp.KernelID, len(resp.Capabilities))))

		return nil
	})(cmd, args)
}

// NewHandshakeCmd creates the system federate handshake command
func NewHandshakeCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSystemFederateHandshakeCommandBuilder()
	cmd.RunE = runHandshake
	return cmd
}

func runHandshake(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		var err error
		_ = err
		ctx := proc.OperationContext()

		var req federation.HandshakeRequest
		if err := json.Unmarshal([]byte(args[0]), &req); err != nil {
			return errfmt.Newf("failed to unmarshal handshake request").Wrap(err)
		}

		// 1. Resolve local identity for response
		idManager := federation.NewIdentityManager(proc.ProjectRoot())
		localKernelID, _ := idManager.GetKernelID()
		localPublicKey, _ := idManager.GetPublicKey()

		// 2. Validate the request (In a real implementation, verify signatures, etc.)
		// For POC, we'll accept if protocol version matches
		if req.ProtocolVersion != "1.0" && req.ProtocolVersion != "1.0.0" {
			return cli.FormatOutput(cmd, federation.HandshakeResponse{
				Accepted: false,
				Message:  fmt.Sprintf("unsupported protocol version: %s", req.ProtocolVersion),
			})
		}

		// 3. Discover local capabilities to share
		sp := proc.Storage()
		secCtx := proc.SecurityContext()

		skillFilter := storage.ListFilter{Kind: objects.KindAgentSkill, Limit: 0}
		skills, err := sp.List(ctx, secCtx, proc.StorageContext(), skillFilter)
		if err != nil {
			proc.Logger().LogWarning("Failed to list agent skills for capabilities", logging.Error(err))
		}

		var caps []federation.Capability
		if skills != nil {
			for _, skill := range skills.Objects {
				id, ok := skill[objects.FieldKeyID].(string)
				if !ok {
					// Log warning and skip if ID is not a string
					proc.Logger().LogWarning("Skipping skill with non-string ID", logging.String("skill", fmt.Sprintf("%v", skill)))
					continue
				}
				title, ok := skill[objects.FieldKeyTitle].(string)
				if !ok {
					// Log warning and skip if Title is not a string
					proc.Logger().LogWarning("Skipping skill with non-string Title", logging.String("skill", fmt.Sprintf("%v", skill)), logging.String("id", id))
					continue
				}
				caps = append(caps, federation.Capability{
					Kind: "agent_skill",
					ID:   id,
					Name: title,
				})
			}
		}

		// 4. Persist the remote kernel object
		remoteState := &federation.RemoteKernelState{
			ID:            req.KernelID,
			PublicKey:     req.PublicKey,
			Capabilities:  req.Capabilities,
			LastHandshake: time.Now(),
		}

		// Create or update remote_kernel object
		obj := remoteState.ToMap()
		obj[objects.FieldKeyStatus] = "implemented"
		syncCtx := storage.WithSyncCreateForKind(ctx, objects.KindRemoteKernel)
		err = sp.Create(syncCtx, secCtx, obj)
		if err != nil {
			peerID := obj[objects.FieldKeyID].(string)
			if updateErr := sp.Update(syncCtx, secCtx, peerID, obj); updateErr != nil {
				return errfmt.Newf("failed to persist federated node").Wrap(err)
			}
		}

		// 5. Return response
		resp := federation.HandshakeResponse{
			Accepted:     true,
			KernelID:     localKernelID,
			PublicKey:    localPublicKey,
			Capabilities: caps,
		}

		return cli.FormatOutput(cmd, resp)
	})(cmd, args)
}
