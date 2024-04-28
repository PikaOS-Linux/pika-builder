package starters

import (
	"context"
	"fmt"
	"pkbldr/workflows"
	"time"

	"go.temporal.io/sdk/client"
)

func FetchPackagesNow(c client.Client) {
	options := client.StartWorkflowOptions{
		ID:        "startup-package-fetch-workflow",
		TaskQueue: workflows.PACKAGE_FETCH_TASK_QUEUE,
	}

	// Start the Workflow
	_, err := c.ExecuteWorkflow(context.Background(), options, workflows.FetchPackages)
	if err != nil {
		fmt.Println("unable to complete startup package fetch Workflow", err)
	} else {
		fmt.Println("startup package fetch Workflow completed")
	}
}

func ScheduleFetchPackages(c client.Client) {
	scheduleID := "package-fetch-schedule"
	workflowID := "scheduled-package-fetch-workflow"
	_, err := c.ScheduleClient().Create(context.Background(), client.ScheduleOptions{
		ID: scheduleID,
		Spec: client.ScheduleSpec{
			Jitter: 1 * time.Minute,
			Intervals: []client.ScheduleIntervalSpec{
				{
					Every: 1 * time.Hour,
				},
			},
		},
		Action: &client.ScheduleWorkflowAction{
			ID:        workflowID,
			Workflow:  workflows.FetchPackages,
			TaskQueue: workflows.PACKAGE_FETCH_TASK_QUEUE,
		},
	})
	if err != nil {
		fmt.Println("unable to create package fetch schedule", err)
	} else {
		fmt.Println("package fetch schedule created")
	}
}
