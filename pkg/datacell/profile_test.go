package datacell

import "testing"

func TestParseStorageProfile(t *testing.T) {
	t.Parallel()
	if _, err := ParseStorageProfile(""); err != nil {
		t.Fatalf("empty: %v", err)
	}
	p, err := ParseStorageProfile("cas_entity")
	if err != nil || p != ProfileCASEntity {
		t.Fatalf("cas_entity: got %v %v", p, err)
	}
	if _, err := ParseStorageProfile("nope"); err == nil {
		t.Fatal("expected error for unknown profile")
	}
}

func TestKnownStorageProfiles_parseStringRoundTrip(t *testing.T) {
	t.Parallel()
	for _, p := range KnownStorageProfiles {
		t.Run(string(p), func(t *testing.T) {
			t.Parallel()
			wire := string(p)
			got, err := ParseStorageProfile(wire)
			if err != nil || got != p {
				t.Fatalf("ParseStorageProfile(%q): got %v err=%v", wire, got, err)
			}
			if got.String() != wire {
				t.Fatalf("String: want %q got %q", wire, got.String())
			}
			if !got.IsKnown() {
				t.Fatal("IsKnown expected true")
			}
		})
	}
}

func TestParseStorageProfile_allWireValues(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want StorageProfile
	}{
		{"cas_entity", ProfileCASEntity},
		{"light_file", ProfileLightFile},
		{"stream", ProfileStream},
	}
	for _, tc := range cases {
		got, err := ParseStorageProfile(tc.in)
		if err != nil || got != tc.want {
			t.Fatalf("%q: got %v %v", tc.in, got, err)
		}
	}
}

func TestStorageProfile_IsKnown(t *testing.T) {
	t.Parallel()
	if !ProfileStream.IsKnown() {
		t.Fatal("stream should be known")
	}
	if !ProfileLightFile.IsKnown() {
		t.Fatal("light_file should be known")
	}
	if StorageProfile("other").IsKnown() {
		t.Fatal("other should not be known")
	}
}
