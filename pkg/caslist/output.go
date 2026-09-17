package caslist

import (
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

func listEnvelope(objects []map[string]any) map[string]any {
	n := len(objects)
	return map[string]any{
		outKeyMeta: map[string]any{
			metaKeyIsolation: false,
			metaKeyScopeMode: namespaceScopeMode,
			metaKeyReturned:  n,
			metaKeyTotal:     n,
			metaKeyCASIndex:  true,
		},
		outKeyObjects: objects,
	}
}

func encodePayload(format string, payload any) ([]byte, error) {
	switch format {
	case formatYAML:
		return yaml.Marshal(payload)
	case formatTable:
		return nil, nil
	default:
		b, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return nil, err
		}
		return append(b, '\n'), nil
	}
}

func writeList(format string, objects []map[string]any) error {
	sort.Slice(objects, func(i, j int) bool {
		return objectString(objects[i], fieldID) < objectString(objects[j], fieldID)
	})
	if format == formatTable {
		return writeTable(objects)
	}
	b, err := encodePayload(format, listEnvelope(objects))
	if err != nil {
		return err
	}
	return writeStdout(b)
}

func writeGet(format string, obj map[string]any) error {
	if format == formatTable {
		return writeTable([]map[string]any{obj})
	}
	b, err := encodePayload(format, obj)
	if err != nil {
		return err
	}
	return writeStdout(b)
}

func writeCount(format string, kind string, objects []map[string]any, byKind map[string]int) error {
	payload := map[string]any{
		outKeyCount: len(objects),
		outKeyMeta: map[string]any{
			metaKeyIsolation: false,
			metaKeyScopeMode: namespaceScopeMode,
			metaKeyTotal:     len(objects),
			metaKeyCASIndex:  true,
		},
	}
	if kind != "" {
		payload[fieldKind] = kind
	}
	if len(byKind) > 0 {
		payload[outKeyCountsByKind] = byKind
		payload[outKeyCount] = 0
		total := 0
		for _, n := range byKind {
			total += n
		}
		payload[outKeyCount] = total
	}
	if format == formatTable {
		return writeCountTable(kind, objects, byKind)
	}
	b, err := encodePayload(format, payload)
	if err != nil {
		return err
	}
	return writeStdout(b)
}

func writeTable(objects []map[string]any) error {
	var b strings.Builder
	b.WriteString("ID\tKIND\tSTATUS\tTITLE\n")
	for _, obj := range objects {
		b.WriteString(objectString(obj, fieldID))
		b.WriteByte('\t')
		b.WriteString(objectString(obj, fieldKind))
		b.WriteByte('\t')
		b.WriteString(objectString(obj, fieldStatus))
		b.WriteByte('\t')
		b.WriteString(objectString(obj, fieldTitle))
		b.WriteByte('\n')
	}
	return writeStdout([]byte(b.String()))
}

func writeCountTable(kind string, objects []map[string]any, byKind map[string]int) error {
	var b strings.Builder
	if len(byKind) > 0 {
		kinds := make([]string, 0, len(byKind))
		for k := range byKind {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)
		b.WriteString("KIND\tCOUNT\n")
		total := 0
		for _, k := range kinds {
			n := byKind[k]
			total += n
			b.WriteString(k)
			b.WriteByte('\t')
			b.WriteString(strconv.Itoa(n))
			b.WriteByte('\n')
		}
		b.WriteString("total\t")
		b.WriteString(strconv.Itoa(total))
		b.WriteByte('\n')
		return writeStdout([]byte(b.String()))
	}
	label := kind
	if label == "" {
		label = "objects"
	}
	b.WriteString(label)
	b.WriteByte('\t')
	b.WriteString(strconv.Itoa(len(objects)))
	b.WriteByte('\n')
	return writeStdout([]byte(b.String()))
}

func objectString(obj map[string]any, key string) string {
	if obj == nil {
		return ""
	}
	v, ok := obj[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func writeStdout(b []byte) error {
	_, err := os.Stdout.Write(b)
	return err
}

func writeStderr(msg string) {
	_, _ = os.Stderr.Write([]byte(msg))
	if !strings.HasSuffix(msg, "\n") {
		_, _ = os.Stderr.Write([]byte{'\n'})
	}
}
