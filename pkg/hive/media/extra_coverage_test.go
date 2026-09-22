// BLI-STARTER-COMMUNITY-040 / PRI-STARTER-COMMUNITY-040 coverage elevation
package media

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestExtraParseAndDownload(t *testing.T) {
	if _, err := ParseVisualPlan(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Fatal("missing plan")
	}
	bad := filepath.Join(t.TempDir(), "bad.yaml")
	if err := fileutil.WriteFile(bad, []byte("{not yaml"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseVisualPlan(bad); err == nil {
		t.Fatal("bad yaml")
	}
	ok := filepath.Join(t.TempDir(), "plan.yaml")
	if err := fileutil.WriteFile(ok, []byte("id: vis-1\nscenes:\n  - visual: hello\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	plan, err := ParseVisualPlan(ok)
	if err != nil || plan["id"] != "vis-1" {
		t.Fatalf("plan = %#v %v", plan, err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ok" {
			_, _ = w.Write([]byte("asset"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	dest := filepath.Join(t.TempDir(), "out.bin")
	if err := DownloadAsset(srv.URL+"/ok", dest); err != nil {
		t.Fatal(err)
	}
	if err := DownloadAsset(srv.URL+"/missing", dest); err == nil {
		t.Fatal("expected bad status")
	}
	if err := RenderCommercial(context.Background(), filepath.Join(t.TempDir(), "no.yaml"), t.TempDir()); err == nil {
		t.Fatal("expected missing plan render")
	}
	nos := filepath.Join(t.TempDir(), "noscenes.yaml")
	if err := fileutil.WriteFile(nos, []byte("id: vis-2\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := RenderCommercial(t.Context(), nos, t.TempDir()); err == nil {
		t.Fatal("expected missing scenes")
	}
}
