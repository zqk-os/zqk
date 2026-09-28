//go:build !zqk_omit_workpack

package main

import (
	"github.com/zqk-os/zqk/cmd/zqk/app"
	packbldr "github.com/zqk-os/zqk/packs/work/bldr_instance_v1"
	"github.com/zqk-os/zqk/pkg/workpack"
)

func init() {
	workpack.Enable()
}

func registerIncludedWorkPack() {
	workpack.Register(app.NewRootCommand())
	_ = []any{
		packbldr.NewGoalInstanceBuilder,
		packbldr.NewRequirementInstanceBuilder,
		packbldr.NewCriteriaInstanceBuilder,
		packbldr.NewTestCaseInstanceBuilder,
	}
}
