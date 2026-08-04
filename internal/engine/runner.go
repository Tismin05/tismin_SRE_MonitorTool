package engine

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"
	"tisminSRETool/internal/collector"
	"tisminSRETool/internal/model"
)

type Runner struct {
	collector collector.Collector
	interval  time.Duration
	logger    *log.Logger

	mu       sync.RWMutex
	last     *model.Metrics
	lastErrs *model.CollectErrors
	lastAt   time.Time
	started  int32
	running  int32
	doneCh   chan struct{}
}

func NewRunner(c collector.Collector, interval time.Duration, logger *log.Logger) *Runner {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &Runner{
		collector: c,
		interval:  interval,
		logger:    logger,
		doneCh:    make(chan struct{}),
	}
}

func (r *Runner) Snapshot() (metrics *model.Metrics, errs *model.CollectErrors, at time.Time) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return cloneMetrics(r.last), cloneCollectErrors(r.lastErrs), r.lastAt
}

func (r *Runner) Run(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}

	// Runner is intentionally one-shot. A second Run call would otherwise
	// create duplicate background samplers and make Done semantics ambiguous.
	if !atomic.CompareAndSwapInt32(&r.started, 0, 1) {
		if r.logger != nil {
			r.logger.Printf("runner already started, skip")
		}
		return
	}
	atomic.StoreInt32(&r.running, 1)
	defer atomic.StoreInt32(&r.running, 0)
	defer close(r.doneCh)

	backgroundDone := make(chan struct{})
	if background, ok := r.collector.(collector.BackgroundCollector); ok {
		go func() {
			defer close(backgroundDone)
			background.RunBackground(ctx)
		}()
	} else {
		close(backgroundDone)
	}

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	r.collectOnce(ctx)

	for {
		select {
		case <-ctx.Done():
			if r.logger != nil {
				r.logger.Printf("runner stopped: %v", ctx.Err())
			}
			<-backgroundDone
			return
		case <-ticker.C:
			r.collectOnce(ctx)
		}
	}
}

// WaitDone 等待 runner 完全停止（用于优雅退出）
func (r *Runner) WaitDone() {
	<-r.doneCh
}

// Done is closed after both the collection loop and collector-owned background
// samplers have stopped.
func (r *Runner) Done() <-chan struct{} {
	return r.doneCh
}

func (r *Runner) IsRunning() bool {
	return atomic.LoadInt32(&r.running) == 1
}

func (r *Runner) collectOnce(parent context.Context) {
	if r.collector == nil {
		if r.logger != nil {
			r.logger.Printf("collect skipped: collector is nil")
		}
		return
	}

	collectCtx, cancel := context.WithTimeout(parent, r.interval)
	defer cancel()

	metrics, errs := r.collector.Collect(collectCtx)

	r.mu.Lock()
	r.last = cloneMetrics(metrics)
	r.lastErrs = cloneCollectErrors(errs)
	r.lastAt = time.Now()
	r.mu.Unlock()

	if errs != nil && errs.HasError() {
		if r.logger != nil {
			r.logger.Printf("collect finished with errors: %+v", errs)
		}
		return
	}

	if metrics == nil {
		if r.logger != nil {
			r.logger.Printf("collect finished with empty metrics")
		}
		return
	}

	if r.logger != nil {
		r.logger.Printf("collect finished: host=%s ts=%s", metrics.Host, metrics.UpdateTimestamp)
	}
}

func cloneMetrics(src *model.Metrics) *model.Metrics {
	if src == nil {
		return nil
	}
	dst := *src
	dst.CPU.PerCPUUsage = append([]float64(nil), src.CPU.PerCPUUsage...)
	dst.Disk = append([]model.DiskStat(nil), src.Disk...)
	dst.Net = append([]model.NetStat(nil), src.Net...)
	dst.Procs = append([]model.ProcStat(nil), src.Procs...)
	return &dst
}

func cloneCollectErrors(src *model.CollectErrors) *model.CollectErrors {
	if src == nil {
		return nil
	}
	return &model.CollectErrors{
		Context: append([]error(nil), src.Context...),
		CPU:     append([]error(nil), src.CPU...),
		Mem:     append([]error(nil), src.Mem...),
		Disk:    append([]error(nil), src.Disk...),
		Net:     append([]error(nil), src.Net...),
	}
}
