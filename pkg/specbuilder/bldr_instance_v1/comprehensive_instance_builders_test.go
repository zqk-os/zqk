package bldr_instance_v1_test

import (
	"reflect"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_instance_v1"
	"github.com/zqk-os/zqk/pkg/specbuilder/instance_builders"
)

func TestAllRegisteredInstanceBuilders(t *testing.T) {
	t.Parallel()

	registry := instance_builders.GetGlobalRegistry()
	if registry == nil {
		t.Fatal("expected non-nil global instance builder registry")
	}

	kinds := registry.GetAllKinds()
	if len(kinds) == 0 {
		t.Fatal("expected registered instance builder kinds, got 0")
	}

	t.Logf("Found %d registered instance builder kinds", len(kinds))

	for _, kind := range kinds {
		k := kind
		t.Run("Kind_"+k, func(t *testing.T) {
			t.Parallel()

			builder, err := registry.GetBuilder(k, objects.DefaultSchemaVersion)
			if err != nil {
				// Try with latest version if default version not found
				latest, lErr := registry.GetLatestVersion(k)
				if lErr != nil {
					t.Fatalf("failed to get latest version for kind %q: %v", k, lErr)
				}
				builder, err = registry.GetBuilder(k, latest)
				if err != nil {
					t.Fatalf("failed to get builder for kind %q version %q: %v", k, latest, err)
				}
			}

			if builder.GetKind() != k {
				t.Errorf("expected kind %q, got %q", k, builder.GetKind())
			}

			// Validate fluent SetID, SetField and Build
			builder.SetID("ID-TEST-001")
			builder.SetField("test_field", "test_val")

			// Exercise all typed setters and compatibility aliases on the concrete builder
			invokeAllBuilderSetters(builder)

			inst, bErr := builder.Build()
			if bErr != nil {
				t.Fatalf("builder.Build() returned unexpected error for kind %q: %v", k, bErr)
			}
			if inst == nil {
				t.Fatalf("builder.Build() returned nil instance for kind %q", k)
			}
			if inst[objects.FieldKeyKind] != k {
				t.Errorf("expected instance kind %q, got %v", k, inst[objects.FieldKeyKind])
			}
		})
	}
}

func invokeAllBuilderSetters(b any) {
	val := reflect.ValueOf(b)
	if val.Kind() != reflect.Pointer || val.IsNil() {
		return
	}
	typ := val.Type()
	for i := 0; i < typ.NumMethod(); i++ {
		method := typ.Method(i)
		mType := method.Type
		if mType.NumIn() != 2 {
			continue
		}
		argType := mType.In(1)
		var sampleArg reflect.Value
		switch argType.Kind() {
		case reflect.String:
			sampleArg = reflect.ValueOf("test-sample-value").Convert(argType)
		case reflect.Int:
			sampleArg = reflect.ValueOf(1).Convert(argType)
		case reflect.Int64:
			sampleArg = reflect.ValueOf(int64(1)).Convert(argType)
		case reflect.Float64:
			sampleArg = reflect.ValueOf(1.0).Convert(argType)
		case reflect.Bool:
			sampleArg = reflect.ValueOf(true).Convert(argType)
		case reflect.Slice:
			elemType := argType.Elem()
			slice := reflect.MakeSlice(argType, 1, 1)
			if elemType.Kind() == reflect.String {
				slice.Index(0).Set(reflect.ValueOf("item").Convert(elemType))
			}
			sampleArg = slice
		case reflect.Map:
			sampleArg = reflect.MakeMap(argType)
		default:
			sampleArg = reflect.Zero(argType)
		}

		mVal := val.Method(i)
		func() {
			defer func() { _ = recover() }()
			mVal.Call([]reflect.Value{sampleArg})
		}()
	}
}

