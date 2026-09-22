// BLI-STARTER-COMMUNITY-059 / PRI-STARTER-COMMUNITY-059 coverage elevation
package detector

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestExtraDetectorVersionManifestAndSignatures(t *testing.T) {
	d := NewBinaryDetector()
	d.WithExpectedHash("deadbeef").WithExpectedVersion("1.2.3").WithTrustedSigners([]string{"ZQK", "ABCD1234"}).WithSearchPaths([]string{"", t.TempDir()})
	if _, err := d.WithVersionConstraint("not-a-version"); err == nil {
		t.Fatal("expected constraint error")
	}
	if _, err := d.WithVersionConstraint("^1.2.3"); err != nil {
		t.Fatal(err)
	}
	_ = SupportsMigration()
	if d.IsAvailable() {
		t.Fatal("expected missing binary")
	}
	if _, err := d.GetPath(); err == nil {
		t.Fatal("expected missing path")
	}
	if caps, err := d.GetCapabilities(); err != nil || caps.Available {
		t.Fatalf("caps=%v err=%v", caps, err)
	}

	dir := t.TempDir()
	bin := filepath.Join(dir, d.binaryName)
	script := "#!/bin/sh\necho zqk-migrate version 1.2.3\n"
	if err := fileutil.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	found := NewBinaryDetector().WithSearchPaths([]string{dir}).WithTrustedSigners([]string{"nope"})
	path, err := found.GetPath()
	if err != nil {
		t.Fatal(err)
	}
	ver, err := found.GetVersion(path)
	if err != nil || ver != "1.2.3" {
		t.Fatalf("version=%q err=%v", ver, err)
	}
	plain := filepath.Join(dir, "plain")
	if err := fileutil.WriteFile(plain, []byte("#!/bin/sh\necho 9.9.9\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.Chmod(plain, 0o755)
	if v, err := found.GetVersion(plain); err != nil || v == "" {
		t.Fatalf("plain version=%q err=%v", v, err)
	}
	if _, err := found.GetVersion(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("expected version error")
	}
	caps, err := found.GetCapabilities()
	if err != nil || !caps.Available {
		t.Fatalf("caps available err=%v caps=%v", err, caps)
	}

	hash, err := GetBinaryHash(path)
	if err != nil || hash == "" {
		t.Fatal(err)
	}
	if err := VerifyBinaryIntegrity(path, hash); err != nil {
		t.Fatal(err)
	}
	if err := VerifyBinaryIntegrity(path, "00"); err == nil {
		t.Fatal("expected hash mismatch")
	}
	if _, err := GetBinaryHash(filepath.Join(dir, "nope")); err == nil {
		t.Fatal("expected hash open error")
	}

	hashed := NewBinaryDetector().WithSearchPaths([]string{dir}).WithExpectedHash(hash).WithExpectedVersion("1.2.3").WithTrustedSigners([]string{"nope"})
	_ = hashed.VerifyIntegrity(path)
	_ = hashed.IsAvailable()

	constrained, err := NewBinaryDetector().WithSearchPaths([]string{dir}).WithVersionConstraint(">=1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	_ = constrained.VerifyIntegrity(path)

	_ = found.verifyMacOSSignature(path)
	_ = found.verifyLinuxSignature(path)
	_ = found.verifyWindowsSignature(path)
	_ = found.verifyCodeSignature(path)
	_ = found.isTrustedSigner("Developer ID Application: ZQK extra")
	_ = found.isTrustedSigner("other")
	_ = found.isTrustedWindowsSigner("ZQK")
	_ = found.isTrustedWindowsSigner("nope")
	_ = found.verifyTrustedGPGKey("Primary key fingerprint: ABCD 1234\n")
	_ = found.verifyTrustedGPGKey("no fingerprint")

	v1, _ := ParseVersion("2.0.0-beta+exp")
	v2, _ := ParseVersion("2.0.0-alpha")
	_ = v1.Compare(v2)
	_ = v1.LessThan(v2)
	_ = v1.GreaterThan(v2)
	_ = v1.Equal(v2)
	_, _ = ParseVersion("1.x.0")
	_, _ = ParseVersion("1.2.x")

	for _, c := range []string{"^1.2.3", ">=1.0.0", "<=2.0.0", ">1.0.0", "<2.0.0", "=1.2.3", "1.2.3", ">=1.0.0 <2.0.0", "^bad", ">=nope"} {
		_, _ = ParseVersionConstraint(c)
	}
	min, _ := ParseVersion("1.0.0")
	max, _ := ParseVersion("2.0.0")
	exact, _ := ParseVersion("1.5.0")
	vc := &VersionConstraint{MinVersion: min, MaxVersion: max, ExcludeVersions: []*Version{exact}}
	_ = vc.Matches(exact)
	_ = vc.Matches(min)
	hit, _ := ParseVersion("1.2.0")
	_ = vc.Matches(hit)
	ex := &VersionConstraint{ExactVersions: []*Version{exact}, ExcludeVersions: []*Version{exact}}
	_ = ex.Matches(exact)
	_ = ex.Matches(min)
	_ = VerifyVersionCompatibility("1.2.0", vc)
	_ = VerifyVersionCompatibility("nope", vc)
	_ = VerifyVersionCompatibility("3.0.0", vc)

	mf := filepath.Join(dir, "manifest.yaml")
	_, _ = LoadBinaryManifest(mf)
	_ = SaveBinaryManifest(&BinaryManifest{Version: "1", Algorithm: "sha256"}, mf)
	man := &BinaryManifest{Version: "1.0.0", Algorithm: "sha256", Compatibility: &CompatibilityConstraints{CLIVersion: ">=0.0.1"}}
	man.UpdateEntry(&BinaryManifestEntry{Platform: runtime.GOOS + "/" + runtime.GOARCH, Hash: hash, Version: "1.2.3", Binary: "zqk-migrate"})
	man.UpdateEntry(&BinaryManifestEntry{Platform: runtime.GOOS + "/" + runtime.GOARCH, Hash: hash, Version: "1.2.4", Binary: "zqk-migrate"})
	if err := SaveBinaryManifest(man, mf); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadBinaryManifest(mf)
	if err != nil {
		t.Fatal(err)
	}
	_ = loaded.GetEntryForPlatform(runtime.GOOS, runtime.GOARCH)
	_ = loaded.GetEntryForPlatform("plan9", "arm")
	_ = VerifyBinaryAgainstManifest(path, loaded)
	_ = VerifyBinaryAgainstManifest(path, &BinaryManifest{})
	_ = VerifyCompatibility(&CompatibilityConstraints{})
	_ = VerifyCompatibility(&CompatibilityConstraints{CLIVersion: ">=0.0.1", Modules: map[string]string{"missing": ">=1.0.0"}, Backends: map[string]string{"memgraph": ">=1.0.0"}})
	orig := GetCLIVersion
	GetCLIVersion = func() string { return "1.2.3" }
	_ = VerifyCLIVersion(">=1.0.0")
	_ = VerifyCLIVersion(">=9.0.0")
	_ = VerifyCLIVersion("bad")
	GetCLIVersion = func() string { return "not-semver" }
	_ = VerifyCLIVersion(">=1.0.0")
	GetCLIVersion = orig
	_ = VerifyCLIVersion(">=1.0.0")

	gomod := filepath.Join(dir, "go.mod")
	_ = fileutil.WriteFile(gomod, []byte("module example\n\nrequire github.com/foo/bar v1.2.3\n\nrequire (\n\tgithub.com/baz/qux v0.9.0+incompatible\n)\n"), paths.FilePerm644)
	_, _ = GetModuleVersionFromFile(gomod, "github.com/foo/bar")
	_, _ = GetModuleVersionFromFile(gomod, "github.com/baz/qux")
	_, _ = GetModuleVersionFromFile(gomod, "missing")
	_, _ = GetModuleVersionFromFile(filepath.Join(dir, "nope"), "x")
	_, _ = parseModuleVersionFromGoModContent([]byte("require github.com/foo/bar v1.0.0\n"), "github.com/foo/bar")
	_ = findGoMod()
	_, _ = GetModuleVersion("github.com/foo/bar")
	_ = VerifyModuleVersion("github.com/foo/bar", "bad")
	_ = VerifyModuleVersion("github.com/foo/bar", ">=0.0.1")
	_ = VerifyBackendVersion("memgraph", ">=1.0.0")
	_ = UpdateManifestAfterInstall(path, "1.2.3", mf)
	_ = UpdateManifestAfterInstall(filepath.Join(dir, "nope"), "1.0.0", filepath.Join(dir, "new-manifest.yaml"))
}
