package matrix

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"testing"

	clitool "github.com/zqk-os/zqk/pkg/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestMatrixVerifyCmd(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		SkipFileStorage: true,
		Kind:            "matrix_verify",
	})

	cleanGo := `package pkg

import "fmt"

func Hello() {
	fmt.Println("hello world")
}
`
	cleanFile := filepath.Join(proj.Root, "pkg", "clean.go")
	if err := fileutil.MkdirAll(filepath.Dir(cleanFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(cleanFile, []byte(cleanGo)); err != nil {
		t.Fatal(err)
	}

	badGo := `package pkg

const Version = "v1.2.3"
const Perm = 0777
`
	badFile := filepath.Join(proj.Root, "pkg", "bad.go")
	if err := fileutil.WriteSecureFile(badFile, []byte(badGo)); err != nil {
		t.Fatal(err)
	}

	baseCtx := pkgctx.WithLifecycleProjectRoot(context.Background(), proj.Root)

	t.Run("verify clean file", func(t *testing.T) {
		var buf bytes.Buffer
		root := clitool.NewCommandBuilder("zqk").Build()
		root.AddCommand(NewMatrixCmd())
		root.SetOut(&buf)
		root.SetErr(&buf)
		root.SetContext(pkgctx.WithCommandOutputWriter(baseCtx, &buf))
		root.SetArgs([]string{"matrix", "verify", "--file", "pkg/clean.go"})

		if err := root.Execute(); err != nil {
			t.Fatalf("unexpected error: %v, out: %s", err, buf.String())
		}
		if !bytes.Contains(buf.Bytes(), []byte("PASSED")) {
			t.Fatalf("expected PASSED in output, got: %s", buf.String())
		}
	})

	t.Run("verify bad file fails", func(t *testing.T) {
		var buf bytes.Buffer
		root := clitool.NewCommandBuilder("zqk").Build()
		root.AddCommand(NewMatrixCmd())
		root.SetOut(&buf)
		root.SetErr(&buf)
		root.SetContext(pkgctx.WithCommandOutputWriter(baseCtx, &buf))
		root.SetArgs([]string{"matrix", "verify", "--file", "pkg/bad.go"})

		err := root.Execute()
		if err == nil {
			t.Fatalf("expected error for violating file, got nil. out: %s", buf.String())
		}
		if !bytes.Contains(buf.Bytes(), []byte("FAILED")) {
			t.Fatalf("expected FAILED in output, got: %s", buf.String())
		}
	})

	t.Run("verify all with cache hit", func(t *testing.T) {
		// Remove bad file so evaluate all can succeed
		_ = fileutil.Remove(badFile)

		var buf bytes.Buffer
		root := clitool.NewCommandBuilder("zqk").Build()
		root.AddCommand(NewMatrixCmd())
		root.SetOut(&buf)
		root.SetErr(io.Discard)
		root.SetContext(pkgctx.WithCommandOutputWriter(baseCtx, &buf))
		root.SetArgs([]string{"matrix", "verify"})

		if err := root.Execute(); err != nil {
			t.Fatalf("unexpected error on first pass: %v, out: %s", err, buf.String())
		}
		if !bytes.Contains(buf.Bytes(), []byte("Content-Addressed File Verification Matrix")) {
			t.Fatalf("missing header: %s", buf.String())
		}

		// Second pass should have cache hits
		buf.Reset()
		root = clitool.NewCommandBuilder("zqk").Build()
		root.AddCommand(NewMatrixCmd())
		root.SetOut(&buf)
		root.SetErr(io.Discard)
		root.SetContext(pkgctx.WithCommandOutputWriter(baseCtx, &buf))
		root.SetArgs([]string{"matrix", "verify"})

		if err := root.Execute(); err != nil {
			t.Fatalf("unexpected error on second pass: %v, out: %s", err, buf.String())
		}
		if !bytes.Contains(buf.Bytes(), []byte("Cache Hits")) {
			t.Fatalf("missing cache hits output: %s", buf.String())
		}
	})
}

func TestMatrixDimensionsCmd(t *testing.T) {
	var buf bytes.Buffer
	root := clitool.NewCommandBuilder("zqk").Build()
	root.AddCommand(NewMatrixCmd())
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	root.SetArgs([]string{"matrix", "dimensions"})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v, out: %s", err, buf.String())
	}
	out := buf.String()
	for _, dim := range []string{"HCODE", "EFFPERF", "ERRHYG", "CONCURR", "SECOBS", "DOCSIG"} {
		if !bytes.Contains(buf.Bytes(), []byte(dim)) {
			t.Errorf("expected dimension %s in output, got:\n%s", dim, out)
		}
	}
}

func TestMatrixStampCmd(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		SkipFileStorage: true,
		Kind:            "matrix_stamp",
	})

	samplePath := filepath.Join(proj.Root, "pkg", "sample.go")
	if err := fileutil.MkdirAll(filepath.Dir(samplePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(samplePath, []byte("package sample\n")); err != nil {
		t.Fatal(err)
	}

	baseCtx := pkgctx.WithLifecycleProjectRoot(context.Background(), proj.Root)
	var buf bytes.Buffer
	root := clitool.NewCommandBuilder("zqk").Build()
	root.AddCommand(NewMatrixCmd())
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetContext(pkgctx.WithCommandOutputWriter(baseCtx, &buf))
	root.SetArgs([]string{
		"matrix", "stamp",
		"--file", "pkg/sample.go",
		"--dimension", "HCODE",
		"--status", "passed",
		"--score", "5",
		"--evaluator", "PER-HARDCODING-ERADICATION-CZAR",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error on stamp: %v, out: %s", err, buf.String())
	}
	if !bytes.Contains(buf.Bytes(), []byte("Stamped pkg/sample.go [HCODE] -> passed")) {
		t.Fatalf("unexpected stamp output: %s", buf.String())
	}
}

func TestMatrixInventoryCmd(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		SkipFileStorage: true,
		Kind:            "matrix_inventory",
	})
	baseCtx := pkgctx.WithLifecycleProjectRoot(context.Background(), proj.Root)

	var buf bytes.Buffer
	root := clitool.NewCommandBuilder("zqk").Build()
	root.AddCommand(NewMatrixCmd())
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetContext(pkgctx.WithCommandOutputWriter(baseCtx, &buf))
	root.SetArgs([]string{"matrix", "inventory"})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error on inventory: %v, out: %s", err, buf.String())
	}
	if !bytes.Contains(buf.Bytes(), []byte("Global String Literal Inventory & Deduplication Report")) {
		t.Fatalf("unexpected inventory output: %s", buf.String())
	}
}

func TestMatrixEvaluateCmd(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		SkipFileStorage: true,
		Kind:            "matrix_evaluate",
	})
	baseCtx := pkgctx.WithLifecycleProjectRoot(context.Background(), proj.Root)

	cleanGo := `package pkg

import "fmt"

func Hello() {
	fmt.Println("hello world")
}
`
	cleanFile := filepath.Join(proj.Root, "pkg", "clean.go")
	if err := fileutil.MkdirAll(filepath.Dir(cleanFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(cleanFile, []byte(cleanGo)); err != nil {
		t.Fatal(err)
	}

	badGo := `package pkg

const Version = "v1.2.3"
const Perm = 0777
`
	badFile := filepath.Join(proj.Root, "pkg", "bad.go")
	if err := fileutil.WriteSecureFile(badFile, []byte(badGo)); err != nil {
		t.Fatal(err)
	}

	t.Run("evaluate clean file passes", func(t *testing.T) {
		var buf bytes.Buffer
		root := clitool.NewCommandBuilder("zqk").Build()
		root.AddCommand(NewMatrixCmd())
		root.SetOut(&buf)
		root.SetErr(&buf)
		root.SetContext(pkgctx.WithCommandOutputWriter(baseCtx, &buf))
		root.SetArgs([]string{"matrix", "evaluate", "--file", "pkg/clean.go", "--auto-mint=false"})

		if err := root.Execute(); err != nil {
			t.Fatalf("unexpected error: %v, out: %s", err, buf.String())
		}
		if !bytes.Contains(buf.Bytes(), []byte("[PASSED]")) {
			t.Fatalf("expected [PASSED] in output, got: %s", buf.String())
		}
		if !bytes.Contains(buf.Bytes(), []byte("5/5")) {
			t.Fatalf("expected 5/5 score in output, got: %s", buf.String())
		}
	})

	t.Run("evaluate clean file json", func(t *testing.T) {
		var buf bytes.Buffer
		root := clitool.NewCommandBuilder("zqk").Build()
		root.AddCommand(NewMatrixCmd())
		root.SetOut(&buf)
		root.SetErr(&buf)
		root.SetContext(pkgctx.WithCommandOutputWriter(baseCtx, &buf))
		root.SetArgs([]string{"matrix", "evaluate", "--file", "pkg/clean.go", "--json", "--auto-mint=false"})

		if err := root.Execute(); err != nil {
			t.Fatalf("unexpected error: %v, out: %s", err, buf.String())
		}
		if !bytes.Contains(buf.Bytes(), []byte(`"file_path": "pkg/clean.go"`)) {
			t.Fatalf("expected json with file_path in output, got: %s", buf.String())
		}
		if !bytes.Contains(buf.Bytes(), []byte(`"passed": true`)) {
			t.Fatalf("expected passed: true in json output, got: %s", buf.String())
		}
	})

	t.Run("evaluate bad file fails policy check", func(t *testing.T) {
		var buf bytes.Buffer
		root := clitool.NewCommandBuilder("zqk").Build()
		root.AddCommand(NewMatrixCmd())
		root.SetOut(&buf)
		root.SetErr(&buf)
		root.SetContext(pkgctx.WithCommandOutputWriter(baseCtx, &buf))
		root.SetArgs([]string{"matrix", "evaluate", "--file", "pkg/bad.go", "--auto-mint=false"})

		err := root.Execute()
		if err == nil {
			t.Fatalf("expected error for bad file, got nil. out: %s", buf.String())
		}
		if !bytes.Contains(buf.Bytes(), []byte("[FAILED]")) {
			t.Fatalf("expected [FAILED] in output, got: %s", buf.String())
		}
	})
}
