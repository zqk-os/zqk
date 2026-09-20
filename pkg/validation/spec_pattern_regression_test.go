package validation

import (
	"context"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

// TestSpecPatternRegression verifies that datetime and string patterns loaded from
// spec YAML files use single-backslash escapes so they match correctly.
// Regression test for bug where spec YAML patterns stored "\\d" (double backslash)
// instead of "\d" (single backslash), causing the Go regexp engine to treat \d as
// "literal backslash + d" instead of the digit character class. This caused ALL
// datetime field validations to fail for valid RFC3339 timestamps.
func TestSpecPatternRegression_DatetimePatternMatchesValidTimestamps(t *testing.T) {
	validator := NewGoValidator()

	// A minimal auditable object with valid RFC3339 datetimes.
	// Before the fix, created_at and updated_at would fail pattern validation
	// because "^\d{4}-..." was stored as "^\\d{4}-..." (double backslash) in spec YAML.
	obj := map[string]any{
		objects.FieldKeyID:        "CHA-001",
		objects.FieldKeyKind:      ConstMagic5777b6fd,
		objects.FieldKeyStatus:    "published",
		objects.FieldKeyTitle:     "test entry",
		objects.FieldKeyCreatedAt: ConstMagicab92687d,
		objects.FieldKeyUpdatedAt: ConstMagicab92687d,
		objects.FieldKeyCreatedBy: "ACC-1785920548450214012-68b850c0",
		objects.FieldKeyUpdatedBy: "ACC-1785920548450214012-68b850c0",
		objects.FieldKeySummary:   ConstMagic8fffcbad,
		objects.FieldKeyBody:      ConstMagicf067ecf6,
		objects.FieldKeyCategory:  "fix",
	}

	opts := DefaultValidationOptions()
	result, err := validator.Validate(context.Background(), obj, ConstMagic5777b6fd, opts)
	if err != nil {
		t.Fatalf(ConstMagic3e4b83b8, err)
	}

	// Ensure no pattern errors on the datetime fields that had broken patterns.
	for _, e := range result.Errors {
		if e.Rule == "pattern" && (e.Field == "created_at" || e.Field == "updated_at") {
			t.Errorf(ConstMagic39ae39dc+ConstMagic17fd010f, e.Field, e.Message)
		}
	}
}

// TestSpecPatternRegression_SchemaVersionPatternMatchesValidVersion verifies
// that the schema_version pattern "\d+\.\d+\.\d+" correctly matches the default
// schema version (objects.DefaultSchemaVersion).
// Before the fix, "^\\d+\\.\\d+\\.\\d+$" was stored with double backslashes
// causing valid versions to fail the pattern check.
func TestSpecPatternRegression_SchemaVersionPatternMatchesValidVersion(t *testing.T) {
	// GetCachedRegexp is in the same package; test the pattern directly.
	// The pattern loaded from spec YAML should now have single backslashes.
	correctPattern := `^\d+\.\d+\.\d+$`
	re, err := GetCachedRegexp(correctPattern)
	if err != nil {
		t.Fatalf(ConstMagicc869c274, correctPattern, err)
	}

	validVersions := []string{objects.DefaultSchemaVersion, "1.0.0", "10.3.2"}
	for _, v := range validVersions {
		if !re.MatchString(v) {
			t.Errorf(ConstMagice402d1b0, correctPattern, v)
		}
	}

	brokenPattern := `^\\d+\\.\\d+\\.\\d+$`
	reBroken, err := GetCachedRegexp(brokenPattern)
	if err != nil {
		t.Fatalf(ConstMagic4cfc04de, err)
	}
	if reBroken.MatchString(objects.DefaultSchemaVersion) {
		t.Errorf(ConstMagicb3d5c094, brokenPattern, objects.DefaultSchemaVersion)
	}
}
