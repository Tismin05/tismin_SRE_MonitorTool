package collectorservice

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfigNormalizesRuntimeValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "collector.yaml")
	content := []byte(`
app:
  refresh_interval: 0s
  log_path: ""
http:
  timeout: 0s
prometheus:
  enabled: true
  path: custom-metrics
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig returned an error: %v", err)
	}
	if cfg.App.RefreshInterval != 5*time.Second {
		t.Fatalf("refresh interval = %v, want 5s", cfg.App.RefreshInterval)
	}
	if cfg.HTTP.Timeout != 30*time.Second {
		t.Fatalf("HTTP timeout = %v, want 30s", cfg.HTTP.Timeout)
	}
	if cfg.Prometheus.Path != "/custom-metrics" {
		t.Fatalf("Prometheus path = %q", cfg.Prometheus.Path)
	}
}

func TestLoadConfigRejectsReservedMetricsPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "collector.yaml")
	if err := os.WriteFile(path, []byte("prometheus:\n  path: /status\n"), 0o600); err != nil {
		t.Fatalf("write test config: %v", err)
	}
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("LoadConfig accepted a reserved Prometheus path")
	}
}
