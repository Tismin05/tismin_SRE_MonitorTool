package exporter

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"tisminSRETool/internal/engine"
	"tisminSRETool/internal/model"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type HTTPServer struct {
	config      model.HTTPConfig
	metricsPath string
	server      *http.Server
	runner      *engine.Runner
}

func NewHTTPServer(config model.HTTPConfig, metricsPath string, runner *engine.Runner, metricsHandlers ...http.Handler) *HTTPServer {
	if metricsPath == "" {
		metricsPath = "/metrics"
	}
	if !strings.HasPrefix(metricsPath, "/") {
		metricsPath = "/" + metricsPath
	}
	if metricsPath == "/health" || metricsPath == "/status" {
		metricsPath = "/metrics"
	}

	mux := http.NewServeMux()

	metricsHandler := http.Handler(promhttp.Handler())
	if len(metricsHandlers) > 0 {
		metricsHandler = metricsHandlers[0]
	}
	if metricsHandler != nil {
		mux.Handle(metricsPath, metricsHandler)
	}

	// Health Check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Status endpoint
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		if runner == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "unavailable"})
			return
		}
		metrics, errs, at := runner.Snapshot()

		if errs != nil && errs.HasError() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"status":      "unavailable",
				"last_update": formatTimestamp(at),
				"errors":      collectErrorCounts(errs),
			})
			return
		}

		if metrics == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "starting"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":      "ok",
			"last_update": formatTimestamp(at),
		})
	})

	return &HTTPServer{
		config:      config,
		metricsPath: metricsPath,
		server: &http.Server{
			Addr:         config.Listen,
			Handler:      mux,
			ReadTimeout:  config.Timeout,
			WriteTimeout: config.Timeout,
		},
		runner: runner,
	}
}

func (s *HTTPServer) Handler() http.Handler {
	return s.server.Handler
}

func (s *HTTPServer) Start(ctx context.Context) error {
	errCh := make(chan error, 1)

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.server.Shutdown(shutdownCtx)
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func formatTimestamp(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	return at.Format(time.RFC3339)
}

func collectErrorCounts(errs *model.CollectErrors) map[string]int {
	return map[string]int{
		"cpu":     len(errs.CPU),
		"memory":  len(errs.Mem),
		"disk":    len(errs.Disk),
		"network": len(errs.Net),
	}
}
