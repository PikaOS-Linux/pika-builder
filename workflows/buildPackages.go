package workflows

import (
	"time"

	"pkbldr/activities"
	"pkbldr/packages"

	"go.temporal.io/sdk/workflow"
)

const PACKAGE_BUILD_TASK_QUEUE = "PACKAGE_BUILD_TASK_QUEUE"

func BuildPackages(ctx workflow.Context) error {
	options := workflow.ActivityOptions{
		StartToCloseTimeout: time.Hour * 720,
	}
	ctx = workflow.WithActivityOptions(ctx, options)

	err := workflow.ExecuteActivity(ctx, activities.UpdateDockerContainer, nil).Get(ctx, nil)
	if err != nil {
		return err
	}

	packagesToBuild := make([]packages.PackageInfo, 0)
	for _, pkg := range packages.GetPackagesSlice() {
		if pkg.Status.Status == packages.Missing || pkg.Status.Status == packages.Stale {
			packagesToBuild = append(packagesToBuild, pkg)
		}
	}

	batches := batchSlice(packagesToBuild, 500)
	for _, batch := range batches {
		err := workflow.ExecuteActivity(ctx, activities.StartBuildLoop, batch).Get(ctx, nil)
		if err != nil {
			return err
		}
	}

	return nil
}

func batchSlice[T any](in []T, size int) (out [][]T) {
	out = make([][]T, 0)

	if size == 0 {
		panic("slice batch size is 0")
	}

	for i := 0; i < len(in); i = i + size {
		j := i + size
		if j > len(in) {
			j = len(in)
		}
		out = append(out, in[i:j])
	}

	return
}
