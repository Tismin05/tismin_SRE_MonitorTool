package model

// Metrics 系统核心指标
type Metrics struct {
	CPU             CPUStat    `json:"cpu"`
	Mem             MemoryStat `json:"memory"`
	Disk            []DiskStat `json:"disk"`
	Net             []NetStat  `json:"net"`
	Procs           []ProcStat `json:"procs"` // 进程信息
	Host            string     `json:"host"`
	UpdateTimestamp string     `json:"update_timestamp"`
}

type CPUStat struct {
	Cores        int       `json:"cores"`         // CPU核心数
	UsagePercent float64   `json:"usage_percent"` // CPU使用率，百分制
	PerCPUUsage  []float64 `json:"per_cpu_usage"` // 单个CPU的使用率
	Load1        float64   `json:"load1"`         // 1分钟的平均负载
	Load5        float64   `json:"load5"`         // 5分钟平均负载
	Load15       float64   `json:"load15"`        // 15分钟平均负载
	TotalTicks   uint64    `json:"total_ticks"`
	IdleTicks    uint64    `json:"idle_ticks"`
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
	ReadSectors       uint64  `json:"read_sectors"`
	ReadSpeed         float64 `json:"read_speed"`
	Write             uint64  `json:"write"`
	WriteSectors      uint64  `json:"write_sectors"`
	WriteSpeed        float64 `json:"write_speed"`
	Await             float64 `json:"await"`
	Util              float64 `json:"util"`
	IOQueueTime       uint64  `json:"io_queue_time"`
}

type NetStat struct {
	Name      string  `json:"name"`       // 网卡名
	RxBytes   uint64  `json:"rx_bytes"`   // 累计接收字节数
	RxPackets uint64  `json:"rx_packets"` // 累计接收数据包数
	RxErrors  uint64  `json:"rx_errors"`  // 累计接收数据包错误数（SRE排查丢包关键）
	RxDropped uint64  `json:"rx_dropped"` // 累计接收数据包丢弃数（如缓冲区满）
	TxBytes   uint64  `json:"tx_bytes"`   // 累计发送字节数
	TxPackets uint64  `json:"tx_packets"` // 累计发送数据包数
	TxErrors  uint64  `json:"tx_errors"`  // 累计发送数据包错误数
	TxDropped uint64  `json:"tx_dropped"` // 累计发送数据包丢弃数
	RxSpeed   float64 `json:"rx_speed"`
	TxSpeed   float64 `json:"tx_speed"`
}

type ProcStat struct {
	PID  int     `json:"pid"`
	Name string  `json:"name"`
	CPU  float64 `json:"cpu"`
	Mem  float64 `json:"mem"`
}

// ============================================
// 尝试适配 Prometheus 的 Counter 逻辑
// 新建数据结构存储相关信息
// 内存不需要累积值，可以不使用 Counter 逻辑
// ============================================

// CPURawData 原始CPU累计数据
type CPURawData struct {
	Cores     int              `json:"cores"`      // CPU核心数
	User      uint64           `json:"user"`       // 用户态时间 (jiffies)
	Nice      uint64           `json:"nice"`       // 低优先级用户态时间
	System    uint64           `json:"system"`     // 内核态时间
	Idle      uint64           `json:"idle"`       // 空闲时间
	Iowait    uint64           `json:"iowait"`     // I/O等待时间
	IRQ       uint64           `json:"irq"`        // 硬中断时间
	SoftIRQ   uint64           `json:"softirq"`    // 软中断时间
	Steal     uint64           `json:"steal"`      // 虚拟化偷取时间
	Guest     uint64           `json:"guest"`      // Guest时间
	GuestNice uint64           `json:"guest_nice"` // Guest Nice时间
	PerCPU    []CPUCoreRawData `json:"per_cpu"`    // 每个核心的原始数据
}

// CPUCoreRawData 单个CPU核心的原始数据
type CPUCoreRawData struct {
	Index   int    `json:"index"`
	User    uint64 `json:"user"`
	Nice    uint64 `json:"nice"`
	System  uint64 `json:"system"`
	Idle    uint64 `json:"idle"`
	Iowait  uint64 `json:"iowait"`
	IRQ     uint64 `json:"irq"`
	SoftIRQ uint64 `json:"softirq"`
	Steal   uint64 `json:"steal"`
	Guest   uint64 `json:"guest"`
}

// NetRawData 原始网络累计数据
type NetRawData []NetInterfaceRaw

// NetInterfaceRaw 单个网卡的原始累计数据
type NetInterfaceRaw struct {
	Name      string `json:"name"`       // 网卡名
	RxBytes   uint64 `json:"rx_bytes"`   // 累计接收字节数
	RxPackets uint64 `json:"rx_packets"` // 累计接收包数
	RxErrors  uint64 `json:"rx_errors"`  // 累计接收错误
	RxDropped uint64 `json:"rx_dropped"` // 累计接收丢包
	TxBytes   uint64 `json:"tx_bytes"`   // 累计发送字节数
	TxPackets uint64 `json:"tx_packets"` // 累计发送包数
	TxErrors  uint64 `json:"tx_errors"`  // 累计发送错误
	TxDropped uint64 `json:"tx_dropped"` // 累计发送丢包
}

// DiskRawData 原始磁盘累计数据
type DiskRawData []DiskInterfaceRaw

// DiskInterfaceRaw 单个磁盘的原始累计数据
type DiskInterfaceRaw struct {
	Device       string `json:"device"`
	MountPoint   string `json:"mount_point"`
	ReadIOs      uint64 `json:"read_ios"`       // 累计读I/O次数
	ReadSectors  uint64 `json:"read_sectors"`   // 累计读扇区数
	WriteIOs     uint64 `json:"write_ios"`      // 累计写I/O次数
	WriteSectors uint64 `json:"write_sectors"`  // 累计写扇区数
	IOQueuesTime uint64 `json:"io_queues_time"` // I/O队列累计时间(ms)
}

// MetricsWithRaw 包含原始累计值的指标
type MetricsWithRaw struct {
	Metrics
	// 原始累计值（用于Prometheus Counter）
	CPURaw  *CPURawData `json:"cpu_raw,omitempty"`
	NetRaw  NetRawData  `json:"net_raw,omitempty"`
	DiskRaw DiskRawData `json:"disk_raw,omitempty"`
}
