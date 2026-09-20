package newcmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// writeDraft writes content to --output path, or default under .zqk/drafts/, or stdout when explicitly "-".
// draftScope and draftKind identify the materialized draft for .zqk/drafts/last-draft.yaml (omit pointer when either is empty).
func writeDraft(cmd *cobra.Command, outFlag, defaultBaseName string, content []byte, draftScope, draftKind string) error {
	out := strings.TrimSpace(outFlag)
	if out == emptyValue {
		root := cli.ResolveProjectRoot(".")
		if root == emptyValue {
			root = "."
		}
		dir := filepath.Join(root, paths.ProjectDataDir, "drafts")
		if err := fileutil.EnsureDir(dir); err != nil {
			return errfmt.Newf("create drafts dir").Wrap(err)
		}
		ts := zqktime.NowLayoutUTC(zqktime.LayoutLogRotateStamp)
		safe := strings.Map(func(r rune) rune {
			if r == '/' || r == '\\' || r == ':' {
				return '-'
			}
			return r
		}, defaultBaseName)
		out = filepath.Join(dir, fmt.Sprintf("%s-%s.yaml", safe, ts))
	}
	if out == "-" {
		return cli.WriteOutput(cmd, content)
	}
	if err := fileutil.WriteSecureFile(out, content); err != nil {
		return errfmt.Newf("write %s", out).Wrap(err)
	}
	root := cli.ResolveProjectRoot(".")
	if root != emptyValue && draftScope != emptyValue && draftKind != emptyValue {
		absOut, err := filepath.Abs(out)
		if err == nil {
			_ = cli.WriteLastDraftPointer(root, draftScope, draftKind, absOut)
		}
	}
	log := logging.GetLoggerFromProfile(string(pkgctx.ProfileHuman))
	logging.Fluent(log).Info("Wrote draft template").Path(out).Bytes(len(content)).Log()
	return nil
}
