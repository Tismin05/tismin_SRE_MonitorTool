package model

import "time"

type Config struct {
	App Appconfig `mapstructure:"app"`
	// Collect    CollectorConfig  `mapstructure:"collect"`
	Diagnostic DiagnosticConfig `mapstructure:"diagnostic"`
	Alert      AlertConfig      `mapstructure:"alert"`
}

type Appconfig struct {
	Name            string        `mapstructure:"name"`
	Version         string        `mapstructure:"version"`
	RefreshInterval time.Duration `mapstructure:"refresh_interval"`
	LogLevel        string        `mapstructure:"loglevel"`
	LogPath         string        `mapstructure:"log_path"`
}

type DiagnosticConfig struct {
	Enabled      bool `mapstructure:"enabled"`
	ShowTopNList int  `mapstructure:"show_top_n_list"`
}

type AlertConfig struct {
	Enabled          bool    `mapstructure:"enabled"`
	CPUThreshold     float64 `mapstructure:"cpu_threshold"`
	MemoryThreshold  float64 `mapstructure:"memory_threshold"`
	DiskThreshold    float64 `mapstructure:"disk_threshold"`
	NetworkThreshold float64 `mapstructure:"network_threshold"`
	InodesThreshold  float64 `mapstructure:"inodes_threshold"`
}
