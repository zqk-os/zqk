package crud

import (
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/validation"
)

func AgentTaskWorkDoneRequiresCommit(status string) bool {
	sc := objects.GetGlobalStatusChecker()
	return sc.IsWorkDone(objects.KindAgentTask, status)
}

func GetObjectID(obj map[string]any) string {
	if id, ok := obj[objects.FieldKeyID].(string); ok {
		return id
	}
	return ""
}

func IsPlanMembershipHardBlockOnCreate(err validation.ValidationError) bool {
	if err.Rule == "scope_creep_protection" {
		return true
	}
	if err.Rule == "composed_integrity" &&
		(err.Field == objects.FieldKeyPriorityPlanRef || err.Field == objects.FieldKeyStatus) &&
		(strings.Contains(err.Message, "execution-facing") ||
			strings.Contains(err.Message, "cannot link to") ||
			strings.Contains(err.Message, "sealed") ||
			strings.Contains(err.Message, "Scope Creep")) {
		return true
	}
	return false
}

func IsPlanMembershipHardBlockError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "execution-facing") ||
		strings.Contains(msg, "sealed") ||
		strings.Contains(msg, "cannot link to") && strings.Contains(msg, "priority plan") ||
		strings.Contains(msg, "Scope Creep Protection") ||
		strings.Contains(msg, "open a grooming plan for new work")
}

func DetectPartialData(obj map[string]any, kind string) error {
	if kind == "" {
		return errfmt.Errorf("object missing required field 'kind'")
	}

	id, _ := obj[objects.FieldKeyID].(string)
	if id == "" {
		return errfmt.Errorf("object missing required field 'id'")
	}

	if len(obj) == 0 {
		return errfmt.Errorf("object is nil or empty")
	}

	if objKind := objects.GetString(obj, objects.FieldKeyKind); objKind != "" && objKind != kind {
		return errfmt.Errorf("kind mismatch: object has kind '%s' but expected '%s'", objKind, kind)
	}

	return nil
}

func NormalizeObjectValues(obj map[string]any, kind string) {
	objects.CoerceMutationFields(kind, obj)
	fieldRegistry := objects.GetGlobalFieldRegistry()
	kindFields, err := fieldRegistry.GetFieldsForKind(kind)
	if err != nil {
		return
	}

	fieldMap := make(map[string]*objects.FieldInfo)
	for i := range kindFields.AllFields {
		field := &kindFields.AllFields[i]
		fieldMap[field.Name] = field
	}

	for fieldName, value := range obj {
		fieldInfo, ok := fieldMap[fieldName]
		if !ok {
			continue
		}

		if fieldInfo.Type == "number" || fieldInfo.Type == "float" {
			switch v := value.(type) {
			case int:
				obj[fieldName] = float64(v)
			case int32:
				obj[fieldName] = float64(v)
			case int64:
				obj[fieldName] = float64(v)
			}
		}

		if fieldInfo.SemanticType == "timestamp" || fieldInfo.Type == "string" {
			if t, ok := value.(time.Time); ok {
				obj[fieldName] = t.Format(time.RFC3339)
			}
		}
	}
}
