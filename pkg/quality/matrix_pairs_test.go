package quality

import "testing"

func TestParseColumnValuePairs(t *testing.T) {
	t.Parallel()
	m, err := ParseColumnValuePairs([]string{"a=b", "c:d"}, false, "--filter")
	if err != nil {
		t.Fatal(err)
	}
	if m["a"] != "b" || m["c"] != "d" {
		t.Fatalf("%v", m)
	}
	_, err = ParseColumnValuePairs(nil, true, "--set")
	if err == nil {
		t.Fatal("expected error")
	}
}
