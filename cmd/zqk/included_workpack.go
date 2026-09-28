//go:build !zqk_omit_workpack

package main

import (
	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/cmd/zqk/app"
	"github.com/zqk-os/zqk/cmd/zqk/object"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/workpack"
)

func init() {
	workpack.Enable()
	if !workpack.Enabled() {
		return
	}
	objects.SetPackOwnedKinds(workpack.Kinds())
	objects.AddModuleSpecRoot(workpack.SpecDir)
	objects.AddModuleLifecycleRoot(workpack.LifecycleDir)
	object.SetPackKindRegistrar(func(objectCmd *cobra.Command) {
		object.RegisterKindCommandsForKinds(objectCmd, workpack.Kinds())
	})
}

func registerIncludedWorkPack() {
	workpack.Register(app.NewRootCommand())
}
