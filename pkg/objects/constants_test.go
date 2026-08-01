package objects

import "testing"

func TestValidSchemaVersion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		in     string
		expect string
	}{
		{"valid 2.0.0", DefaultSchemaVersion, DefaultSchemaVersion},
		{"valid 1.0.0", "1.0.0", "1.0.0"},
		{"valid 10.20.30", "10.20.30", "10.20.30"},
		{"empty", "", DefaultSchemaVersion},
		{"missing patch", "2.0", DefaultSchemaVersion},
		{"non-semver", "v2.0.0", DefaultSchemaVersion},
		{"with prefix", "2.0.0-beta", DefaultSchemaVersion},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidSchemaVersion(tt.in)
			if got != tt.expect {
				t.Errorf("ValidSchemaVersion(%q) = %q, want %q", tt.in, got, tt.expect)
			}
		})
	}
}
