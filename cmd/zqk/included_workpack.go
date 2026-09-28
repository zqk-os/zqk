//go:build !zqk_omit_workpack

package main

import (
	"github.com/zqk-os/zqk/cmd/zqk/app"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/workpack"
)

func init() {
	objects.SetPackOwnedKinds(workpack.Kinds())
	workpack.Enable()
}

func registerIncludedWorkPack() {
	workpack.Register(app.NewRootCommand())
}
