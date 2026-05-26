package pipeline

import (
	"context"
	"sync"
	"time"
)

// Engine is the consumer that processes metrics and triggers alerts.
type Engine struct {
	dataChan <-chan MetricData
	alerts   chan<- Alert
	config   ThresholdConfig
	wg       sync.WaitGroup
	cancel   context.CancelFunc
}

// NewEngine creates a new Engine instance.
func NewEngine(dataChan <-chan MetricData, alerts chan<- Alert, config ThresholdConfig) *Engine {
	return &Engine{
		dataChan: dataChan,
		alerts:   alerts,
		config:   config,
	}
}

// Start begins processing metrics from the channel.
// It runs in a goroutine and will stop when ctx is cancelled.
func (e *Engine) Start(ctx context.Context) {
	ctx, e.cancel = context.WithCancel(ctx)

	e.wg.Add(1)
	go e.run(ctx)

	<-ctx.Done()
	e.wg.Wait()
}

// Stop signals the engine to stop.
func (e *Engine) Stop() {
	if e.cancel != nil {
		e.cancel()
	}
}

func (e *Engine) run(ctx context.Context) {
	defer e.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-e.dataChan:
			if !ok {
				// Channel closed, normal shutdown
				return
			}
			e.process(ctx, data)
		}
	}
}

func (e *Engine) process(ctx context.Context, data MetricData) {
	alerts := e.checkThresholds(data)

	for _, alert := range alerts {
		select {
		case e.alerts <- alert:
		case <-ctx.Done():
			return
		default:
			// Alert channel full, skip
		}
	}
}

func (e *Engine) checkThresholds(data MetricData) []Alert {
	var alerts []Alert

	// CPU threshold check
	if data.CPU.UsagePercent > e.config.CPUUsagePercent {
		alerts = append(alerts, Alert{
			Level:   AlertLevelWarning,
			Message: formatCPUAlert(data, e.config),
			Time:    time.Now(),
		})
	}

	// Memory threshold check
	if data.Mem.UsedPercent > e.config.MemoryUsagePercent {
		alerts = append(alerts, Alert{
			Level:   AlertLevelCritical,
			Message: formatMemAlert(data, e.config),
			Time:    time.Now(),
		})
	}

	// TCP TIME_WAIT threshold check
	if timeWaitCount := data.TCP.States[TCPStateTimeWait]; timeWaitCount > e.config.TCPTimeWaitThreshold {
		alerts = append(alerts, Alert{
			Level:   AlertLevelWarning,
			Message: formatTCPAlert(data, e.config, timeWaitCount),
			Time:    time.Now(),
		})
	}

	return alerts
}

func formatCPUAlert(data MetricData, cfg ThresholdConfig) string {
	return "High CPU usage: " + formatFloat(data.CPU.UsagePercent) + "% (threshold: " + formatFloat(cfg.CPUUsagePercent) + "%)"
}

func formatMemAlert(data MetricData, cfg ThresholdConfig) string {
	return "High memory usage: " + formatFloat(data.Mem.UsedPercent) + "% (threshold: " + formatFloat(cfg.MemoryUsagePercent) + "%)"
}

func formatTCPAlert(data MetricData, cfg ThresholdConfig, timeWaitCount int) string {
	return "High TCP TIME_WAIT connections: " + itoa(timeWaitCount) + " (threshold: " + itoa(cfg.TCPTimeWaitThreshold) + ")"
}

func formatFloat(f float64) string {
	return string(rune(int(f*100)/100+'0')) + "." + string(rune(int(f*100)%100+'0')) + "%"
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte(i%10) + '0'
		i /= 10
	}
	return string(buf[pos:])
}