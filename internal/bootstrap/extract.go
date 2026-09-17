package bootstrap

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/appledouble"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

const (
	embeddedBootstrapArchivePath                   = "archive/bootstrap.tar.gz"
	archivePrefixCLISpecs                          = "cli_specs/"
	archivePrefixScripts                           = "scripts/"
	archivePrefixCleanupConfig                     = "cleanup_config/"
	internalSubdir                                 = paths.ProcessInternalDir
	scriptsSubdir                                  = "scripts"
	cleanupConfigSubdir                            = ".zqk/cleanup"
	bootstrapDirPerm             fileutil.FileMode = 0o755
	bootstrapFilePerm            fileutil.FileMode = 0o600
	// maxFileSize limits extracted file size to avoid decompression-bomb DoS (gosec G110)
	maxFileSize = 50 << 20 // 50 MiB per file
)

// ExtractTo extracts the embedded bootstrap archive into the project.
// projectRoot is the repo root. Archive entries are mapped as follows:
//   - entries under "cli_specs/" -> projectRoot/.zqk/cli/specs/
//   - entries under "scripts/" -> projectRoot/scripts/ (e.g. scheduler_jobs maintenance templates)
//   - entries under "cleanup_config/" -> projectRoot/.zqk/cleanup/ (cleanup config for SCH-cleanup job)
//   - all other entries -> projectRoot/.zqk/specs/
//
// Preserves directory structure. When force is true, overwrites existing files.
// REQ-9010: Init command extracts bootstrap archive; preserve structure; respect --force; clear errors.
func ExtractTo(projectRoot string, logger logging.Logger, force bool) error {
	data, err := archiveFS.ReadFile(embeddedBootstrapArchivePath)
	if err != nil {
		return errfmt.Newf("failed to read embedded bootstrap archive").Wrap(err)
	}
	if len(data) == 0 {
		return errfmt.Errorf("bootstrap archive is empty")
	}

	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return errfmt.Newf("failed to open gzip stream").Wrap(err)
	}
	defer zr.Close()

	internalDir := filepath.Join(projectRoot, internalSubdir)
	cliSpecsDir := filepath.Join(projectRoot, paths.CLICommandSpecsDir)
	scriptsDir := filepath.Join(projectRoot, scriptsSubdir)
	cleanupConfigDir := filepath.Join(projectRoot, cleanupConfigSubdir)

	tr := tar.NewReader(zr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errfmt.Newf("failed to read archive").Wrap(err)
		}

		name := filepath.Clean(hdr.Name)
		// Skip macOS AppleDouble / resource-fork sidecar files (._*) that may be bundled if the
		// archive was built from a tree that contained them; they are not valid YAML/spec payloads.
		if appledouble.SkipPathInTreeWalk(name) {
			if hdr.Typeflag == tar.TypeReg && hdr.Size > 0 {
				if _, err := io.CopyN(io.Discard, tr, hdr.Size); err != nil {
					return errfmt.Errorf("failed to skip archive entry %s: %w", hdr.Name, err)
				}
			}
			continue
		}
		if after, ok := strings.CutPrefix(name, "./"); ok {
			name = after
		} else if after, ok := strings.CutPrefix(name, ".\\"); ok {
			name = after
		}
		// Community edition: omit internal/dev-only command specs ([REDACTED-ID]).
		if zqkenv.IsCommunityEdition && strings.HasPrefix(name, archivePrefixCLISpecs) && shouldExcludeCommunityCommandSpec(name) {
			if hdr.Typeflag == tar.TypeReg && hdr.Size > 0 {
				if _, err := io.CopyN(io.Discard, tr, hdr.Size); err != nil {
					return errfmt.Errorf("failed to skip community-denied archive entry %s: %w", hdr.Name, err)
				}
			}
			continue
		}
		var targetPath string
		switch {
		case strings.HasPrefix(name, archivePrefixCLISpecs):
			if rest, ok := strings.CutPrefix(name, archivePrefixCLISpecs); ok {
				targetPath = filepath.Join(cliSpecsDir, rest)
			} else {
				targetPath = filepath.Join(internalDir, name)
			}
		case strings.HasPrefix(name, archivePrefixScripts):
			if rest, ok := strings.CutPrefix(name, archivePrefixScripts); ok {
				targetPath = filepath.Join(scriptsDir, rest)
			} else {
				targetPath = filepath.Join(internalDir, name)
			}
		case strings.HasPrefix(name, archivePrefixCleanupConfig):
			if rest, ok := strings.CutPrefix(name, archivePrefixCleanupConfig); ok {
				targetPath = filepath.Join(cleanupConfigDir, rest)
			} else {
				targetPath = filepath.Join(internalDir, name)
			}
		default:
			targetPath = filepath.Join(internalDir, name)
		}
		baseDir := filepath.Dir(targetPath)
		if !withinBase(internalDir, targetPath) && !withinBase(cliSpecsDir, targetPath) && !withinBase(scriptsDir, targetPath) && !withinBase(cleanupConfigDir, targetPath) {
			return errfmt.Errorf("archive entry path escapes target: %s", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := fileutil.MkdirAll(targetPath, bootstrapDirPerm); err != nil {
				return errfmt.Errorf("failed to create directory %s: %w", hdr.Name, err)
			}
		case tar.TypeReg:
			if err := fileutil.MkdirAll(baseDir, bootstrapDirPerm); err != nil {
				return errfmt.Errorf("failed to create directory for %s: %w", hdr.Name, err)
			}
			if _, err := fileutil.Stat(targetPath); err == nil && !force {
				continue
			}
			size := hdr.Size
			if size < 0 || size > maxFileSize {
				return errfmt.Errorf("archive entry %s has invalid or oversized length %d", hdr.Name, size)
			}
			flags := fileutil.O_WRONLY | fileutil.O_CREATE | fileutil.O_TRUNC
			f, err := fileutil.OpenFile(targetPath, flags, bootstrapFilePerm) //nolint:gosec // Bootstrap files - 0600 acceptable
			if err != nil {
				return errfmt.Errorf("failed to create file %s: %w", hdr.Name, err)
			}
			if _, err := io.CopyN(f, tr, size); err != nil {
				_ = f.Close()
				return errfmt.Errorf("failed to write %s: %w", hdr.Name, err)
			}
			if err := f.Close(); err != nil {
				return errfmt.Errorf("failed to close %s: %w", hdr.Name, err)
			}
			if logger != nil {
				logging.Fluent(logger).Debug("Extracted bootstrap file").
					File(hdr.Name).
					Path(targetPath).
					Log()
			}
		}
	}

	if logger != nil {
		logging.Fluent(logger).Info("Bootstrap archive extracted successfully").
			ProjectRoot(projectRoot).
			Log()
	}
	return nil
}

func withinBase(base, path string) bool {
	absBase, _ := filepath.Abs(base)
	absPath, _ := filepath.Abs(path)
	rel, err := filepath.Rel(absBase, absPath)
	if err != nil {
		return false
	}
	return rel == "." || !strings.HasPrefix(rel, "..")
}
