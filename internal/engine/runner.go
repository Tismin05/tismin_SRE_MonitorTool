package engine

import (
	"context"
	"log"
	"sync"
	"time"
	"tisminSRETool/internal/alert"
	"tisminSRETool/internal/collector"
	"tisminSRETool/internal/model"
)

type Runner struct {
	collector collector.Collector
	interval  time.Duration
	logger    *log.Logger
	checker   alert.AlertChecker
	sender    alert.AlertSender
	emailCfg  model.EmailConfig

	mu       sync.RWMutex
	last     *model.Metrics
	lastErrs *model.CollectErrors
	lastAt   time.Time
}

func NewRunner(c collector.Collector, interval time.Duration, logger *log.Logger) *Runner {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &Runner{
		collector: c,
		interval:  interval,
		logger:    logger,
	}
}

func (r *Runner) SetAlerting(checker alert.AlertChecker, sender alert.AlertSender, emailCfg model.EmailConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.checker = checker
	r.sender = sender
	r.emailCfg = emailCfg
}

func (r *Runner) Run(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
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
			return
		case <-ticker.C:
			r.collectOnce(ctx)
		}
	}
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
	r.last = metrics
	r.lastErrs = errs
	r.lastAt = time.Now()
	r.mu.Unlock()

	if r.logger == nil {
		return
	}

	if errs != nil && errs.HasError() {
		r.logger.Printf("collect finished with errors: %+v", errs)
		return
	}

	if metrics == nil {
		r.logger.Printf("collect finished with empty metrics")
		return
	}

	r.logger.Printf("collect finished: host=%s ts=%s", metrics.Host, metrics.UpdateTimestamp)
	r.processAlerts(parent, metrics)
}

func (r *Runner) Snapshot() (metrics *model.Metrics, errs *model.CollectErrors, at time.Time) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.last, r.lastErrs, r.lastAt
}

func (r *Runner) processAlerts(ctx context.Context, metrics *model.Metrics) {
	r.mu.RLock()
	checker := r.checker
	sender := r.sender
	emailCfg := r.emailCfg
	r.mu.RUnlock()

	if checker == nil || metrics == nil {
		return
	}

	alerts, err := checker.Check(ctx, metrics)
	if err != nil {
		if r.logger != nil {
			r.logger.Printf("alert check failed: %v", err)
		}
		return
	}
	if len(alerts) == 0 {
		return
	}

	now := time.Now()
	for i := range alerts {
		if alerts[i].Host == "" {
			alerts[i].Host = metrics.Host
		}
		if alerts[i].Timestamp.IsZero() {
			alerts[i].Timestamp = now
		}
	}

	if r.logger != nil {
		r.logger.Printf("alerts triggered: count=%d", len(alerts))
	}

	if sender == nil {
		if r.logger != nil {
			r.logger.Printf("alert sender not configured, skip sending")
		}
		return
	}

	if !isEmailConfigUsable(emailCfg) {
		if r.logger != nil {
			r.logger.Printf("email config incomplete, skip sending")
		}
		return
	}

	sendCtx, cancel := context.WithTimeout(ctx, r.interval)
	defer cancel()
	if err := sender.Send(sendCtx, alerts, emailCfg); err != nil && r.logger != nil {
		r.logger.Printf("alert send failed: %v", err)
	}
}

func isEmailConfigUsable(cfg model.EmailConfig) bool {
	return cfg.Host != "" && cfg.Port > 0 && cfg.From != "" && len(cfg.To) > 0
}
