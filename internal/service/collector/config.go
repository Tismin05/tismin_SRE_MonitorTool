package collectorservice

import (
	"log"
	"os"
	"strings"

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
	v.SetDefault("alert.enabled", false)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok && configPath != "" {
			return nil, err
		}
		log.Printf("warning: collector config not loaded, using defaults: %v", err)
	}

	var cfg model.Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	cfg.Alert.Enabled = false
	return &cfg, nil
}

func SetupLogger(appCfg model.Appconfig) *log.Logger {
	output := os.Stdout
	var err error

	if appCfg.LogPath != "" {
		output, err = os.OpenFile(appCfg.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
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
