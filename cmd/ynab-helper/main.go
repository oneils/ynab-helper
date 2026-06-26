package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jessevdk/go-flags"

	"github.com/oneils/ynab-helper/internal/app"
)

var revision = "unknown"

func main() {
	// Setup structured logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("starting YNAB Helper", "version", revision)

	var cfg app.Config
	if _, err := flags.Parse(&cfg); err != nil {
		slog.Error("failed to parse flags", "error", err)
		os.Exit(1)
	}

	// Log config (mask sensitive data)
	maskedToken := "****"
	if len(cfg.YnabToken) > 4 {
		maskedToken = cfg.YnabToken[:4] + "***"
	}
	slog.Info("configuration loaded", "ynab_token_prefix", maskedToken, "addr", cfg.Addr)

	// Create application
	application, err := app.New(cfg)
	if err != nil {
		slog.Error("failed to create application", "error", err)
		os.Exit(1)
	}

	// Setup signal handling
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// Run application
	slog.Info("starting HTTP server", "addr", cfg.Addr)
	if err := application.Run(ctx); err != nil {
		slog.Error("application failed", "error", err)
		os.Exit(1)
	}

	// Cleanup
	if err := application.Close(context.Background()); err != nil {
		slog.Error("failed to close application", "error", err)
	}

	slog.Info("YNAB Helper stopped")
}
