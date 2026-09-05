package quick

import (
	"testing"
)

const (
	testEmptyValue = ""
)

func TestParseMarkdownOrText_Empty(t *testing.T) {
	t.Parallel()
	out := ParseMarkdownOrText(testEmptyValue)
	if out.Title != testEmptyValue || out.Body != testEmptyValue {
		t.Errorf("empty input: got title=%q body=%q", out.Title, out.Body)
	}
	out = ParseMarkdownOrText("   \n\n  ")
	if out.Title != testEmptyValue || out.Body != testEmptyValue {
		t.Errorf("whitespace only: got title=%q body=%q", out.Title, out.Body)
	}
}

func TestParseMarkdownOrText_FirstLineHeading(t *testing.T) {
	t.Parallel()
	in := "# My Title\n\nSome body here."
	out := ParseMarkdownOrText(in)
	if out.Title != "My Title" {
		t.Errorf("title: got %q", out.Title)
	}
	if out.Body != "Some body here." {
		t.Errorf("body: got %q", out.Body)
	}
}

func TestParseMarkdownOrText_FirstLinePlain(t *testing.T) {
	t.Parallel()
	in := "Plain first line\n\nRest of body."
	out := ParseMarkdownOrText(in)
	if out.Title != "Plain first line" {
		t.Errorf("title: got %q", out.Title)
	}
	if out.Body != "Rest of body." {
		t.Errorf("body: got %q", out.Body)
	}
}

func TestParseMarkdownOrText_HeadingWithExtraHash(t *testing.T) {
	t.Parallel()
	in := "## Sub title\nBody"
	out := ParseMarkdownOrText(in)
	if out.Title != "Sub title" {
		t.Errorf("title: got %q", out.Title)
	}
	if out.Body != "Body" {
		t.Errorf("body: got %q", out.Body)
	}
}

func TestParseMarkdownOrText_TitleOverrideFromContent(t *testing.T) {
	t.Parallel()
	in := "Only one line"
	out := ParseMarkdownOrText(in)
	if out.Title != "Only one line" {
		t.Errorf("title: got %q", out.Title)
	}
	if out.Body != testEmptyValue {
		t.Errorf("body: got %q", out.Body)
	}
}

func TestParseFileContent(t *testing.T) {
	t.Parallel()
	raw := []byte("# File title\n\nFile body.")
	out := ParseFileContent(raw)
	if out.Title != "File title" {
		t.Errorf("title: got %q", out.Title)
	}
	if out.Body != "File body." {
		t.Errorf("body: got %q", out.Body)
	}
}
