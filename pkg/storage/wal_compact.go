// Package storage: compact WAL encoding (v2) — short keys and positional payloads by kind.

package storage

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strconv"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"gopkg.in/yaml.v3"
)

const (
	walFormatVersion2 = 2
	opCreate          = "c"
	opUpdate          = "u"
	opDelete          = "d"
)

// auditEventCompactFields is the canonical field order for audit_event payloads.
// Values are stored in this order so keys are not repeated. Must match decode order.
var auditEventCompactFields = []string{
	objects.FieldKeyID, objects.FieldKeyKind, objects.FieldKeySchemaVersion, objects.FieldKeyStatus, objects.FieldKeyCreatedAt, objects.FieldKeyUpdatedAt,
	objects.FieldKeyCreatedBy, objects.FieldKeyUpdatedBy, objects.FieldKeyNamespaceID, objects.FieldKeyOriginProject, objects.FieldKeyOriginSystem,
	objects.FieldKeyEventType, objects.FieldKeyOperation, objects.FieldKeySeverity, objects.FieldKeyTitle,
	objects.FieldKeyTargetID, objects.FieldKeyTargetKind, objects.FieldKeyTargetPath,
	objects.FieldKeyMetadata, objects.FieldKeyReason, objects.FieldKeyRecoveryMethod, objects.FieldKeyNewValue, objects.FieldKeyOriginalValue,
	objects.FieldKeyAggregatedCount, objects.FieldKeyAggregationWindow, objects.FieldKeyAggregatedEvents, objects.FieldKeyPreservedSamples,
	objects.FieldKeyOccurrenceCount, objects.FieldKeyOccurrenceTimestamps,
}

// compactRecordV2 is the JSON shape for one WAL record in v2 format (short keys).
type compactRecordV2 struct {
	O string `json:"o"` // op: c | u | d
	K string `json:"k"` // kind
	I string `json:"i"` // id
	S int64  `json:"s"` // seq
	D string `json:"d,omitempty"`
}

func opToShort(op string) string {
	switch op {
	case "create":
		return opCreate
	case "update":
		return opUpdate
	case "delete":
		return opDelete
	default:
		return op
	}
}

func shortToOp(s string) string {
	switch s {
	case opCreate:
		return "create"
	case opUpdate:
		return "update"
	case opDelete:
		return "delete"
	default:
		return s
	}
}

// encodePayloadCompact returns a compact representation of the payload for the given kind.
// For audit_event uses positional values (JSON array base64); for others returns base64 YAML.
func encodePayloadCompact(kind string, yamlBytes []byte) (string, error) {
	if len(yamlBytes) == 0 {
		return "", nil
	}
	if kind != objects.KindAuditEvent {
		return base64.StdEncoding.EncodeToString(yamlBytes), nil
	}
	var m map[string]any
	if err := yaml.Unmarshal(yamlBytes, &m); err != nil {
		return "", errfmt.Newf(ConstMiscAuditEventYamlUnmarshal).Wrap(err)
	}
	values := make([]any, 0, len(auditEventCompactFields))
	for _, key := range auditEventCompactFields {
		v := m[key]
		if v == nil {
			values = append(values, "")
			continue
		}
		values = append(values, v)
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return "", errfmt.Newf(ConstMiscAuditEventValuesMarshal).Wrap(err)
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

// decodePayloadCompact decodes the compact payload for the given kind back to YAML bytes.
func decodePayloadCompact(kind string, encoded string) ([]byte, error) {
	if encoded == emptyValue {
		return nil, nil
	}
	dec, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errfmt.Newf("base64 decode").Wrap(err)
	}
	if kind != objects.KindAuditEvent {
		return dec, nil
	}
	var values []any
	if err := json.Unmarshal(dec, &values); err != nil {
		return nil, errfmt.Newf(ConstMiscAuditEventValuesUnmarshal).Wrap(err)
	}
	m := make(map[string]any)
	for i, key := range auditEventCompactFields {
		if i >= len(values) {
			break
		}
		v := values[i]
		if s, ok := v.(string); ok && s == emptyValue {
			continue
		}
		m[key] = v
	}
	out, err := yaml.Marshal(m)
	if err != nil {
		return nil, errfmt.Newf(ConstMiscAuditEventYamlMarshal).Wrap(err)
	}
	return out, nil
}

// marshalCompactRecord encodes rec as a single v2 JSON line (short keys, compact payload).
func marshalCompactRecord(rec *WALRecord) ([]byte, error) {
	d := ""
	if rec.DataB64 != emptyValue {
		yamlBytes, err := base64.StdEncoding.DecodeString(rec.DataB64)
		if err != nil {
			return nil, errfmt.Newf(ConstMiscDecodeDatab64ForCompact).Wrap(err)
		}
		d, err = encodePayloadCompact(rec.Kind, yamlBytes)
		if err != nil {
			return nil, err
		}
	}
	v2 := compactRecordV2{
		O: opToShort(rec.Op),
		K: rec.Kind,
		I: rec.ID,
		S: rec.Seq,
		D: d,
	}
	return json.Marshal(v2)
}

// marshalCompactBatch encodes recs as one v2 batch line (JSON array of compact records).
// Caller must assign Seq on each rec before calling; this only marshals.
func marshalCompactBatch(recs []*WALRecord) ([]byte, error) {
	if len(recs) == 0 {
		return nil, errfmt.Errorf(ConstMiscMarshalcompactbatchEmptySlice)
	}
	v2s := make([]compactRecordV2, 0, len(recs))
	for _, rec := range recs {
		d := ""
		if rec.DataB64 != emptyValue {
			yamlBytes, err := base64.StdEncoding.DecodeString(rec.DataB64)
			if err != nil {
				return nil, errfmt.Newf(ConstMiscDecodeDatab64ForCompact).Wrap(err)
			}
			d, err = encodePayloadCompact(rec.Kind, yamlBytes)
			if err != nil {
				return nil, err
			}
		}
		v2s = append(v2s, compactRecordV2{
			O: opToShort(rec.Op),
			K: rec.Kind,
			I: rec.ID,
			S: rec.Seq,
			D: d,
		})
	}
	return json.Marshal(v2s)
}

// getMaxSeqFromLine returns the maximum seq from a WAL line (v1 single, v2 single, or v2 batch).
// Does not decode payloads. Returns 0 if the line is invalid or has no seq.
func getMaxSeqFromLine(line []byte) int64 {
	var max int64

	findMax := func(key []byte) {
		idx := 0
		for {
			i := bytes.Index(line[idx:], key)
			if i == -1 {
				break
			}
			idx += i + len(key)
			// Skip whitespace
			for idx < len(line) && (line[idx] == ' ' || line[idx] == '\t') {
				idx++
			}
			// Parse digits
			start := idx
			for idx < len(line) && line[idx] >= '0' && line[idx] <= '9' {
				idx++
			}
			if idx > start {
				val, err := strconv.ParseInt(string(line[start:idx]), 10, 64)
				if err == nil && val > max {
					max = val
				}
			}
		}
	}

	findMax([]byte(`"s":`))
	findMax([]byte(`"seq":`))
	return max
}

// parseWALLine parses a WAL line (v1 single, v2 single, or v2 batch) into one or more WALRecords.
// For v2, D is decoded to YAML and stored in DataB64 so DecodeRecordData() works.
func parseWALLine(line []byte) ([]*WALRecord, error) {
	// v2 batch: JSON array of compact records
	var arr []json.RawMessage
	if err := json.Unmarshal(line, &arr); err == nil && len(arr) > 0 {
		out := make([]*WALRecord, 0, len(arr))
		for _, raw := range arr {
			var v2 compactRecordV2
			if err := json.Unmarshal(raw, &v2); err != nil {
				continue
			}
			rec, err := compactV2ToWALRecord(&v2)
			if err != nil {
				continue
			}
			out = append(out, rec)
		}
		if len(out) > 0 {
			return out, nil
		}
	}

	// v2 single
	var v2 compactRecordV2
	if err := json.Unmarshal(line, &v2); err == nil && v2.O != emptyValue {
		rec, err := compactV2ToWALRecord(&v2)
		if err != nil {
			return nil, err
		}
		return []*WALRecord{rec}, nil
	}

	// v1 single
	var v1 WALRecord
	if err := json.Unmarshal(line, &v1); err != nil {
		return nil, err
	}
	if v1.Op == emptyValue {
		return nil, errfmt.Errorf(ConstMiscInvalidWalLineMissingOp)
	}
	return []*WALRecord{&v1}, nil
}

func compactV2ToWALRecord(v2 *compactRecordV2) (*WALRecord, error) {
	rec := &WALRecord{
		Op:   shortToOp(v2.O),
		Kind: v2.K,
		ID:   v2.I,
		Seq:  v2.S,
	}
	if v2.D != emptyValue {
		yamlBytes, err := decodePayloadCompact(v2.K, v2.D)
		if err != nil {
			return nil, err
		}
		rec.DataB64 = base64.StdEncoding.EncodeToString(yamlBytes)
	}
	return rec, nil
}

// isCompactKind returns true if the kind uses positional (values-only) encoding in v2.
func isCompactKind(kind string) bool {
	return kind == objects.KindAuditEvent
}
