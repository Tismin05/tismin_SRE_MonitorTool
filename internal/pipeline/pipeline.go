package pipeline

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// Pipeline manages the data flow from Collector through Engine to Renderer.
type Pipeline struct {
	ctx         context.Context
	cancel      context.CancelFunc
	collector   *Collector
	dataChan    chan MetricData
	engine      *Engine
	alertsChan  chan Alert
	renderer    Renderer
	config      Config
	wg          sync.WaitGroup
}

// Config holds pipeline configuration.
type Config struct {
	BufferSize     int
	Interval       time.Duration
	Threshold      ThresholdConfig
	Verbose        bool
}

// DefaultConfig returns the default pipeline configuration.
func DefaultConfig() Config {
	return Config{
		BufferSize: 100,
		Interval:   1 * time.Second,
		Threshold: ThresholdConfig{
			CPUUsagePercent:      80.0,
			MemoryUsagePercent:  85.0,
			TCPTimeWaitThreshold: 5000,
		},
		Verbose: false,
	}
}

// NewPipeline creates a new Pipeline instance.
func NewPipeline(cfg Config) *Pipeline {
	ctx, cancel := context.WithCancel(context.Background())

	dataChan := make(chan MetricData, cfg.BufferSize)
	alertsChan := make(chan Alert, cfg.BufferSize)

	collector := &Collector{
		dataChan: dataChan,
		interval: cfg.Interval,
	}

	engine := NewEngine(dataChan, alertsChan, cfg.Threshold)

	renderer := NewConsoleRenderer(cfg.Verbose)

	return &Pipeline{
		ctx:        ctx,
		cancel:     cancel,
		collector:  collector,
		dataChan:   dataChan,
		engine:     engine,
		alertsChan: alertsChan,
		renderer:   renderer,
		config:     cfg,
	}
}

// Run starts the pipeline and blocks until shutdown.
func (p *Pipeline) Run() error {
	sigsCh := make(chan os.Signal, 1)
	signal.Notify(sigsCh, syscall.SIGINT, syscall.SIGTERM)

	// Start collector goroutine
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.collector.Start(p.ctx)
	}()

	// Start engine goroutine
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.engine.Start(p.ctx)
	}()

	// Start renderer loop (consumes data from engine output)
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.renderLoop()
	}()

	// Wait for signal
	sig := <-sigsCh
	log.Printf("received signal: %v, initiating graceful shutdown...", sig)

	p.Shutdown()

	return nil
}

func (p *Pipeline) renderLoop() {
	for {
		select {
		case <-p.ctx.Done():
			return
		case data := <-p.dataChan:
			p.renderer.Render(data)
		case alert := <-p.alertsChan:
			// Handle alert rendering
			RenderAlerts([]Alert{alert})
		}
	}
}

// Shutdown gracefully stops the pipeline.
func (p *Pipeline) Shutdown() {
	log.Println("pipeline: shutting down...")

	// Cancel context to stop all goroutines
	p.cancel()

	// Stop collector and engine
	p.collector.Stop()
	p.engine.Stop()

	// Close channels
	close(p.dataChan)
	close(p.alertsChan)

	// Wait for all goroutines to finish with timeout
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("pipeline: all goroutines stopped gracefully")
	case <-time.After(10 * time.Second):
		log.Println("pipeline: shutdown timed out")
	}
}

// RunWithContext runs the pipeline with an external context for lifecycle control.
func (p *Pipeline) RunWithContext(ctx context.Context) error {
	p.ctx, p.cancel = context.WithCancel(ctx)

	// Start collector goroutine
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.collector.Start(p.ctx)
	}()

	// Start engine goroutine
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.engine.Start(p.ctx)
	}()

	// Start renderer loop
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.renderLoop()
	}()

	// Wait for context cancellation
	<-ctx.Done()
	fmt.Println("pipeline: context cancelled, shutting down...")

	p.Shutdown()

	return nil
}