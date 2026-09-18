package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/brand"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// applybrand rewrites dest first-run copy from canonical `zqk` tokens to
// brand.executable_name in config/zqk-local.yaml then config/zqk.yaml.
// SKU overlay sources stay canonical.
// TRACK: TDE-1789678536875854000-47240146
func main() {
	root := flag.String("root", ".", "project root whose brand.executable_name is applied")
	flag.Parse()
	if err := run(*root); err != nil {
		fmt.Fprintf(os.Stderr, "applybrand: %v\n", err)
		os.Exit(1)
	}
}

func run(root string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	exe := brand.LoadFromProject(root).ExecutableName
	if exe == "" {
		exe = brand.CanonicalExecutableToken
	}
	if exe == brand.CanonicalExecutableToken {
		fmt.Printf("applybrand: executable=%s (canonical; no rewrite)\n", brand.CanonicalExecutableToken)
		return nil
	}
	var n int
	for _, p := range productDocPaths(root) {
		data, err := fileutil.ReadFile(p)
		if err != nil {
			continue
		}
		next := brand.ApplyCanonicalExecutable(string(data), exe)
		if next == string(data) {
			continue
		}
		if err := fileutil.WriteStandardFile(p, []byte(next)); err != nil {
			return err
		}
		n++
		fmt.Printf("applybrand: %s\n", p)
	}
	fmt.Printf("applybrand: executable=%s files=%d\n", exe, n)
	return nil
}

func productDocPaths(root string) []string {
	var out []string
	for _, p := range []string{
		filepath.Join(root, "README.md"),
		filepath.Join(root, "CONTRIBUTING.md"),
		filepath.Join(root, "ZQK_GETTING_STARTED.md"),
		filepath.Join(root, "docs", "getting-started.md"),
		filepath.Join(root, ".iderules"),
		filepath.Join(root, ".clinerules"),
		filepath.Join(root, ".windsurfrules"),
	} {
		out = append(out, p)
	}
	onb := filepath.Join(root, "docs", "onboarding")
	if entries, err := os.ReadDir(onb); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			out = append(out, filepath.Join(onb, e.Name()))
		}
	}
	return out
}
