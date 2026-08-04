package collectorservice

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"tisminSRETool/internal/collector"
	"tisminSRETool/internal/engine"
	"tisminSRETool/internal/exporter"
	"tisminSRETool/internal/model"
)

type Service struct {
	config          *model.Config
	logger          *log.Logger
	collector       collector.Collector
	shutdownTimeout time.Duration
}

type Option func(*Service)

func WithCollector(c collector.Collector) Option {
	return func(service *Service) {
		if c != nil {
			service.collector = c
		}
	}
}

func WithShutdownTimeout(timeout time.Duration) Option {
	return func(service *Service) {
		if timeout > 0 {
			service.shutdownTimeout = timeout
		}
	}
}

func New(config *model.Config, logger *log.Logger, options ...Option) (*Service, error) {
	if config == nil {
		return nil, fmt.Errorf("collector service config is nil")
	}
	if logger == nil {
		logger = log.New(os.Stdout, "", log.LstdFlags)
	}
	service := &Service{
		config:          config,
		logger:          logger,
		collector:       &collector.LinuxCollector{},
		shutdownTimeout: 10 * time.Second,
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service, nil
}

func (s *Service) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	runner := engine.NewRunner(s.collector, s.config.App.RefreshInterval, s.logger)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		runner.Run(runCtx)
	}()

	var metricsHandler http.Handler
	if s.config.Prometheus.Enabled {
		promExporter := exporter.NewPrometheusExporter(runner)
		metricsHandler = promExporter.Handler()
		wg.Add(1)
		go func() {
			defer wg.Done()
			promExporter.StartMetricsCollector(runCtx, s.config.App.RefreshInterval)
		}()
	}

	serviceErrCh := make(chan error, 1)
	if s.config.HTTP.Listen != "" {
		httpServer := exporter.NewHTTPServer(s.config.HTTP, s.config.Prometheus.Path, runner, metricsHandler)
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.logger.Printf("collector HTTP server listening on %s", s.config.HTTP.Listen)
			if err := httpServer.Start(runCtx); err != nil {
				select {
				case serviceErrCh <- fmt.Errorf("HTTP server: %w", err):
				default:
				}
			}
		}()
	}

	var runErr error
	select {
	case <-ctx.Done():
		s.logger.Printf("context canceled: %v", ctx.Err())
	case runErr = <-serviceErrCh:
		s.logger.Printf("collector service component failed: %v", runErr)
	}

	s.logger.Println("collector service shutting down...")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer shutdownCancel()

	componentsDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(componentsDone)
	}()

	select {
	case <-componentsDone:
		s.logger.Println("collector service components stopped")
	case <-shutdownCtx.Done():
		if runErr != nil {
			return fmt.Errorf("%v; collector shutdown timeout", runErr)
		}
		return fmt.Errorf("collector shutdown timeout")
	}

	s.logger.Println("collector service stopped")
	return runErr
}
