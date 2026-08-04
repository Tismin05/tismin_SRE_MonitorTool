package exporter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"tisminSRETool/internal/engine"
	"tisminSRETool/internal/model"
)

type exporterTestCollector struct {
	called  chan struct{}
	metrics *model.Metrics
}

func (c *exporterTestCollector) Collect(context.Context) (*model.Metrics, *model.CollectErrors) {
	select {
	case c.called <- struct{}{}:
	default:
	}
	return c.metrics, nil
}

func startExporterTestRunner(t *testing.T) (*engine.Runner, context.CancelFunc) {
	t.Helper()
	collector := &exporterTestCollector{
		called: make(chan struct{}, 1),
		metrics: &model.Metrics{
			Host: "host-a",
			CPU:  model.CPUStat{UsagePercent: 42},
			Net:  []model.NetStat{{Name: "eth0", RxBytes: 1000, RxSpeed: 25}},
		},
	}
	runner := engine.NewRunner(collector, time.Hour, nil)
	ctx, cancel := context.WithCancel(context.Background())
	go runner.Run(ctx)
	select {
	case <-collector.called:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("runner did not collect test metrics")
	}
	return runner, cancel
}

func TestPrometheusExporterUsesIsolatedRegistry(t *testing.T) {
	runner, cancel := startExporterTestRunner(t)
	defer func() {
		cancel()
		<-runner.Done()
	}()

	first := NewPrometheusExporter(runner)
	second := NewPrometheusExporter(runner)
	first.collectMetrics()
	second.collectMetrics()

	recorder := httptest.NewRecorder()
	first.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := recorder.Body.String()
	for _, want := range []string{
		`system_cpu_usage_percent{host="host-a"} 42`,
		`system_network_receive_bytes_total{host="host-a",interface="eth0"} 1000`,
		`system_network_receive_bytes_per_second{host="host-a",interface="eth0"} 25`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics output does not contain %q", want)
		}
	}
}

func TestHTTPServerStatusAndOptionalMetrics(t *testing.T) {
	runner, cancel := startExporterTestRunner(t)
	defer func() {
		cancel()
		<-runner.Done()
	}()

	exporter := NewPrometheusExporter(runner)
	exporter.collectMetrics()
	server := NewHTTPServer(model.HTTPConfig{}, "custom-metrics", runner, exporter.Handler())

	status := httptest.NewRecorder()
	server.Handler().ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/status", nil))
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"status":"ok"`) {
		t.Fatalf("unexpected status response: code=%d body=%s", status.Code, status.Body.String())
	}

	metrics := httptest.NewRecorder()
	server.Handler().ServeHTTP(metrics, httptest.NewRequest(http.MethodGet, "/custom-metrics", nil))
	if metrics.Code != http.StatusOK || !strings.Contains(metrics.Body.String(), "system_cpu_usage_percent") {
		t.Fatalf("unexpected metrics response: code=%d", metrics.Code)
	}

	disabled := NewHTTPServer(model.HTTPConfig{}, "/metrics", runner, nil)
	notFound := httptest.NewRecorder()
	disabled.Handler().ServeHTTP(notFound, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if notFound.Code != http.StatusNotFound {
		t.Fatalf("disabled metrics endpoint returned %d, want 404", notFound.Code)
	}
}
