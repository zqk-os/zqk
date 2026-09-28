// Package packrecord verifies an uploaded spec pack and records its specs
// so those kinds load as typed objects. Spec-only packs need no rebuild.
package packrecord

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
)

const (
	manifestName    = "kindpack.yaml"
	specSubdir      = "specs"
	lifecycleSubdir = "lifecycles"
	storeDirName    = "installed-packs"
	dirPerm         = 0o755
	filePerm        = 0o644
)

// Manifest is the pack's declaration of the kinds it owns.
type Manifest struct {
	Name  string   `yaml:"name"`
	Kinds []string `yaml:"kinds"`
}

// Install verifies src and copies the formal specs into the project store.
// The same process can load the new kinds immediately.
func Install(src, projectRoot string) (Manifest, error) {
	manifest, err := verify(src)
	if err != nil {
		return Manifest{}, err
	}
	if projectRoot == "" {
		return Manifest{}, fmt.Errorf("pack record: project root is empty")
	}
	dest := recordDir(projectRoot, manifest.Name)
	if _, err := os.Stat(dest); err == nil {
		return Manifest{}, fmt.Errorf("pack record: %s is already recorded", manifest.Name)
	}
	if err := copyVerified(src, dest, manifest); err != nil {
		_ = os.RemoveAll(dest)
		return Manifest{}, err
	}
	if err := register(dest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// Load registers every recorded pack under the project root.
func Load(projectRoot string) error {
	if projectRoot == "" {
		return nil
	}
	store := filepath.Join(projectRoot, paths.ProjectDataDir, storeDirName)
	buckets, err := os.ReadDir(store)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("pack record: %w", err)
	}
	for _, bucket := range buckets {
		if !bucket.IsDir() {
			continue
		}
		packs, err := os.ReadDir(filepath.Join(store, bucket.Name()))
		if err != nil {
			return fmt.Errorf("pack record: %w", err)
		}
		for _, pack := range packs {
			if !pack.IsDir() {
				continue
			}
			dir := filepath.Join(store, bucket.Name(), pack.Name())
			if _, err := os.Stat(filepath.Join(dir, manifestName)); err != nil {
				continue
			}
			if err := register(dir); err != nil {
				return err
			}
		}
	}
	return nil
}

func verify(src string) (Manifest, error) {
	data, err := os.ReadFile(filepath.Join(src, manifestName))
	if err != nil {
		return Manifest{}, fmt.Errorf("pack record: %w", err)
	}
	var manifest Manifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("pack record: manifest: %w", err)
	}
	if err := validName(manifest.Name); err != nil {
		return Manifest{}, err
	}
	if len(manifest.Kinds) == 0 {
		return Manifest{}, fmt.Errorf("pack record: %s declares no kinds", manifest.Name)
	}
	seen := make(map[string]struct{}, len(manifest.Kinds))
	for _, kind := range manifest.Kinds {
		if err := validName(kind); err != nil {
			return Manifest{}, fmt.Errorf("pack record: kind %q: %w", kind, err)
		}
		if _, ok := seen[kind]; ok {
			return Manifest{}, fmt.Errorf("pack record: duplicate kind %s", kind)
		}
		seen[kind] = struct{}{}
		specPath := filepath.Join(src, specSubdir, kind+".yaml")
		specData, err := os.ReadFile(specPath)
		if err != nil {
			return Manifest{}, fmt.Errorf("pack record: kind %s spec: %w", kind, err)
		}
		var spec struct {
			Ontology string `yaml:"ontology"`
		}
		if err := yaml.Unmarshal(specData, &spec); err != nil {
			return Manifest{}, fmt.Errorf("pack record: kind %s spec: %w", kind, err)
		}
		if spec.Ontology != kind {
			return Manifest{}, fmt.Errorf("pack record: kind %s spec ontology is %q", kind, spec.Ontology)
		}
		lifePath := filepath.Join(src, lifecycleSubdir, kind+"_lifecycle.yaml")
		lifeData, err := os.ReadFile(lifePath)
		if err != nil {
			return Manifest{}, fmt.Errorf("pack record: kind %s lifecycle: %w", kind, err)
		}
		var life map[string]any
		if err := yaml.Unmarshal(lifeData, &life); err != nil || len(life) == 0 {
			return Manifest{}, fmt.Errorf("pack record: kind %s lifecycle is not a spec", kind)
		}
	}
	return manifest, nil
}

func copyVerified(src, dest string, manifest Manifest) error {
	if err := os.MkdirAll(filepath.Join(dest, specSubdir), dirPerm); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dest, lifecycleSubdir), dirPerm); err != nil {
		return err
	}
	encoded, err := yaml.Marshal(manifest)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dest, manifestName), encoded, filePerm); err != nil {
		return err
	}
	for _, kind := range manifest.Kinds {
		specName := kind + ".yaml"
		if err := copyFile(filepath.Join(src, specSubdir, specName), filepath.Join(dest, specSubdir, specName)); err != nil {
			return err
		}
		lifeName := kind + "_lifecycle.yaml"
		if err := copyFile(filepath.Join(src, lifecycleSubdir, lifeName), filepath.Join(dest, lifecycleSubdir, lifeName)); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, filePerm)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func register(dir string) error {
	objects.AddSpecRoot(filepath.Join(dir, specSubdir))
	objects.AddLifecycleRoot(filepath.Join(dir, lifecycleSubdir))
	return nil
}

func recordDir(projectRoot, name string) string {
	return filepath.Join(projectRoot, paths.ProjectDataDir, storeDirName, bucket(name), name)
}

func bucket(name string) string {
	return name[:1]
}

func validName(name string) error {
	if name == "" || len(name) > 63 {
		return fmt.Errorf("pack record: name %q is empty or too long", name)
	}
	for i, r := range name {
		if r == '-' {
			if i == 0 {
				return fmt.Errorf("pack record: name %q must start with a letter", name)
			}
			continue
		}
		if !unicode.IsLetter(r) || !unicode.IsLower(r) {
			if unicode.IsDigit(r) && i > 0 {
				continue
			}
			return fmt.Errorf("pack record: name %q must be lowercase letters, digits, and hyphens", name)
		}
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("pack record: name %q is not a single path segment", name)
	}
	return nil
}
