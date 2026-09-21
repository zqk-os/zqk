package internal

import (
	"github.com/zqk-os/zqk/pkg/stampmemo"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"gopkg.in/yaml.v3"
)

var yamlDocs stampmemo.Table[map[string]any] // keyed by spec/lifecycle path (closed kind-file set)

func loadYAMLDoc(path string) (map[string]any, error) {
	doc, err := yamlDocs.Load(path, stampmemo.Of(path), func() (map[string]any, error) {
		data, err := fileutil.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var parsed map[string]any
		if err := yaml.Unmarshal(data, &parsed); err != nil {
			return nil, err
		}
		if parsed == nil {
			parsed = map[string]any{}
		}
		return parsed, nil
	})
	if err != nil {
		return nil, err
	}
	return cloneYAMLDoc(doc), nil
}

func cloneYAMLDoc(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		switch x := v.(type) {
		case map[string]any:
			out[k] = cloneYAMLDoc(x)
		case []any:
			out[k] = cloneYAMLDocSlice(x)
		default:
			out[k] = v
		}
	}
	return out
}

func cloneYAMLDocSlice(in []any) []any {
	if in == nil {
		return nil
	}
	out := make([]any, len(in))
	for i, v := range in {
		switch x := v.(type) {
		case map[string]any:
			out[i] = cloneYAMLDoc(x)
		case []any:
			out[i] = cloneYAMLDocSlice(x)
		default:
			out[i] = v
		}
	}
	return out
}
