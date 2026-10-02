package mesh

import (
	"crypto/rand"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// NewLeaseCmd creates the mesh lease command
func NewLeaseCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewMeshLeaseCommandBuilder()
	cmd.RunE = runLease
	return cmd
}

func runLease(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		resourceID := args[0]
		providerRef, _ := cmd.Flags().GetString("from")
		durationStr, _ := cmd.Flags().GetString("duration")
		duration, _ := time.ParseDuration(durationStr)

		// Resolve local identity
		kernelID, err := resolveLocalKernelID(proc.ProjectRoot())
		if err != nil {
			return err
		}

		// 1. Authenticate with provider (In a real implementation, this is an MCP call)
		token := generateToken()

		// 2. Create lease agreement
		leaseObj := map[string]any{
			objects.FieldKeyKind:              "skill_lease",
			objects.FieldKeyTitle:             fmt.Sprintf("Lease: %s from %s", resourceID, providerRef),
			objects.FieldKeyProviderKernelRef: providerRef,
			objects.FieldKeyConsumerKernelRef: kernelID,
			objects.FieldKeyResourceRef:       resourceID,
			objects.FieldKeyTokenID:           token,
			objects.FieldKeyStatus:            objects.ObjectStatusImplemented,
		}

		if duration > 0 {
			leaseObj[objects.FieldKeyExpiresAt] = time.Now().Add(duration).Format(time.RFC3339)
		}

		// Use synchronous creation for immediate visibility in the dashboard
		syncCtx := storage.WithSyncCreateForKind(proc.OperationContext(), "skill_lease")
		if err := proc.Storage().Create(syncCtx, proc.SecurityContext(), leaseObj); err != nil {
			return errfmt.Newf("failed to create lease").Wrap(err)
		}

		_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("✅ Lease established for %s.\nAuthorization Token: %s\n", resourceID, token)))
		return nil
	})(cmd, args)
}

func generateToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("TOK-%X", b)
}
