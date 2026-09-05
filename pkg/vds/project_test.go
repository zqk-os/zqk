package vds

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestLookupProvider_default(t *testing.T) {
	cfg := DefaultVendorProviders()
	p, err := LookupProvider(cfg, "")
	if err != nil || p.ID != "ide" || p.Format != FormatIDEMDC {
		t.Fatalf("got %+v err=%v", p, err)
	}
}

func TestRenderProviderBody_markerUsesProviderID(t *testing.T) {
	prov := VendorProvider{ID: "ide", Format: FormatIDEMDC, Out: "x.mdc"}
	body, _, err := RenderProviderBody(context.Background(), prov, ProjectOptions{
		Spine:         &SpineProfile{Stages: []StageDef{{ID: "design", Purpose: "d"}}},
		Customization: &Customization{TestExecution: TestExecutionPrefs{Mode: "scheduler"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "vds project --provider ide") {
		t.Fatal(body)
	}
}

func TestWriteOrCheckProvider_roundTrip(t *testing.T) {
	dir := t.TempDir()
	cust := &Customization{
		TestExecution: TestExecutionPrefs{Mode: "ci"},
		VendorProviders: VendorProvidersConfig{
			Default: "ide",
			Providers: []VendorProvider{{
				ID: "ide", Format: FormatIDEMDC, Out: "rules/vds.mdc",
			}},
		},
	}
	opt := ProjectOptions{
		ProjectRoot:   dir,
		ProviderID:    "ide",
		Spine:         &SpineProfile{Stages: []StageDef{{ID: "implement", Purpose: "code"}}},
		Customization: cust,
	}
	if _, err := WriteOrCheckProvider(context.Background(), opt, false, true); err == nil {
		t.Fatal("expected missing file check fail")
	}
	res, err := WriteOrCheckProvider(context.Background(), opt, true, true)
	if err != nil || !res.Wrote {
		t.Fatalf("%+v %v", res, err)
	}
	if _, err := fileutil.Stat(filepath.Join(dir, "rules/vds.mdc")); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteOrCheckProvider(context.Background(), opt, false, true); err != nil {
		t.Fatal(err)
	}
}

func TestRenderProviderBody_unsupportedFormat(t *testing.T) {
	_, _, err := RenderProviderBody(context.Background(), VendorProvider{
		ID: "x", Format: "nope_format", Out: "x",
	}, ProjectOptions{})
	if err == nil {
		t.Fatal("expected unsupported format")
	}
}

func TestWriteOrCheckAllProviders(t *testing.T) {
	dir := t.TempDir()
	cust := &Customization{
		VendorProviders: VendorProvidersConfig{
			Default: "a",
			Providers: []VendorProvider{
				{ID: "a", Format: FormatIDEMDC, Out: "out/a.mdc"},
				{ID: "b", Format: FormatIDEMDC, Out: "out/b.mdc"},
			},
		},
	}
	opt := ProjectOptions{
		ProjectRoot:   dir,
		Spine:         &SpineProfile{Stages: []StageDef{{ID: "design", Purpose: "d"}}},
		Customization: cust,
	}
	batch, err := WriteOrCheckAllProviders(context.Background(), opt, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Results) != 2 {
		t.Fatalf("results=%d", len(batch.Results))
	}
	batch2, err := WriteOrCheckAllProviders(context.Background(), opt, false, true)
	if err != nil || len(batch2.Failed) != 0 {
		t.Fatalf("%+v err=%v", batch2, err)
	}
}

func TestWriteOrCheckAllProviders_requiresWriteOrCheck(t *testing.T) {
	_, err := WriteOrCheckAllProviders(context.Background(), ProjectOptions{ProjectRoot: t.TempDir()}, false, false)
	if err == nil {
		t.Fatal("expected error")
	}
}
