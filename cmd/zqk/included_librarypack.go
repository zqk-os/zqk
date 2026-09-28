//go:build !zqk_omit_librarypack

package main

import (
	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/cmd/zqk/object"
	"github.com/zqk-os/zqk/pkg/librarypack"
	"github.com/zqk-os/zqk/pkg/objects"
)

func init() {
	objects.AddPackOwnedKinds(librarypack.Kinds())
	objects.AddModuleSpecRoot(librarypack.SpecDir)
	objects.AddModuleLifecycleRoot(librarypack.LifecycleDir)
	object.AddPackKindRegistrar(func(objectCmd *cobra.Command) {
		object.RegisterKindCommandsForKinds(objectCmd, librarypack.Kinds())
	})
}
