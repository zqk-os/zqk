// Extracted from compressed_snapshot.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/when"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
)

func CompressObjects(objects []map[string]any, dict *SnapshotDictionary, refTime time.Time) ([]CompressedObject, error) {
	rev := buildReverseDictionary(dict, refTime)
	compressed := make([]CompressedObject, 0, len(objects))

	for _, obj := range objects {
		compressedObj, err := compressObject(obj, rev)
		if err != nil {
			return nil, errfmt.Newf(ConstMiscFailedToCompressObject).Wrap(err)
		}
		compressed = append(compressed, compressedObj)
	}

	return compressed, nil
}

func compressObject(obj map[string]any, rev *reverseDictionary) (CompressedObject, error) {
	compressed := make(CompressedObject)

	for key, value := range obj {
		fieldID, hasFieldID := rev.fieldNames[key]
		fieldKey := key
		if hasFieldID {
			fieldKey = fmt.Sprintf("f%d", fieldID)
		}

		compressedValue, err := compressValue(value, key, rev)
		if err != nil {
			return nil, errfmt.Errorf(ConstMiscFailedToCompressValueForFieldSW, key, err)
		}
		compressed[fieldKey] = compressedValue
	}

	return compressed, nil
}

func compressValue(value any, fieldPath string, rev *reverseDictionary) (any, error) {
	switch v := value.(type) {
	case map[string]any:
		return compressObject(v, rev)
	case []any:
		compressed := make([]any, 0, len(v))
		for _, elem := range v {
			compressedElem, err := compressValue(elem, fieldPath, rev)
			if err != nil {
				return nil, err
			}
			compressed = append(compressed, compressedElem)
		}
		return compressed, nil
	case string:
		// 1. Delta encoding for timestamps
		if isTimestampField(fieldPath) && !rev.referenceTimestamp.IsZero() {
			if ts, err := time.Parse(time.RFC3339, v); err == nil {
				delta := ts.Sub(rev.referenceTimestamp).Seconds()
				return fmt.Sprintf("t%d", int64(delta)), nil
			}
		}

		// 2. Dictionary reference
		if valueID, hasValueID := rev.values[v]; hasValueID {
			return fmt.Sprintf("v%d", valueID), nil
		}

		// 3. Pattern reference
		if pattern := extractPattern(v); pattern != emptyValue {
			if patternID, hasPatternID := rev.patterns[pattern]; hasPatternID {
				return fmt.Sprintf("p%d:%s", patternID, v), nil
			}
		}

		return v, nil
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return 0.0, nil
		}
		return v, nil
	case float32:
		f := float64(v)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return float32(0.0), nil
		}
		return v, nil
	default:
		return v, nil
	}
}

// ExpandObjects expands compressed objects back to original format
func ExpandObjects(compressed []CompressedObject, dict *SnapshotDictionary, refTime time.Time) ([]map[string]any, error) {
	expanded := make([]map[string]any, 0, len(compressed))

	for _, compObj := range compressed {
		expandedObj, err := expandObject(compObj, dict, refTime)
		if err != nil {
			return nil, errfmt.Newf(ConstMiscFailedToExpandObject).Wrap(err)
		}
		expanded = append(expanded, expandedObj)
	}

	return expanded, nil
}

func expandObject(compObj CompressedObject, dict *SnapshotDictionary, refTime time.Time) (map[string]any, error) {
	expanded := make(map[string]any)

	for key, value := range compObj {
		var fieldName string
		when.When(func() bool { return strings.HasPrefix(key, "f") }).Then(func() {
			var fieldID int
			when.When(func() bool { _, err := fmt.Sscanf(key, "f%d", &fieldID); return err != nil }).Then(func() {
				fieldName = key
			}).OrElseWhen(func() bool { _, ok := dict.FieldNames[fieldID]; return ok }).Then(func() {
				fieldName = dict.FieldNames[fieldID]
			}).OrElse(func() {
				fieldName = key
			}).Run()
		}).OrElse(func() {
			fieldName = key
		}).Run()

		expandedValue, err := expandValue(value, fieldName, dict, refTime)
		if err != nil {
			return nil, errfmt.Errorf(ConstMiscFailedToExpandValueForFieldSW, fieldName, err)
		}
		expanded[fieldName] = expandedValue
	}

	return expanded, nil
}

func expandValue(value any, fieldPath string, dict *SnapshotDictionary, refTime time.Time) (any, error) {
	switch v := value.(type) {
	case CompressedObject:
		return expandObject(map[string]any(v), dict, refTime)
	case map[string]any:
		return expandObject(v, dict, refTime)
	}

	switch v := value.(type) {
	case []any:
		expanded := make([]any, 0, len(v))
		for _, elem := range v {
			expandedElem, err := expandValue(elem, fieldPath, dict, refTime)
			if err != nil {
				return nil, err
			}
			expanded = append(expanded, expandedElem)
		}
		return expanded, nil
	case string:
		// 1. Timestamp delta
		if strings.HasPrefix(v, "t") && !refTime.IsZero() {
			var delta int64
			if _, err := fmt.Sscanf(v, "t%d", &delta); err == nil {
				return refTime.Add(time.Duration(delta) * time.Second).Format(time.RFC3339), nil
			}
		}

		// 2. Dictionary reference
		if strings.HasPrefix(v, "v") {
			var valueID int
			if _, err := fmt.Sscanf(v, "v%d", &valueID); err == nil {
				if val, ok := dict.Values[valueID]; ok {
					return val, nil
				}
			}
		}

		// 3. Pattern reference
		if strings.HasPrefix(v, "p") {
			var patternID int
			var patternValue string
			if _, err := fmt.Sscanf(v, "p%d:%s", &patternID, &patternValue); err == nil {
				return patternValue, nil
			}
		}

		return v, nil
	default:
		return v, nil
	}
}

// CalculateChecksum calculates SHA256 checksum of compressed snapshot data
func CalculateChecksum(cs *CompressedSnapshot) (string, error) {
	var data any = cs.Data
	if cs != nil && cs.Data != nil {
		data = sanitizeNonFiniteFloats(cs.Data)
	}
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return "", errfmt.Newf(ConstMiscFailedToMarshalDataForChecksum).Wrap(err)
	}

	hash := sha256.Sum256(dataJSON)
	return fmt.Sprintf("%x", hash), nil
}

func sanitizeNonFiniteFloats(v any) any {
	switch val := v.(type) {
	case float64:
		if math.IsNaN(val) || math.IsInf(val, 0) {
			return 0.0
		}
		return val
	case float32:
		f := float64(val)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return float32(0.0)
		}
		return val
	case map[string]any:
		m := make(map[string]any, len(val))
		for k, item := range val {
			m[k] = sanitizeNonFiniteFloats(item)
		}
		return m
	case CompressedObject:
		m := make(CompressedObject, len(val))
		for k, item := range val {
			m[k] = sanitizeNonFiniteFloats(item)
		}
		return m
	case []CompressedObject:
		s := make([]CompressedObject, len(val))
		for i, item := range val {
			if co, ok := sanitizeNonFiniteFloats(item).(CompressedObject); ok {
				s[i] = co
			} else if m, ok := sanitizeNonFiniteFloats(item).(map[string]any); ok {
				s[i] = CompressedObject(m)
			}
		}
		return s
	case []map[string]any:
		s := make([]map[string]any, len(val))
		for i, item := range val {
			if sm, ok := sanitizeNonFiniteFloats(item).(map[string]any); ok {
				s[i] = sm
			} else {
				s[i] = item
			}
		}
		return s
	case []any:
		s := make([]any, len(val))
		for i, item := range val {
			s[i] = sanitizeNonFiniteFloats(item)
		}
		return s
	case *SnapshotData:
		if val == nil {
			return nil
		}
		var objs []CompressedObject
		if val.Objects != nil {
			objs = sanitizeNonFiniteFloats(val.Objects).([]CompressedObject)
		}
		return &SnapshotData{
			Objects: objs,
		}
	default:
		rv := reflect.ValueOf(v)
		switch rv.Kind() {
		case reflect.Float64:
			f := rv.Float()
			if math.IsNaN(f) || math.IsInf(f, 0) {
				return 0.0
			}
			return f
		case reflect.Float32:
			f := rv.Float()
			if math.IsNaN(f) || math.IsInf(f, 0) {
				return float32(0.0)
			}
			return float32(f)
		case reflect.Slice:
			n := rv.Len()
			s := make([]any, n)
			for i := 0; i < n; i++ {
				s[i] = sanitizeNonFiniteFloats(rv.Index(i).Interface())
			}
			return s
		case reflect.Map:
			m := make(map[string]any, rv.Len())
			for _, key := range rv.MapKeys() {
				kStr := fmt.Sprintf("%v", key.Interface())
				m[kStr] = sanitizeNonFiniteFloats(rv.MapIndex(key).Interface())
			}
			return m
		default:
			return v
		}
	}
}

// WriteCompressedSnapshot writes a compressed snapshot to a file
func WriteCompressedSnapshot(cs *CompressedSnapshot, filePath string) error {
	checksum, err := CalculateChecksum(cs)
	if err != nil {
		return errfmt.Newf(ConstMiscFailedToCalculateChecksum).Wrap(err)
	}
	cs.Header.Checksum = checksum

	data, err := yaml.Marshal(cs)
	if err != nil {
		return errfmt.Newf(ConstMiscFailedToMarshalCompressedSnapshot).Wrap(err)
	}

	if err := fileutil.MkdirAll(filepath.Dir(filePath), paths.DirPerm755); err != nil {
		return errfmt.Newf(ErrMsgCreateDir).Wrap(err)
	}

	return fileutil.WriteFile(filePath, data, paths.FilePerm644)
}

// ReadCompressedSnapshot reads a compressed snapshot from a file
func ReadCompressedSnapshot(filePath string) (*CompressedSnapshot, error) {
	data, err := fileutil.ReadFile(filePath)
	if err != nil {
		return nil, errfmt.Newf(ConstMiscFailedToReadCompressedSnapshot).Wrap(err)
	}

	var cs CompressedSnapshot
	if err := yaml.Unmarshal(data, &cs); err != nil {
		return nil, errfmt.Newf(ConstMiscFailedToUnmarshalCompressedSnapshot).Wrap(err)
	}

	expectedChecksum := cs.Header.Checksum
	actualChecksum, err := CalculateChecksum(&cs)
	if err != nil {
		return nil, errfmt.Newf(ConstMiscFailedToCalculateChecksum).Wrap(err)
	}

	if expectedChecksum != actualChecksum {
		return nil, errfmt.Errorf(ConstMiscChecksumMismatchExpectedSGotS, expectedChecksum, actualChecksum)
	}

	return &cs, nil
}

// CreateCompressedSnapshot creates a compressed snapshot from objects
