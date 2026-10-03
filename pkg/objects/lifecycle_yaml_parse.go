package objects

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// smash (bool flag + leftover bullet on one
// line, unquoted ':' in a postcondition) used to surface only in loadAllLifecycleDocs.
// EnsureReady now walks the dir so CLI init / system check Warns before a later LoadLifecycle.

// LifecycleYAMLIssue is one file that failed typed parse or the leftover-bullet heuristic.
type LifecycleYAMLIssue struct {
	Path string
	Err  error
}

// smashedBoolFlagLine matches `archive: true      - leftover bullet` (valid-ish YAML
// that silently poisons bool flags). Display-line leftovers are still typed-valid.
var smashedBoolFlagLine = regexp.MustCompile(`(?m)^\s+(?:origin|terminal|preliminary|archive|work_done|satisfied|system|manual|auto):\s+(?:true|false)[ \t]+\S`)

func walkYAMLDir(dir string, inspect func(path string, data []byte) error) ([]LifecycleYAMLIssue, error) {
	var issues []LifecycleYAMLIssue
	walkErr := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d == nil || d.IsDir() || filepath.Ext(d.Name()) != ".yaml" {
			return nil
		}
		data, err := fileutil.ReadFile(path)
		if err != nil {
			issues = append(issues, LifecycleYAMLIssue{Path: path, Err: err})
			return nil
		}
		if inspectErr := inspect(path, data); inspectErr != nil {
			issues = append(issues, LifecycleYAMLIssue{Path: path, Err: inspectErr})
		}
		return nil
	})
	if walkErr != nil {
		return nil, errfmt.Errorf("read yaml dir %s: %w", dir, walkErr)
	}
	return issues, nil
}

// ParseLifecycleYAMLDir unmarshals every *.yaml in dir (including subdirectories) into Lifecycle and scans for
// smashed bool-flag lines. Parse failures do not stop the walk; callers Warn then decide
// whether to fail closed (tests / pre-commit) or continue (CLI init).
func ParseLifecycleYAMLDir(dir string) ([]LifecycleYAMLIssue, error) {
	return walkYAMLDir(dir, func(path string, data []byte) error {
		if loc := smashedBoolFlagLine.FindIndex(data); loc != nil {
			line := strings.TrimSpace(string(data[loc[0]:loc[1]]))
			return errfmt.Errorf("smashed bool flag (leftover bullet on same line): %s", line)
		}
		var lifecycle Lifecycle
		return yaml.Unmarshal(data, &lifecycle)
	})
}

// ParseConfigYAMLDir syntax-checks every *.yaml in dir (including subdirectories). Typed
// per-schema validation stays with each config loader; this is the early smash/syntax net.
func ParseConfigYAMLDir(dir string) ([]LifecycleYAMLIssue, error) {
	return walkYAMLDir(dir, func(path string, data []byte) error {
		var raw map[string]any
		return yaml.Unmarshal(data, &raw)
	})
}

// FormatLifecycleYAMLIssues joins file-level parse errors for EnsureReady / tests.
func FormatLifecycleYAMLIssues(issues []LifecycleYAMLIssue) error {
	if len(issues) == 0 {
		return nil
	}
	errs := make([]error, 0, len(issues))
	for _, iss := range issues {
		errs = append(errs, errfmt.Errorf("%s: %w", iss.Path, iss.Err))
	}
	return errfmt.Errorf("startup YAML parse: %d file(s) invalid: %w", len(issues), errors.Join(errs...))
}

func warnLifecycleYAMLIssues(kind string, issues []LifecycleYAMLIssue) {
	if len(issues) == 0 {
		return
	}
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	for _, iss := range issues {
		logging.Fluent(logger).Warn("Startup YAML failed typed parse").
			Path(iss.Path).
			String("yaml_kind", kind).
			WithError(iss.Err).
			Log()
	}
	logging.Fluent(logger).Warn("Startup YAML parse found invalid files; CLI init continues").
		String("yaml_kind", kind).
		Count(len(issues)).
		Log()
}
