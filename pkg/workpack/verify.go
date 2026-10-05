package workpack

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

var (
	verifiedSpecs map[string]string
	verifyErr     error
)

// kindsWithoutLifecycle are work-graph mixins. They are not instantiable and have no lifecycle file.
var kindsWithoutLifecycle = map[string]struct{}{
	objects.KindWorkInterval:  {},
	objects.KindWorkUnit:      {},
	objects.KindOccupancy:     {},
	objects.KindRemainingOpen: {},
}

// VerifiedSpec returns the spec path recorded when this pack was verified.
func VerifiedSpec(kind string) (string, bool) {
	if verifiedSpecs == nil {
		return "", false
	}
	path, ok := verifiedSpecs[kind]
	return path, ok
}

// VerifyError is the last verification failure. It is nil after a successful Enable.
func VerifyError() error { return verifyErr }

func verifyOwnedKinds() (map[string]string, error) {
	root, err := moduleRoot()
	if err != nil {
		return nil, err
	}
	return verifyKindFiles(root, Kinds(), SpecDir, LifecycleDir)
}

func verifyKindFiles(root string, kinds []string, specRel, lifecycleRel string) (map[string]string, error) {
	recorded := make(map[string]string, len(kinds))
	for _, kind := range kinds {
		specPath := filepath.Join(root, specRel, kind+".yaml")
		if _, err := os.Stat(specPath); err != nil {
			return nil, fmt.Errorf("work pack kind %s spec: %w", kind, err)
		}
		if _, skip := kindsWithoutLifecycle[kind]; !skip {
			lifecyclePath := filepath.Join(root, lifecycleRel, kind+"_lifecycle.yaml")
			if _, err := os.Stat(lifecyclePath); err != nil {
				return nil, fmt.Errorf("work pack kind %s lifecycle: %w", kind, err)
			}
		}
		recorded[kind] = specPath
	}
	return recorded, nil
}

func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil || dir == "" {
		return "", fmt.Errorf("work pack module root: %w", err)
	}
	for {
		data, readErr := fileutil.ReadFile(filepath.Join(dir, "go.mod"))
		if readErr == nil {
			line, _, _ := strings.Cut(string(data), "\n")
			if strings.TrimSpace(line) == "module github.com/zqk-os/zqk" {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("work pack module root not found")
		}
		dir = parent
	}
}
