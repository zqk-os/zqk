//go:build !zqk_omit_interfacepack

package main

import (
	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/cmd/zqk/object"
	"github.com/zqk-os/zqk/pkg/interfacepack"
	"github.com/zqk-os/zqk/pkg/objects"
)

func init() {
	objects.AddPackOwnedKinds(interfacepack.Kinds())
	objects.AddModuleSpecRoot(interfacepack.SpecDir)
	objects.AddModuleLifecycleRoot(interfacepack.LifecycleDir)
	object.AddPackKindRegistrar(func(objectCmd *cobra.Command) {
		object.RegisterKindCommandsForKinds(objectCmd, interfacepack.Kinds())
	})
}
