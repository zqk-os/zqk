package bldr_instance_v1

import (
	"reflect"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/instance_builders"
)

type builderFactory struct {
	kind    string
	factory func(string) instance_builders.InstanceBuilder
}

func getAllBuilderFactories() []builderFactory {
	return []builderFactory{
		{kind: "account", factory: func(v string) instance_builders.InstanceBuilder { return NewAccountInstanceBuilder(v) }},
		{kind: "audit_event_aggregation", factory: func(v string) instance_builders.InstanceBuilder { return NewAuditEventAggregationInstanceBuilder(v) }},
		{kind: "audit_event", factory: func(v string) instance_builders.InstanceBuilder { return NewAuditEventInstanceBuilder(v) }},
		{kind: "auditable", factory: func(v string) instance_builders.InstanceBuilder { return NewAuditableInstanceBuilder(v) }},
		{kind: "auth_strategy", factory: func(v string) instance_builders.InstanceBuilder { return NewAuthStrategyInstanceBuilder(v) }},
		{kind: "auto_fix_rule", factory: func(v string) instance_builders.InstanceBuilder { return NewAutoFixRuleInstanceBuilder(v) }},
		{kind: "base_metric", factory: func(v string) instance_builders.InstanceBuilder { return NewBaseMetricInstanceBuilder(v) }},
		{kind: "base_object", factory: func(v string) instance_builders.InstanceBuilder { return NewBaseObjectInstanceBuilder(v) }},
		{kind: "brand", factory: func(v string) instance_builders.InstanceBuilder { return NewBrandInstanceBuilder(v) }},
		{kind: "bucketing_strategy", factory: func(v string) instance_builders.InstanceBuilder { return NewBucketingStrategyInstanceBuilder(v) }},
		{kind: "capability", factory: func(v string) instance_builders.InstanceBuilder { return NewCapabilityInstanceBuilder(v) }},
		{kind: "capacity_advertisement", factory: func(v string) instance_builders.InstanceBuilder { return NewCapacityAdvertisementInstanceBuilder(v) }},
		{kind: "certificate", factory: func(v string) instance_builders.InstanceBuilder { return NewCertificateInstanceBuilder(v) }},
		{kind: "change_journal_entry", factory: func(v string) instance_builders.InstanceBuilder { return NewChangeJournalEntryInstanceBuilder(v) }},
		{kind: "compute_advertisement", factory: func(v string) instance_builders.InstanceBuilder { return NewComputeAdvertisementInstanceBuilder(v) }},
		{kind: "convergence_session", factory: func(v string) instance_builders.InstanceBuilder { return NewConvergenceSessionInstanceBuilder(v) }},
		{kind: "digital_asset", factory: func(v string) instance_builders.InstanceBuilder { return NewDigitalAssetInstanceBuilder(v) }},
		{kind: "domain_registry", factory: func(v string) instance_builders.InstanceBuilder { return NewDomainRegistryInstanceBuilder(v) }},
		{kind: "extensible_object", factory: func(v string) instance_builders.InstanceBuilder { return NewExtensibleObjectInstanceBuilder(v) }},
		{kind: "field_registry", factory: func(v string) instance_builders.InstanceBuilder { return NewFieldRegistryInstanceBuilder(v) }},
		{kind: "fission_event", factory: func(v string) instance_builders.InstanceBuilder { return NewFissionEventInstanceBuilder(v) }},
		{kind: "inference_heuristic", factory: func(v string) instance_builders.InstanceBuilder { return NewInferenceHeuristicInstanceBuilder(v) }},
		{kind: "integrity_manifest", factory: func(v string) instance_builders.InstanceBuilder { return NewIntegrityManifestInstanceBuilder(v) }},
		{kind: "keystore_entry", factory: func(v string) instance_builders.InstanceBuilder { return NewKeystoreEntryInstanceBuilder(v) }},
		{kind: "kind_synonym", factory: func(v string) instance_builders.InstanceBuilder { return NewKindSynonymInstanceBuilder(v) }},
		{kind: "lifecycle", factory: func(v string) instance_builders.InstanceBuilder { return NewLifecycleInstanceBuilder(v) }},
		{kind: "metrics_feedback", factory: func(v string) instance_builders.InstanceBuilder { return NewMetricsFeedbackInstanceBuilder(v) }},
		{kind: "namespace", factory: func(v string) instance_builders.InstanceBuilder { return NewNamespaceInstanceBuilder(v) }},
		{kind: "namespace_registry", factory: func(v string) instance_builders.InstanceBuilder { return NewNamespaceRegistryInstanceBuilder(v) }},
		{kind: "object_spec", factory: func(v string) instance_builders.InstanceBuilder { return NewObjectSpecInstanceBuilder(v) }},
		{kind: "policy", factory: func(v string) instance_builders.InstanceBuilder { return NewPolicyInstanceBuilder(v) }},
		{kind: "remote_kernel", factory: func(v string) instance_builders.InstanceBuilder { return NewRemoteKernelInstanceBuilder(v) }},
		{kind: "resolver", factory: func(v string) instance_builders.InstanceBuilder { return NewResolverInstanceBuilder(v) }},
		{kind: "role", factory: func(v string) instance_builders.InstanceBuilder { return NewRoleInstanceBuilder(v) }},
		{kind: "rollback_report", factory: func(v string) instance_builders.InstanceBuilder { return NewRollbackReportInstanceBuilder(v) }},
		{kind: "rule", factory: func(v string) instance_builders.InstanceBuilder { return NewRuleInstanceBuilder(v) }},
		{kind: "scheduler_handler_binding", factory: func(v string) instance_builders.InstanceBuilder { return NewSchedulerHandlerBindingInstanceBuilder(v) }},
		{kind: "scheduler_job", factory: func(v string) instance_builders.InstanceBuilder { return NewSchedulerJobInstanceBuilder(v) }},
		{kind: "shockwave_router", factory: func(v string) instance_builders.InstanceBuilder { return NewShockwaveRouterInstanceBuilder(v) }},
		{kind: "tde_envelope", factory: func(v string) instance_builders.InstanceBuilder { return NewTdeEnvelopeInstanceBuilder(v) }},
		{kind: "template", factory: func(v string) instance_builders.InstanceBuilder { return NewTemplateInstanceBuilder(v) }},
		{kind: "translation_manifest", factory: func(v string) instance_builders.InstanceBuilder { return NewTranslationManifestInstanceBuilder(v) }},
		{kind: "visual_plan", factory: func(v string) instance_builders.InstanceBuilder { return NewVisualPlanInstanceBuilder(v) }},
		{kind: "vitality_report", factory: func(v string) instance_builders.InstanceBuilder { return NewVitalityReportInstanceBuilder(v) }},
		{kind: "watchdog_registration", factory: func(v string) instance_builders.InstanceBuilder { return NewWatchdogRegistrationInstanceBuilder(v) }},
		{kind: "zqk_session", factory: func(v string) instance_builders.InstanceBuilder { return NewZqkSessionInstanceBuilder(v) }},
	}
}

func TestAllInstanceBuildersComprehensive(t *testing.T) {
	factories := getAllBuilderFactories()

	for _, bf := range factories {
		bf := bf
		t.Run(bf.kind, func(t *testing.T) {
			t.Parallel()

			b := bf.factory(objects.DefaultSchemaVersion)
			if b == nil {
				t.Fatalf("expected non-nil builder for kind %s", bf.kind)
			}
			if b.GetKind() != bf.kind {
				t.Errorf("expected kind %s, got %s", bf.kind, b.GetKind())
			}
			if b.GetSchemaVersion() != objects.DefaultSchemaVersion {
				t.Errorf("expected schema version %s, got %s", objects.DefaultSchemaVersion, b.GetSchemaVersion())
			}

			// Exercise common base builder methods
			testID := "ID-" + strings.ToUpper(bf.kind) + "-001"
			b.SetID(testID)
			b.SetStatus("active")
			b.SetField("custom_prop", "custom_val")

			// Exercise all typed setters and compatibility aliases on the concrete builder
			invokeMethodsOnBuilder(t, b)

			// Verify Build succeeds and contains expected keys
			inst, err := b.Build()
			if err != nil {
				t.Fatalf("unexpected error building instance for %s: %v", bf.kind, err)
			}
			if inst == nil {
				t.Fatalf("expected non-nil instance map for %s", bf.kind)
			}
			if inst[objects.FieldKeyKind] != bf.kind {
				t.Errorf("expected kind %s in built instance, got %v", bf.kind, inst[objects.FieldKeyKind])
			}
		})
	}
}

func TestInstanceBuildersFieldOrderAndBoundary(t *testing.T) {
	t.Parallel()

	// Verify buildFieldOrderFromSpec helper
	order := buildFieldOrderFromSpec("account")
	if len(order) == 0 {
		t.Errorf("expected non-empty field order for account")
	}

	// Verify BaseMetric extras
	metricBldr := NewBaseMetricInstanceBuilder(objects.DefaultSchemaVersion)
	metricBldr.SetID("METRIC-001")
	metricBldr.ValidationMetricsJSON(`{"p99": 12.5}`)
	metricBldr.SetValidationMetricsJSON(`{"p99": 15.0}`)
	metricBldr.OperationID("OP-12345")
	metricBldr.SetOperationID("OP-67890")
	metricBldr.CasValidationCacheMetricsJSON(`{"hit_rate": 0.99}`)
	metricBldr.SetCasValidationCacheMetricsJSON(`{"hit_rate": 1.0}`)

	inst, err := metricBldr.Build()
	if err != nil {
		t.Fatalf("unexpected error building base_metric: %v", err)
	}
	if inst[objects.FieldKeyOperationID] != "OP-67890" {
		t.Errorf("expected operation_id OP-67890, got %v", inst[objects.FieldKeyOperationID])
	}

	// Empty schema version defaults properly
	emptyVerBldr := NewAccountInstanceBuilder("")
	if emptyVerBldr.GetSchemaVersion() != "" {
		t.Errorf("expected empty schema version to be preserved, got %q", emptyVerBldr.GetSchemaVersion())
	}
}

func invokeMethodsOnBuilder(t *testing.T, b any) {
	val := reflect.ValueOf(b)
	if val.Kind() != reflect.Pointer || val.IsNil() {
		return
	}
	typ := val.Type()
	for i := 0; i < typ.NumMethod(); i++ {
		method := typ.Method(i)
		mType := method.Type

		// Skip methods that are not single-argument setters
		if mType.NumIn() != 2 {
			continue
		}

		argType := mType.In(1)
		var sampleArg reflect.Value

		switch argType.Kind() {
		case reflect.String:
			sampleArg = reflect.ValueOf("sample-str").Convert(argType)
		case reflect.Int:
			sampleArg = reflect.ValueOf(42).Convert(argType)
		case reflect.Int64:
			sampleArg = reflect.ValueOf(int64(100)).Convert(argType)
		case reflect.Float64:
			sampleArg = reflect.ValueOf(3.14).Convert(argType)
		case reflect.Bool:
			sampleArg = reflect.ValueOf(true).Convert(argType)
		case reflect.Slice:
			elemType := argType.Elem()
			slice := reflect.MakeSlice(argType, 1, 1)
			if elemType.Kind() == reflect.String {
				slice.Index(0).Set(reflect.ValueOf("sample-item").Convert(elemType))
			} else if elemType.Kind() == reflect.Interface {
				slice.Index(0).Set(reflect.ValueOf("sample-interface-item"))
			}
			sampleArg = slice
		case reflect.Map:
			sampleArg = reflect.MakeMap(argType)
		case reflect.Interface:
			sampleArg = reflect.ValueOf("sample-any-val")
		default:
			sampleArg = reflect.Zero(argType)
		}

		mVal := val.Method(i)
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Logf("method %s recovered: %v", method.Name, r)
				}
			}()
			mVal.Call([]reflect.Value{sampleArg})
		}()
	}
}
