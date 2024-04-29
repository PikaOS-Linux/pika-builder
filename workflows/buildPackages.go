package workflows

import (
	"time"

	"pkbldr/activities"

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

	err = workflow.ExecuteActivity(ctx, activities.StartBuildLoop, nil).Get(ctx, nil)
	if err != nil {
		return err
	}

	return nil
}
