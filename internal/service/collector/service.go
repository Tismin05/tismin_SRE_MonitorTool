package collectorservice

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tisminSRETool/internal/collector"
	"tisminSRETool/internal/engine"
	"tisminSRETool/internal/exporter"
	"tisminSRETool/internal/model"
)

type Service struct {
	config *model.Config
	logger *log.Logger
}

func New(config *model.Config, logger *log.Logger) (*Service, error) {
	if config == nil {
		return nil, fmt.Errorf("collector service config is nil")
	}
	if logger == nil {
		logger = log.New(os.Stdout, "", log.LstdFlags)
	}
	return &Service{
		config: config,
		logger: logger,
	}, nil
}

func (s *Service) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	linuxCollector := &collector.LinuxCollector{}
	runner := engine.NewRunner(linuxCollector, s.config.App.RefreshInterval, s.logger)

	go runner.Run(runCtx)

	if s.config.Prometheus.Enabled {
		promExporter := exporter.NewPrometheusExporter(runner)
		go promExporter.StartMetricsCollector(runCtx, s.config.App.RefreshInterval)
	}

	if s.config.HTTP.Listen != "" {
		httpServer := exporter.NewHTTPServer(s.config.HTTP, s.config.Prometheus.Path, runner)
		go func() {
			s.logger.Printf("collector HTTP server listening on %s", s.config.HTTP.Listen)
			if err := httpServer.Start(runCtx); err != nil {
				s.logger.Printf("collector HTTP server stopped with error: %v", err)
				cancel()
			}
		}()
	}

	sigsCh := make(chan os.Signal, 1)
	signal.Notify(sigsCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigsCh)

	select {
	case sig := <-sigsCh:
		s.logger.Printf("received signal: %v", sig)
	case <-ctx.Done():
		s.logger.Printf("context canceled: %v", ctx.Err())
	case <-runCtx.Done():
	}

	s.logger.Println("collector service shutting down...")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	runnerDone := make(chan struct{})
	go func() {
		runner.WaitDone()
		close(runnerDone)
	}()

	select {
	case <-runnerDone:
		s.logger.Println("collector runner stopped")
	case <-shutdownCtx.Done():
		return fmt.Errorf("collector shutdown timeout")
	}

	s.logger.Println("collector service stopped")
	return nil
}
