package collectorservice

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"tisminSRETool/internal/model"

	"github.com/spf13/viper"
)

func LoadConfig(configPath string) (*model.Config, error) {
	v := viper.New()
	if configPath != "" {
		v.SetConfigFile(configPath)
	}

	v.SetEnvPrefix("TISMIN")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	v.SetDefault("app.name", "collector-agent")
	v.SetDefault("app.version", "1.0.0")
	v.SetDefault("app.refresh_interval", "5s")
	v.SetDefault("app.loglevel", "info")
	v.SetDefault("app.log_path", "./logs/collector-agent.log")
	v.SetDefault("http.listen", ":8080")
	v.SetDefault("http.timeout", "30s")
	v.SetDefault("prometheus.enabled", true)
	v.SetDefault("prometheus.path", "/metrics")
	v.SetDefault("diagnostic.enabled", false)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok && configPath != "" {
			return nil, fmt.Errorf("read collector config: %w", err)
		}
		log.Printf("warning: collector config not loaded, using defaults: %v", err)
	}

	var cfg model.Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("decode collector config: %w", err)
	}

	if cfg.App.RefreshInterval <= 0 {
		cfg.App.RefreshInterval = 5 * time.Second
	}
	if cfg.HTTP.Timeout <= 0 {
		cfg.HTTP.Timeout = 30 * time.Second
	}
	cfg.Prometheus.Path = strings.TrimSpace(cfg.Prometheus.Path)
	if cfg.Prometheus.Path == "" {
		cfg.Prometheus.Path = "/metrics"
	} else if !strings.HasPrefix(cfg.Prometheus.Path, "/") {
		cfg.Prometheus.Path = "/" + cfg.Prometheus.Path
	}
	if cfg.Prometheus.Path == "/health" || cfg.Prometheus.Path == "/status" {
		return nil, fmt.Errorf("prometheus path %q conflicts with a reserved endpoint", cfg.Prometheus.Path)
	}

	return &cfg, nil
}

func SetupLogger(appCfg model.AppConfig) *log.Logger {
	output := os.Stdout
	var err error

	if appCfg.LogPath != "" {
		if err = os.MkdirAll(filepath.Dir(appCfg.LogPath), 0o755); err == nil {
			output, err = os.OpenFile(appCfg.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		}
		if err != nil {
			log.Printf("warning: cannot open log file %s: %v", appCfg.LogPath, err)
			output = os.Stdout
		}
	}

	logger := log.New(output, "", log.LstdFlags)
	if appCfg.LogLevel == "debug" {
		logger.SetFlags(log.LstdFlags | log.Lshortfile)
	}
	return logger
}
