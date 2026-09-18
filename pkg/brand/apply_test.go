package brand

import "testing"

func TestApplyCanonicalExecutable_preservesDataDir(t *testing.T) {
	in := "Kernel data stays under `.zqk/`. Run `./bin/zqk system init`."
	got := ApplyCanonicalExecutable(in, "zcom")
	want := "Kernel data stays under `.zqk/`. Run `./bin/zcom system init`."
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestApplyCanonicalExecutable_preservesHyphenatedIdentifiers(t *testing.T) {
	in := "config/zqk.yaml and zqk-stable stay"
	got := ApplyCanonicalExecutable(in, "zcom")
	if got != in {
		t.Fatalf("hyphenated identifiers rewritten: %q", got)
	}
}

func TestApplyCanonicalExecutable_rewritesEnvPrefix(t *testing.T) {
	in := "Do not export ZQK_PROJECT_ROOT."
	got := ApplyCanonicalExecutable(in, "zcom")
	want := "Do not export ZCOM_PROJECT_ROOT."
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestApplyCanonicalExecutable_canonicalIsNoOp(t *testing.T) {
	in := "./bin/zqk object list"
	got := ApplyCanonicalExecutable(in, "zqk")
	if got != in {
		t.Fatalf("canonical rewrite: %q", got)
	}
}
