package contextevents

import (
	"encoding/json"

	"github.com/lanceman/zqk/pkg/objects"
)

// MarshalJSON emits object-map-aligned keys (objects.FieldKey* + wire-only constants).
func (r Record) MarshalJSON() ([]byte, error) {
	m := make(map[string]any, 16)
	m[WireTsRFC3339] = r.TsRFC3339
	m[objects.FieldKeySchemaVersion] = r.SchemaVersion
	m[objects.FieldKeyEventType] = r.EventType
	if r.Source != "" {
		m[objects.FieldKeySource] = r.Source
	}
	if r.CorrelationID != "" {
		m[WireCorrelationID] = r.CorrelationID
	}
	if r.JobID != "" {
		m[WireJobID] = r.JobID
	}
	if r.Note != "" {
		m[objects.FieldKeyNote] = r.Note
	}
	if len(r.CriteriaRefs) > 0 {
		m[objects.FieldKeyCriteriaRefs] = r.CriteriaRefs
	}
	if len(r.BacklogItemRefs) > 0 {
		m[objects.FieldKeyBacklogItemRefs] = r.BacklogItemRefs
	}
	if r.ConvergenceSessionRef != "" {
		m[objects.FieldKeyConvergenceSessionRef] = r.ConvergenceSessionRef
	}
	if len(r.Payload) > 0 {
		m[objects.FieldKeyPayload] = r.Payload
	}
	return json.Marshal(m)
}

// UnmarshalJSON accepts canonical keys plus legacy backlog/convergence keys from earlier writers.
func (r *Record) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	decodeStr := func(key string, dest *string) {
		rm, ok := raw[key]
		if !ok || len(rm) == 0 || string(rm) == "null" {
			return
		}
		_ = json.Unmarshal(rm, dest)
	}
	decodeStrSlice := func(keys ...string) *[]string {
		for _, key := range keys {
			rm, ok := raw[key]
			if !ok || len(rm) == 0 || string(rm) == "null" {
				continue
			}
			var s []string
			if err := json.Unmarshal(rm, &s); err == nil {
				return &s
			}
		}
		return nil
	}

	decodeStr(WireTsRFC3339, &r.TsRFC3339)
	decodeStr(objects.FieldKeySchemaVersion, &r.SchemaVersion)
	decodeStr(objects.FieldKeyEventType, &r.EventType)
	decodeStr(objects.FieldKeySource, &r.Source)
	decodeStr(WireCorrelationID, &r.CorrelationID)
	decodeStr(WireJobID, &r.JobID)
	decodeStr(objects.FieldKeyNote, &r.Note)
	decodeStr(objects.FieldKeyConvergenceSessionRef, &r.ConvergenceSessionRef)
	if r.ConvergenceSessionRef == "" {
		decodeStr(wireLegacyConvergenceSessionID, &r.ConvergenceSessionRef)
	}

	if xs := decodeStrSlice(objects.FieldKeyCriteriaRefs); xs != nil {
		r.CriteriaRefs = *xs
	}
	if xs := decodeStrSlice(objects.FieldKeyBacklogItemRefs, wireLegacyBacklogItemIDs); xs != nil {
		r.BacklogItemRefs = *xs
	}

	rm, ok := raw[objects.FieldKeyPayload]
	if ok && len(rm) > 0 && string(rm) != "null" {
		var pl map[string]any
		if err := json.Unmarshal(rm, &pl); err != nil {
			return err
		}
		r.Payload = pl
	}
	return nil
}
