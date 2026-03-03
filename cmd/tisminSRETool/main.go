package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
	"tisminSRETool/internal/alert"
	"tisminSRETool/internal/collector"
	"tisminSRETool/internal/engine"
	"tisminSRETool/internal/exporter"
	"tisminSRETool/internal/model"

	"github.com/spf13/viper"
)

var (
	configPath  = flag.String("config", "configs/config.yaml", "path to config file")
	showVersion = flag.Bool("version", false, "show version")
)

func main() {
	flag.Parse()

	if *showVersion {
		fmt.Println("tisminSRETool v0.1.0")
		os.Exit(0)
	}

	// config 和日志加载
	cfg := loadConfig()
	logger := setupLogger(cfg.App)

	// 创建底层 Collector
	linuxCollector := &collector.LinuxCollector{}

	// 创建 Runner
	runner := engine.NewRunner(linuxCollector, cfg.App.RefreshInterval, logger)

	// 设置 Alert 层
	if cfg.Alert.Enabled {
		checker := alert.NewRuleChecker(cfg.Alert)
		sender := alert.NewEmailSender(3)
		runner.SetAlerting(checker, sender, cfg.Email)
	}

	// 启动 Runner 层
	ctx, cancel := context.WithCancel(context.Background())
	go runner.Run(ctx)

	// 启动 Prometheus Exporter
	var promExporter *exporter.PrometheusExporter
	if cfg.Prometheus.Enabled {
		promExporter = exporter.NewPrometheusExporter(runner)
		go promExporter.StartMetricsCollector(ctx, cfg.App.RefreshInterval)
	}

	// 启动 HTTP Server
	if cfg.HTTP.Listen != "" {
		httpServer := exporter.NewHTTPServer(cfg.HTTP, cfg.Prometheus.Path, runner)
		go func() {
			logger.Printf("HTTP server listening on %s", cfg.HTTP.Listen)
			if err := httpServer.Start(ctx); err != nil {
				logger.Printf("HTTP server error: %v", err)
			}
		}()
	}

	sigsCh := make(chan os.Signal, 1)
	signal.Notify(sigsCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigsCh

	logger.Println("shutting down...")
	cancel()
	time.Sleep(2 * time.Second)
	logger.Println("stopped")
}

func loadConfig() *model.Config {
	viper.SetConfigFile(*configPath)

	// 支持环境变量覆盖
	viper.SetEnvPrefix("TISMIN")
	viper.AutomaticEnv()

	// 默认值
	viper.SetDefault("app.refresh_interval", "5s")
	viper.SetDefault("http.listen", ":8080")
	viper.SetDefault("http.timeout", "30s")
	viper.SetDefault("prometheus.enabled", true)
	viper.SetDefault("prometheus.path", "/metrics")

	if err := viper.ReadInConfig(); err != nil {
		log.Printf("warning: config file not found, using defaults: %v", err)
	}

	var cfg model.Config
	if err := viper.Unmarshal(&cfg); err != nil {
		log.Fatalf("failed to unmarshal config: %v", err)
	}

	return &cfg
}

func setupLogger(appCfg model.Appconfig) *log.Logger {
	output := os.Stdout
	var err error

	if appCfg.LogPath != "" {
		output, err = os.OpenFile(appCfg.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.Printf("warning: cannot open log file: %v", err)
			output = os.Stdout
		}
	}

	logger := log.New(output, "", log.LstdFlags)

	switch appCfg.LogLevel {
	case "debug":
		logger.SetFlags(log.LstdFlags | log.Lshortfile)
	}

	return logger
}
