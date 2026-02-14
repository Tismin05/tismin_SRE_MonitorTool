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
	Enabled bool `mapstructure:"enabled"`
	// CPU阈值
	CPUThreshold float64 `mapstructure:"cpu_threshold"`
	// 内存阈值
	MemoryThreshold float64 `mapstructure:"memory_threshold"`
	// 硬盘阈值
	DiskThreshold      float64 `mapstructure:"disk_threshold"`
	DiskAwaitThreshold float64 `mapstructure:"disk_await_threshold"`
	DiskUtilThreshold  float64 `mapstructure:"disk_util_threshold"`
	InodesThreshold    float64 `mapstructure:"inodes_threshold"`
	// 网络阈值
	NetworkBandwidthThreshold  float64 `mapstructure:"network_bandwidth_threshold"`   // 网卡带宽使用率阈值（百分比）
	NetworkPacketLossThreshold float64 `mapstructure:"network_packet_loss_threshold"` // 丢包率阈值（百分比）
	NetworkRTTThreshold        float64 `mapstructure:"network_rtt_threshold"`         // 网络延迟阈值（毫秒）
	TCPTimeWaitThreshold       uint64  `mapstructure:"tcp_time_wait_threshold"`       // TIME_WAIT连接数阈值
	TCPCLOSEWaitThreshold      uint64  `mapstructure:"tcp_close_wait_threshold"`      // CLOSE_WAIT连接数阈值
	TotalTCPThreshold          uint64  `mapstructure:"total_tcp_threshold"`           // 总TCP连接数阈值
}
