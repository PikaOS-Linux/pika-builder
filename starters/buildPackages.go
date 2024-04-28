package starters

import (
	"context"
	"fmt"
	"pkbldr/workflows"
	"time"

	"go.temporal.io/sdk/client"
)

func BuildPackagesNow(c client.Client) {
	options := client.StartWorkflowOptions{
		ID:        "startup-package-build-workflow",
		TaskQueue: workflows.PACKAGE_BUILD_TASK_QUEUE,
	}

	// Start the Workflow
	_, err := c.ExecuteWorkflow(context.Background(), options, workflows.BuildPackages)
	if err != nil {
		fmt.Println("unable to complete startup package build Workflow", err)
	} else {
		fmt.Println("startup package build Workflow completed")
	}
}

func ScheduleBuildPackages(c client.Client) {
	scheduleID := "package-build-schedule"
	workflowID := "scheduled-package-build-workflow"
	_, err := c.ScheduleClient().Create(context.Background(), client.ScheduleOptions{
		ID: scheduleID,
		Spec: client.ScheduleSpec{
			Jitter: 1 * time.Minute,
			Intervals: []client.ScheduleIntervalSpec{
				{
					Every: 6 * time.Hour,
				},
			},
		},
		Action: &client.ScheduleWorkflowAction{
			ID:        workflowID,
			Workflow:  workflows.BuildPackages,
			TaskQueue: workflows.PACKAGE_BUILD_TASK_QUEUE,
		},
	})
	if err != nil {
		fmt.Println("unable to create package build schedule", err)
	} else {
		fmt.Println("package build schedule created")
	}
}
