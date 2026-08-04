package model

import "time"

type Config struct {
	App        AppConfig        `mapstructure:"app"`
	HTTP       HTTPConfig       `mapstructure:"http"`
	Prometheus PrometheusConfig `mapstructure:"prometheus"`
	Diagnostic DiagnosticConfig `mapstructure:"diagnostic"`
}
type AppConfig struct {
	Name            string        `mapstructure:"name"`
	Version         string        `mapstructure:"version"`
	RefreshInterval time.Duration `mapstructure:"refresh_interval"`
	LogLevel        string        `mapstructure:"loglevel"`
	LogPath         string        `mapstructure:"log_path"`
}

// Appconfig is kept as an alias for source compatibility with older callers.
// New code should use AppConfig.
type Appconfig = AppConfig

type DiagnosticConfig struct {
	Enabled      bool `mapstructure:"enabled"`
	ShowTopNList int  `mapstructure:"show_top_n_list"`
}

type HTTPConfig struct {
	Listen  string        `mapstructure:"listen"`
	Timeout time.Duration `mapstructure:"timeout"`
}

type PrometheusConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Path    string `mapstructure:"path"`
}
