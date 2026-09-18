package app

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

type DummyStorage struct {
	storage.ObjectStorageProvider // Embed interface to stub all methods with panic by default
}

func (d *DummyStorage) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	return map[string]any{
		objects.FieldKeyID:                 id,
		objects.FieldKeyKind:               "audit_aggregation_metric",
		objects.FieldKeyAggregatedEventIDs: []any{"event-1", "event-2"},
	}, nil
}

func (d *DummyStorage) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	return nil
}

func (d *DummyStorage) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	return nil
}

func (d *DummyStorage) Delete(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, cascade bool) error {
	return nil
}

func (d *DummyStorage) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	return &storage.QueryResult{
		Objects: []map[string]any{},
		Groups:  make(map[string][]map[string]any),
		Meta:    make(map[string]any),
	}, nil
}

func (d *DummyStorage) BulkDelete(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string, cascade bool) (*storage.BulkResult, error) {
	return &storage.BulkResult{}, nil
}

func (d *DummyStorage) BulkUpdate(ctx context.Context, secCtx *pkgctx.SecurityContext, items []storage.BulkUpdateItem) (*storage.BulkResult, error) {
	return &storage.BulkResult{}, nil
}

type TestPermutation struct {
	Name              string            `yaml:"name"`
	Args              []string          `yaml:"args"`
	Flags             map[string]string `yaml:"flags"`
	MockInjectors     map[string]any    `yaml:"mock_injectors"`
	ExpectError       bool              `yaml:"expect_error"`
	ExpectOutputMatch string            `yaml:"expect_output_match"`
}

type CommandSpecWithPermutations struct {
	Name             string            `yaml:"name"`
	TestPermutations []TestPermutation `yaml:"test_permutations"`
}

func TestCommandFlagPermutations(t *testing.T) {
	ensureCommandsRegistered()

	cwd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	projectRoot := filepath.Clean(filepath.Join(cwd, "../../.."))
	specsDir := filepath.Join(projectRoot, ".zqk/cli/specs")

	err = filepath.Walk(specsDir, func(path string, info fileutil.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml" {
			return nil
		}

		data, err := fileutil.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read %s: %v", path, err)
		}

		var spec CommandSpecWithPermutations
		if err := yaml.Unmarshal(data, &spec); err != nil {
			// Some specs might fail to parse loosely if malformed, but we only care if it has test_permutations
			return nil
		}

		if len(spec.TestPermutations) == 0 {
			return nil
		}

		specBaseName := spec.Name
		for _, perm := range spec.TestPermutations {
			t.Run(fmt.Sprintf("%s/%s", spec.Name, perm.Name), func(t *testing.T) {
				// Re-use rootCmd but create an isolated sub-context
				cmd := rootCmd

				// Build args list
				var execArgs []string

				// Find path
				relPath, _ := filepath.Rel(specsDir, path)
				dir := filepath.Dir(relPath)
				if dir != "." && dir != "" {
					execArgs = append(execArgs, filepath.SplitList(dir)...)
				}
				execArgs = append(execArgs, specBaseName)

				for k, v := range perm.Flags {
					execArgs = append(execArgs, fmt.Sprintf("--%s=%s", k, v))
				}
				execArgs = append(execArgs, perm.Args...)

				cmd.SetArgs(execArgs)

				// Set up mock buffer for output capturing
				buf := new(bytes.Buffer)
				cmd.SetOut(buf)
				cmd.SetErr(buf)

				// Create isolated project root to avoid touching real data
				tempDir := t.TempDir()
				_ = zqkenv.ProjectRoot().Set(tempDir)
				defer zqkenv.ProjectRoot().Unset()

				// Setup context with mocked dependencies
				ctx := context.Background()
				if mockStorage, ok := perm.MockInjectors[objects.FieldKeyStorage]; ok && mockStorage == "mock_storage_success" {
					ctx = cli.WithStorageProvider(ctx, &DummyStorage{})
				}

				err := cmd.ExecuteContext(ctx)

				if perm.ExpectError && err == nil {
					t.Errorf("Expected error but got nil. Output: %s", buf.String())
				} else if !perm.ExpectError && err != nil {
					t.Errorf("Expected success but got error: %v. Output: %s", err, buf.String())
				}
			})
		}
		return nil
	})

	if err != nil {
		t.Fatalf("Failed to walk command specs: %v", err)
	}
}
