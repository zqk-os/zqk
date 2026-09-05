package crud

import (
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"gopkg.in/yaml.v3"
)

func RewriteObjectIDForRawRename(data []byte, newID string) ([]byte, error) {
	var obj map[string]any
	if err := yaml.Unmarshal(data, &obj); err != nil {
		return nil, errfmt.Newf("decode object payload").Wrap(err)
	}
	if strings.TrimSpace(newID) == "" {
		return nil, errfmt.Errorf("new object ID is empty")
	}
	obj[objects.FieldKeyID] = newID
	renamedData, err := yaml.Marshal(obj)
	if err != nil {
		return nil, errfmt.Newf("encode object payload").Wrap(err)
	}
	return renamedData, nil
}
