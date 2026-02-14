package model

// Metrics 系统核心指标
type Metrics struct {
	CPU             CPUStat     `json:"cpu"`
	Mem             MemoryStat  `json:"memory"`
	Disk            []DiskStat  `json:"disk"`
	Net             []NetStat   `json:"net"`
	Procs           []ProcStat  `json:"procs"` // 进程信息
	Host            string      `json:"host"`
	UpdateTimestamp string      `json:"update_timestamp"`
}

type CPUStat struct {
	Cores        int       `json:"cores"`         // CPU核心数
	UsagePercent float64   `json:"usage_percent"` // CPU使用率，百分制
	PerCPUUsage  []float64 `json:"per_cpu_usage"` // 单个CPU的使用率
	Load1        float64   `json:"load1"`         // 1分钟的平均负载
	Load5        float64   `json:"load5"`         // 5分钟平均负载
	Load15       float64   `json:"load15"`        // 15分钟平均负载
}

type MemoryStat struct {
	Total           uint64  `json:"total"`             // 总内存 (Bytes)
	Free            uint64  `json:"free"`              // 空闲内存 (Bytes)
	Available       uint64  `json:"available"`         // 可用内存 (Bytes)
	Used            uint64  `json:"used"`              // 已用内存 (Bytes)
	UsedPercent     float64 `json:"used_percent"`      // 内存使用率 (百分制)
	SwapTotal       uint64  `json:"swap_total"`        // Swap总大小 (Bytes)
	SwapFree        uint64  `json:"swap_free"`         // Swap空闲 (Bytes)
	SwapUsed        uint64  `json:"swap_used"`         // Swap已用 (Bytes)
	SwapUsedPercent float64 `json:"swap_used_percent"` // Swap使用率 (百分制)
}

type DiskStat struct {
	MountPoint        string  `json:"mount_point"`
	Device            string  `json:"device"`
	Total             uint64  `json:"total"`
	Used              uint64  `json:"used"`
	Free              uint64  `json:"free"`
	UsedPercent       float64 `json:"used_percent"`
	InodesTotal       uint64  `json:"inodes_total"`
	InodesUsed        uint64  `json:"inodes_used"`
	InodesFree        uint64  `json:"inodes_free"`
	InodesUsedPercent float64 `json:"inodes_used_percent"`
	Read              uint64  `json:"read"`
	Write             uint64  `json:"write"`
}

type NetStat struct {
	Name      string `json:"name"`       // 网卡名
	RxBytes   uint64 `json:"rx_bytes"`   // 累计接收字节数
	RxPackets uint64 `json:"rx_packets"` // 累计接收数据包数
	RxErrors  uint64 `json:"rx_errors"`  // 累计接收数据包错误数（SRE排查丢包关键）
	RxDropped uint64 `json:"rx_dropped"` // 累计接收数据包丢弃数（如缓冲区满）
	TxBytes   uint64 `json:"tx_bytes"`   // 累计发送字节数
	TxPackets uint64 `json:"tx_packets"` // 累计发送数据包数
	TxErrors  uint64 `json:"tx_errors"`  // 累计发送数据包错误数
	TxDropped uint64 `json:"tx_dropped"` // 累计发送数据包丢弃数
}

type ProcStat struct {
	PID  int     `json:"pid"`
	Name string  `json:"name"`
	CPU  float64 `json:"cpu"`
	Mem  float64 `json:"mem"`
}
