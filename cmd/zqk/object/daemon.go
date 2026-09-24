package object

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/accumulator"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

const (
	// daemonEnvEnabled is the truthy value for the brand-prefixed IS_DAEMON env flag.
	daemonEnvEnabled = "1"
	// daemonSocketPerm keeps the writer socket owner-only (it accepts privileged CAS writes).
	daemonSocketPerm = paths.FilePerm600
)

type daemonAdapter struct {
	fileStorage *storage.FileObjectStorage
}

func (a *daemonAdapter) WriteObject(ctx context.Context, id, kind string, payload []byte, isDraft bool) error {
	if isDraft {
		return a.fileStorage.WriteObjectToDraftPlane(id, kind, payload)
	}
	return a.fileStorage.WriteObjectRaw(ctx, kind, id, payload)
}

func (a *daemonAdapter) DeleteObject(ctx context.Context, id, kind string) error {
	return a.fileStorage.DeleteObjectRaw(ctx, kind, id)
}

func (a *daemonAdapter) RenameObject(ctx context.Context, oldID, newID, kind string) error {
	return a.fileStorage.RenameObjectRaw(ctx, kind, oldID, newID)
}

// NewDaemonCmd creates the daemon subcommand.
func NewDaemonCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "daemon",
		Short:  "Start the privileged writer daemon",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Must be set before Processor/storage init: NewFileObjectStorage and the
			// CAS membrane key off DaemonProcess() (skip WAL replay, write locally).
			// Setting it after WithProcessor left the daemon as a second write-behind
			// owner that then self-dialed until EMFILE.
			if err := zqkenv.IsDaemon().Set(daemonEnvEnabled); err != nil {
				return errfmt.Errorf("failed to mark process as privileged writer daemon: %w", err)
			}
			// Privileged writer daemon must run under the system service account
			// so it does not inherit ambient unprivileged sessions from the workspace.
			if os.Getenv(zqkenv.APIKey().Name()) == "" {
				_ = zqkenv.APIKey().Set(pkgctx.SystemAccountID)
			}
			return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
				logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
				fileStorage := storage.UnwrapToFileObjectStorage(proc.Storage())
				if fileStorage == nil {
					return errfmt.Errorf("could not resolve underlying file storage")
				}
				if err := fileStorage.RefuseWriteBehindIfPrivilegedWriterRole(); err != nil {
					return err
				}

				projectRoot := proc.ProjectRoot()
				adapter := &daemonAdapter{fileStorage: fileStorage}
				daemon := storage.NewPrivilegedWriterDaemon(adapter, projectRoot)

				socketPath := storage.ProjectScopedPrivilegedWriterSocketPath(projectRoot)
				if err := fileutil.MkdirAll(filepath.Dir(socketPath), paths.DirPerm755); err != nil {
					return errfmt.Errorf("failed to create socket directory: %w", err)
				}
				// Clean up old socket if it exists
				_ = fileutil.Remove(socketPath)

				listener, err := storage.StartIPCServer(socketPath, daemon)
				if err != nil {
					return errfmt.Errorf("failed to start daemon: %w", err)
				}
				defer listener.Close()

				_ = fileutil.Chmod(socketPath, daemonSocketPerm)

				// Structured log start (POL-CODE-007)
				logging.Fluent(logger).Info("Privileged writer daemon listening").Path(socketPath).Log()

				// Start daemon-hosted reactive accumulator WAL subscribers (Section 8 of REACTIVE_MATERIALIZED_VIEW_ACCUMULATOR_PATTERN.md).
				daemonCtx, daemonCancel := context.WithCancel(cmd.Context())
				if cmd.Context() == nil {
					daemonCtx, daemonCancel = context.WithCancel(pkgctx.NewSystemContext())
				}
				defer daemonCancel()
				if projectRoot := proc.ProjectRoot(); projectRoot != "" {
					accumulator.StartAllSubscribers(daemonCtx, projectRoot)
				}

				sigChan := make(chan os.Signal, 1)
				signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
				<-sigChan

				logging.Fluent(logger).Info("Shutting down privileged writer daemon").Log()
				return nil
			})(cmd, args)
		},
	}
	return cmd
}
