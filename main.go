package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mail-bridge-desktop/internal/config"
	"mail-bridge-desktop/internal/logger"
	"mail-bridge-desktop/internal/smtpserver"
)

const shutdownTimeout = 10 * time.Second

func main() {
	cfg := config.Load()
	log := logger.New("daemon")
	log.Info("starting mail-bridge")

	// Create the SMTP server instance
	smtpServer := smtpserver.New(cfg)

	// Start the SMTP server
	if err := smtpServer.Start(); err != nil {
		log.Error("smtp: %v", err)
		os.Exit(1)
	}

	// Wait for SIGINT or SIGTERM (Ctrl+C)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	// Shutdown the SMTP server
	log.Info("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := smtpServer.Stop(ctx); err != nil {
		log.Error("Error while shutting downsmtp: %v", err)
	}
}
