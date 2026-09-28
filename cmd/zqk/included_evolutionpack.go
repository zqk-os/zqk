//go:build !zqk_omit_evolutionpack

package main

import (
	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/cmd/zqk/object"
	"github.com/zqk-os/zqk/pkg/evolutionpack"
	"github.com/zqk-os/zqk/pkg/objects"
)

func init() {
	objects.AddPackOwnedKinds(evolutionpack.Kinds())
	objects.AddModuleSpecRoot(evolutionpack.SpecDir)
	objects.AddModuleLifecycleRoot(evolutionpack.LifecycleDir)
	object.AddPackKindRegistrar(func(objectCmd *cobra.Command) {
		object.RegisterKindCommandsForKinds(objectCmd, evolutionpack.Kinds())
	})
}
