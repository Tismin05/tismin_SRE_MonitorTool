package model

// Metrics 系统核心指标
type Metrics struct {
	CPU             CPUStat    `json:"cpu"`
	Mem             MemoryStat `json:"memory"`
	Disk            []DiskStat `json:"disk"`
	Net             []NetStat  `json:"net"`
	Host            string     `json:"host"`
	UpdateTimestamp string     `json:"update_timestamp"`
}

type CPUStat struct {
	Cores        int     `json:"cores"`         // CPU核心数
	UsagePercent float64 `json:"usage_percent"` // CPU使用率，百分制
	Load1        float64 `json:"load1"`         // 1分钟的平均负载
	Load5        float64 `json:"load5"`         // 5分钟平均负载
	Load15       float64 `json:"load15"`        // 15分钟平均负载
}

type MemoryStat struct {
	Total        uint64  `json:"total"`         // 总内存
	Free         uint64  `json:"free"`          // 空闲内存
	Used         uint64  `json:"used"`          // 使用内存
	SwapTotal    uint64  `json:"swap_total"`    // 交换
	UsagePercent float64 `json:"usage_percent"` // 使用率
}

type DiskStat struct {
	MountPoint       string  `json:"mount_point"`
	Total            uint64  `json:"total"`
	Used             uint64  `json:"used"`
	Free             uint64  `json:"free"`
	UsagePercent     float64 `json:"usage_percent"`
	InodesTotal      uint64  `json:"inodes_total"`
	InodesUsed       uint64  `json:"inodes_used"`
	InodesFree       uint64  `json:"inodes_free"`
	InodeUsedPercent float64 `json:"inode_used_percent"`
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
