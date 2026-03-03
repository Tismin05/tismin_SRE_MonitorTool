package exporter

import (
	"context"
	"log"
	"net/http"
	"time"
	"tisminSRETool/internal/engine"
	"tisminSRETool/internal/model"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type HTTPServer struct {
	config model.HTTPConfig
	server *http.Server
	runner *engine.Runner
}

func NewHTTPServer(config model.HTTPConfig, runner *engine.Runner) *HTTPServer {
	mux := http.NewServeMux()

	// Prometheus metrics endpoint
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		handler := promhttp.Handler()
		handler.ServeHTTP(w, r)
	})

	// Health Check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("OK")); err != nil {
			log.Printf("failed to write response: %v", err)
		}
	})

	// Status endpoint
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		metrics, errs, at := runner.Snapshot()
		w.Header().Set("Content-Type", "application/json")

		if errs != nil && errs.HasError() {
			w.WriteHeader(http.StatusServiceUnavailable)
			if _, err := w.Write([]byte(`{"status":"unavailable"}`)); err != nil {
				log.Printf("failed to write response: %v", err)
			}
			if _, err := w.Write([]byte(`{"status":"ok","last_update":"` + at.Format(time.RFC3339) + `"}`)); err != nil {
				log.Printf("failed to write response: %v", err)
			}
			return
		}

		if metrics == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if _, err := w.Write([]byte(`{"status":"ok","last_update":"` + at.Format(time.RFC3339) + `"}`)); err != nil {
			log.Printf("failed to write response: %v", err)
		}
	})

	return &HTTPServer{
		config: config,
		server: &http.Server{
			Addr:         config.Listen,
			Handler:      mux,
			ReadTimeout:  config.Timeout,
			WriteTimeout: config.Timeout,
		},
		runner: runner,
	}
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
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return s.server.Shutdown(shutdownCtx)
	}
}
