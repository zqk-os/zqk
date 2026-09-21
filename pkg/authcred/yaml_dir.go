package authcred

import "github.com/zqk-os/zqk/pkg/stampmemo"

func forEachYAMLFile(dir string, fn func(fileID string, data []byte)) error {
	return stampmemo.WalkYAML(dir, func(fileID, _ string, data []byte) {
		fn(fileID, data)
	})
}
