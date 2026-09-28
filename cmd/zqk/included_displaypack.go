//go:build !zqk_omit_displaypack

package main

import (
	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/cmd/zqk/object"
	"github.com/zqk-os/zqk/pkg/displaypack"
	"github.com/zqk-os/zqk/pkg/objects"
)

func init() {
	objects.AddPackOwnedKinds(displaypack.Kinds())
	objects.AddModuleSpecRoot(displaypack.SpecDir)
	objects.AddModuleLifecycleRoot(displaypack.LifecycleDir)
	object.AddPackKindRegistrar(func(objectCmd *cobra.Command) {
		object.RegisterKindCommandsForKinds(objectCmd, displaypack.Kinds())
	})
}
