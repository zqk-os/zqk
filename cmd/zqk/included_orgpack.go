//go:build !zqk_omit_orgpack

package main

import (
	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/cmd/zqk/object"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/orgpack"
)

func init() {
	objects.AddPackOwnedKinds(orgpack.Kinds())
	objects.AddModuleSpecRoot(orgpack.SpecDir)
	objects.AddModuleLifecycleRoot(orgpack.LifecycleDir)
	object.AddPackKindRegistrar(func(objectCmd *cobra.Command) {
		object.RegisterKindCommandsForKinds(objectCmd, orgpack.Kinds())
	})
}
