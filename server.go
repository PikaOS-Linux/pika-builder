package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"pkbldr/activities"
	"pkbldr/auth"
	"pkbldr/config"
	"pkbldr/packages"
	"pkbldr/starters"
	"pkbldr/workflows"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/template/html/v2"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	gowebly "github.com/gowebly/helpers"
)

// runServer runs a new HTTP server with the loaded environment variables.
func runServer(ctx context.Context) error {
	// Validate environment variables.
	port, err := strconv.Atoi(gowebly.Getenv("BACKEND_PORT", "7555"))
	if err != nil {
		slog.Error(fmt.Sprintf("invalid backend port: %d %s", port, err.Error()))
		return err
	}

	// Load configuration.
	err = config.Init()
	if err != nil {
		slog.Error("unable to load configuration: " + err.Error())
		return err
	}

	// Init session cache.
	err = auth.Init()
	if err != nil {
		slog.Error("unable to init session cache: " + err.Error())
		return err
	}

	err = packages.LoadFromDb()
	if err != nil {
		slog.Error("unable to load packages from db: " + err.Error())
		return err
	}

	c, err := client.Dial(client.Options{
		HostPort: config.Configs.TemporalUrl,
	})
	if err != nil {
		fmt.Println("unable to create Temporal client: " + err.Error())
	}
	defer c.Close()

	go startTemporalFetchWorker(c)
	go startTemporalBuildWorker(c)
	//go starters.FetchPackagesNow(c, ctx)
	go starters.ScheduleFetchPackages(c, ctx)
	//go starters.BuildPackagesNow(c, ctx)
	go starters.ScheduleBuildPackages(c, ctx)

	// Create a new server instance with options from environment variables.
	// For more information, see https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/
	config := fiber.Config{
		Views:        html.NewFileSystem(http.Dir("./templates"), ".html"),
		ViewsLayout:  "main",
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Create a new Fiber server.
	server := fiber.New(config)

	// Add Fiber middlewares.
	server.Use(logger.New())

	// Handle static files.
	server.Static("/static", "./static")

	// Handle index page view.
	server.Get("/", indexViewHandler)

	server.Get("/packages", packagesPageHandler)

	return server.Listen(fmt.Sprintf(":%d", port))
}

func startTemporalFetchWorker(c client.Client) {
	// This worker hosts both Workflow and Activity functions
	w := worker.New(c, workflows.PACKAGE_FETCH_TASK_QUEUE, worker.Options{})
	w.RegisterWorkflow(workflows.FetchPackages)
	w.RegisterActivity(activities.FetchPackages)

	// Start listening to the Task Queue
	err := w.Run(worker.InterruptCh())
	if err != nil {
		slog.Error("unable to start temporal fetch Worker: " + err.Error())
	}
}

func startTemporalBuildWorker(c client.Client) {
	// This worker hosts both Workflow and Activity functions
	w := worker.New(c, workflows.PACKAGE_BUILD_TASK_QUEUE, worker.Options{})
	w.RegisterWorkflow(workflows.BuildPackages)
	w.RegisterActivity(activities.StartBuildLoop)
	w.RegisterActivity(activities.UpdateDockerContainer)
	w.RegisterActivity(activities.FetchPackages)

	// Start listening to the Task Queue
	err := w.Run(worker.InterruptCh())
	if err != nil {
		slog.Error("unable to start temporal build Worker: " + err.Error())
	}
}
