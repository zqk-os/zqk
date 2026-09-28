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
	objects.SetPackOwnedKinds(workpack.Kinds())
	objects.AddModuleSpecRoot(workpack.SpecDir)
	object.SetPackKindRegistrar(func(objectCmd *cobra.Command) {
		object.RegisterKindCommandsForKinds(objectCmd, workpack.Kinds())
	})
	workpack.Enable()
}

func registerIncludedWorkPack() {
	workpack.Register(app.NewRootCommand())
}
