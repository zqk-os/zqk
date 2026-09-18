package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/brand"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"gopkg.in/yaml.v3"
)

// applybrand rewrites dest first-run copy from canonical `zqk` tokens to
// brand.executable_name. SKU overlay sources stay canonical.
// TRACK: TDE-1789678536875854000-47240146
func main() {
	root := flag.String("root", ".", "project root whose brand.executable_name is applied")
	flag.Parse()
	if err := run(*root); err != nil {
		fmt.Fprintf(os.Stderr, "applybrand: %v\n", err)
		os.Exit(1)
	}
}

type brandYAML struct {
	Brand struct {
		ExecutableName string `yaml:"executable_name"`
		ProductName    string `yaml:"product_name"`
	} `yaml:"brand"`
	ExecutableName string `yaml:"executable_name"`
}

func run(root string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	exe := loadExecutable(root)
	if exe == "" || exe == brand.CanonicalExecutableToken {
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

func loadExecutable(root string) string {
	candidates := []string{
		filepath.Join(root, paths.ProjectDataDir, paths.ConfigDir, paths.ProjectConfigFile),
		filepath.Join(root, paths.ProjectDataDir, paths.ProjectConfigFile),
	}
	for _, p := range candidates {
		data, err := fileutil.ReadFile(p)
		if err != nil {
			continue
		}
		var cfg brandYAML
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			continue
		}
		if v := strings.TrimSpace(cfg.Brand.ExecutableName); v != "" {
			return v
		}
		if v := strings.TrimSpace(cfg.ExecutableName); v != "" {
			return v
		}
	}
	return brand.CanonicalExecutableToken
}

func productDocPaths(root string) []string {
	var out []string
	for _, p := range []string{
		filepath.Join(root, "README.md"),
		filepath.Join(root, "CONTRIBUTING.md"),
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
