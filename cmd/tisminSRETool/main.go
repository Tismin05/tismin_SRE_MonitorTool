package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	collectorservice "tisminSRETool/internal/service/collector"
)

var (
	configPath  = flag.String("config", "configs/config.yaml", "path to config file")
	showVersion = flag.Bool("version", false, "show version")
)

func main() {
	flag.Parse()

	if *showVersion {
		fmt.Println("tisminSRETool v1.0.0")
		return
	}

	cfg, err := collectorservice.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	logger := collectorservice.SetupLogger(cfg.App)
	service, err := collectorservice.New(cfg, logger)
	if err != nil {
		logger.Fatalf("failed to create collector service: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := service.Run(ctx); err != nil {
		logger.Fatalf("collector service failed: %v", err)
	}
}
