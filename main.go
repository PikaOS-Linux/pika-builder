package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
)

func main() {
	// Run your server.
	ctx := context.Background()

	// trap Ctrl+C and call cancel on the context
	ctx, cancel := context.WithCancel(ctx)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	defer func() {
		signal.Stop(c)
		cancel()
	}()
	go func() {
		select {
		case <-c:
			cancel()
		case <-ctx.Done():
		}
	}()

	go func() {
		if err := runServer(ctx); err != nil {
			slog.Error("Failed to start server!", "details", err.Error())
			os.Exit(1)
		}
	}()
	<-ctx.Done()
}
