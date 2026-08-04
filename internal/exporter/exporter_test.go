package exporter

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"tisminSRETool/internal/engine"
	"tisminSRETool/internal/model"
)

type exporterTestCollector struct {
	called  chan struct{}
	metrics *model.Metrics
}

type mutableExporterCollector struct {
	mu      sync.RWMutex
	metrics *model.Metrics
	errs    *model.CollectErrors
	called  chan struct{}
}

func (c *mutableExporterCollector) Collect(context.Context) (*model.Metrics, *model.CollectErrors) {
	c.mu.RLock()
	metrics, errs := c.metrics, c.errs
	c.mu.RUnlock()
	select {
	case c.called <- struct{}{}:
	default:
	}
	return metrics, errs
}

func (c *mutableExporterCollector) set(metrics *model.Metrics, errs *model.CollectErrors) {
	c.mu.Lock()
	c.metrics = metrics
	c.errs = errs
	c.mu.Unlock()
}

func (c *exporterTestCollector) Collect(context.Context) (*model.Metrics, *model.CollectErrors) {
	select {
	case c.called <- struct{}{}:
	default:
	}
	return c.metrics, nil
}

func waitForRunnerSnapshot(t *testing.T, runner *engine.Runner, match func(*model.Metrics, *model.CollectErrors) bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		metrics, errs, _ := runner.Snapshot()
		if match(metrics, errs) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	metrics, errs, _ := runner.Snapshot()
	t.Fatalf("runner snapshot did not reach expected state: metrics=%#v errors=%#v", metrics, errs)
}

func findMetricLine(t *testing.T, body, prefix string) string {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	t.Fatalf("metrics output does not contain a line with prefix %q", prefix)
	return ""
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

func TestPrometheusExporterKeepsLastCompleteSeriesOnCollectionError(t *testing.T) {
	collector := &mutableExporterCollector{
		called: make(chan struct{}, 1),
		metrics: &model.Metrics{
			Host: "host-a",
			CPU:  model.CPUStat{UsagePercent: 42},
			Net:  []model.NetStat{{Name: "eth0", RxBytes: 1000}},
		},
	}
	runner := engine.NewRunner(collector, 20*time.Millisecond, nil)
	ctx, cancel := context.WithCancel(context.Background())
	go runner.Run(ctx)
	t.Cleanup(func() {
		cancel()
		<-runner.Done()
	})

	select {
	case <-collector.called:
	case <-time.After(time.Second):
		t.Fatal("runner did not publish initial complete snapshot")
	}
	waitForRunnerSnapshot(t, runner, func(metrics *model.Metrics, errs *model.CollectErrors) bool {
		return metrics != nil && metrics.CPU.UsagePercent == 42 && (errs == nil || !errs.HasError())
	})

	exporter := NewPrometheusExporter(runner)
	exporter.collectMetrics()
	initial := httptest.NewRecorder()
	exporter.Handler().ServeHTTP(initial, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	initialBody := initial.Body.String()
	for _, want := range []string{
		`system_collector_last_collection_error{host="host-a"} 0`,
		`system_collector_last_success_timestamp_seconds{host="host-a"}`,
	} {
		if !strings.Contains(initialBody, want) {
			t.Fatalf("initial collection state does not contain %q", want)
		}
	}
	lastSuccessPrefix := `system_collector_last_success_timestamp_seconds{host="host-a"} `
	initialLastSuccess := findMetricLine(t, initialBody, lastSuccessPrefix)

	collector.set(
		&model.Metrics{
			Host: "host-a",
			CPU:  model.CPUStat{UsagePercent: 99},
			Net:  []model.NetStat{{Name: "eth1", RxBytes: 2000}},
		},
		&model.CollectErrors{Net: []error{errors.New("partial network failure")}},
	)
	select {
	case <-collector.called:
	case <-time.After(time.Second):
		t.Fatal("runner did not collect error snapshot")
	}
	waitForRunnerSnapshot(t, runner, func(_ *model.Metrics, errs *model.CollectErrors) bool {
		return errs != nil && errs.HasError()
	})
	exporter.collectMetrics()
	// Polling the same Runner snapshot again must not increment counters twice.
	exporter.collectMetrics()

	recorder := httptest.NewRecorder()
	exporter.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := recorder.Body.String()
	if got := findMetricLine(t, body, lastSuccessPrefix); got != initialLastSuccess {
		t.Fatalf("last success timestamp changed on partial collection: before=%q after=%q", initialLastSuccess, got)
	}
	for _, want := range []string{
		`system_cpu_usage_percent{host="host-a"} 42`,
		`system_network_receive_bytes_total{host="host-a",interface="eth0"} 1000`,
		`system_collector_last_collection_error{host="host-a"} 1`,
		`system_collector_collection_errors_total{host="host-a",subsystem="network"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("last complete metric disappeared after error snapshot: missing %q", want)
		}
	}
	for _, unwanted := range []string{
		`system_cpu_usage_percent{host="host-a"} 99`,
		`system_network_receive_bytes_total{host="host-a",interface="eth1"}`,
	} {
		if strings.Contains(body, unwanted) {
			t.Errorf("partial metric was published: %q", unwanted)
		}
	}
	if strings.Contains(body, `system_collector_collection_errors_total{host="host-a",subsystem="network"} 2`) {
		t.Fatal("the same error snapshot was counted more than once")
	}

	collector.set(
		&model.Metrics{
			Host: "host-a",
			CPU:  model.CPUStat{UsagePercent: 55},
			Net:  []model.NetStat{{Name: "eth1", RxBytes: 3000}},
		},
		nil,
	)
	waitForRunnerSnapshot(t, runner, func(metrics *model.Metrics, errs *model.CollectErrors) bool {
		return metrics != nil && metrics.CPU.UsagePercent == 55 && (errs == nil || !errs.HasError())
	})
	exporter.collectMetrics()

	recovered := httptest.NewRecorder()
	exporter.Handler().ServeHTTP(recovered, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	recoveredBody := recovered.Body.String()
	if got := findMetricLine(t, recoveredBody, lastSuccessPrefix); got == initialLastSuccess {
		t.Fatalf("last success timestamp did not advance after recovery: %q", got)
	}
	for _, want := range []string{
		`system_collector_last_collection_error{host="host-a"} 0`,
		`system_cpu_usage_percent{host="host-a"} 55`,
		`system_network_receive_bytes_total{host="host-a",interface="eth1"} 3000`,
		`system_collector_collection_errors_total{host="host-a",subsystem="network"} 1`,
	} {
		if !strings.Contains(recoveredBody, want) {
			t.Errorf("recovered metrics output does not contain %q", want)
		}
	}
}

func TestPrometheusExporterRecordsSubsystemErrorsOncePerSnapshot(t *testing.T) {
	runner := engine.NewRunner(nil, time.Second, nil)
	exporter := NewPrometheusExporter(runner)
	at := time.Unix(123, 0)
	errs := &model.CollectErrors{
		Context: []error{context.Canceled},
		CPU:     []error{errors.New("cpu 1"), errors.New("cpu 2")},
		Mem:     []error{errors.New("memory")},
		Disk:    []error{errors.New("disk")},
		Net:     []error{errors.New("network")},
	}

	exporter.recordCollectionState("host-a", false, errs, at)
	exporter.recordCollectionState("host-a", false, errs, at)

	recorder := httptest.NewRecorder()
	exporter.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := recorder.Body.String()
	for _, want := range []string{
		`system_collector_last_collection_error{host="host-a"} 1`,
		`system_collector_collection_errors_total{host="host-a",subsystem="context"} 1`,
		`system_collector_collection_errors_total{host="host-a",subsystem="cpu"} 2`,
		`system_collector_collection_errors_total{host="host-a",subsystem="memory"} 1`,
		`system_collector_collection_errors_total{host="host-a",subsystem="disk"} 1`,
		`system_collector_collection_errors_total{host="host-a",subsystem="network"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("collection state does not contain %q", want)
		}
	}
}

func TestPrometheusExporterSkipsEmptyRunnerSnapshot(t *testing.T) {
	exporter := NewPrometheusExporter(engine.NewRunner(nil, time.Second, nil))
	exporter.collectMetrics()

	recorder := httptest.NewRecorder()
	exporter.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if strings.Contains(recorder.Body.String(), "system_collector_last_collection_error") {
		t.Fatal("exporter published collection state before Runner produced a snapshot")
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
