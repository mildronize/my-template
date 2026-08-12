// Command server is the template's HTTP service entrypoint. It wires
// config, logging, the SQLite connection, and the gin router together and
// starts listening. Domain routes (todo, identity) are registered here too
// once those modules exist — this file stays thin on purpose.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/mildronize/my-template/internal/platform"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server exited with error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := platform.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	logger := platform.NewLogger()
	slog.SetDefault(logger)

	db, err := platform.OpenDB(cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	router := platform.NewRouter(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	addr := fmt.Sprintf(":%d", cfg.Port)
	if err := platform.RunServer(ctx, logger, addr, router); err != nil {
		return fmt.Errorf("running server: %w", err)
	}

	return nil
}
