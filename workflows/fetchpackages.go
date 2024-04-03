package workflows

import (
	"time"

	"pkbldr/activities"

	"go.temporal.io/sdk/workflow"
)

const PACKAGE_FETCH_TASK_QUEUE = "PACKAGE_FETCH_TASK_QUEUE"

func FetchPackages(ctx workflow.Context) error {
	options := workflow.ActivityOptions{
		StartToCloseTimeout: time.Hour * 1,
	}
	ctx = workflow.WithActivityOptions(ctx, options)
	err := workflow.ExecuteActivity(ctx, activities.FetchPackages, nil).Get(ctx, nil)

	return err
}
