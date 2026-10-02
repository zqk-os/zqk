package system

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/strutil"
)

// getSystemCliContext returns a CLI context initialized for system commands.
func getSystemCliContext(cmd *cobra.Command) (*cli.Context, error) {
	initCtx := &pkgctx.CliInitializationContext{
		ProjectRoot: ProjectRootOrResolve(""),
	}
	return cli.GetContextFromCommand(cmd, initCtx)
}

func profileOrDefault(profile, fallback string) string {
	return strutil.OrDefault(profile, fallback)
}

func ProjectRootOrResolve(projectRoot string) string {
	return strutil.OrDefault(projectRoot, cli.ResolveProjectRoot("."))
}

// ProjectRootOrResolveDot resolves when projectRoot is empty or the "." sentinel (unresolved cwd).
// Non-empty paths other than "." are returned unchanged.
func ProjectRootOrResolveDot(projectRoot string) string {
	if projectRoot == emptyValue || projectRoot == "." {
		return cli.ResolveProjectRoot(".")
	}
	return projectRoot
}

func resolveCommandProjectRoot(cmd *cobra.Command) (string, error) {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return "", errfmt.Errorf("failed to get context")
	}
	projectRoot := ProjectRootOrResolve(ctx.ProjectRoot)
	if projectRoot == emptyValue {
		return "", errfmt.Errorf("project root not found")
	}
	return projectRoot, nil
}

// openStorageProvider creates a StorageFactory for projectRoot and returns its ObjectStorageProvider along with a deferrable cleanup function.
func openStorageProvider(ctx context.Context, projectRoot string) (storage.ObjectStorageProvider, func(), error) {
	factory, err := storage.NewStorageFactory(ctx, projectRoot)
	if err != nil {
		return nil, nil, errfmt.Newf("storage factory").Wrap(err)
	}
	storageProvider := factory.GetStorage()
	cleanup := func() {
		if storageProvider != nil {
			_ = storageProvider.Shutdown(context.Background())
		}
	}
	return storageProvider, cleanup, nil
}
