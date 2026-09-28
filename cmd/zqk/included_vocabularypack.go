//go:build !zqk_omit_vocabularypack

package main

import (
	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/cmd/zqk/object"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/vocabularypack"
)

func init() {
	objects.AddPackOwnedKinds(vocabularypack.Kinds())
	objects.AddModuleSpecRoot(vocabularypack.SpecDir)
	objects.AddModuleLifecycleRoot(vocabularypack.LifecycleDir)
	object.AddPackKindRegistrar(func(objectCmd *cobra.Command) {
		object.RegisterKindCommandsForKinds(objectCmd, vocabularypack.Kinds())
	})
}
