package collector

import (
	"context"
	"fmt"
	"log"
	"time"
	"tisminSRETool/internal/model"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

// MacOSCollector 结构体，目前可以是空的，未来可以放一些缓存变量
type MacOSCollector struct{}

// 确保编译时检查 MacOSCollector 实现了 Collector 接口
var _ Collector = &MacOSCollector{}

// Collect 是外部调用的唯一入口
func (c *MacOSCollector) Collect(ctx context.Context) (*model.Metrics, error) {
	// 初始化一个空的 Metrics 对象
	metrics := &model.Metrics{
		Host:            "localhost", // 暂时写死，后续可以用 os.Hostname()
		UpdateTimestamp: time.Now().Format(time.RFC3339),
	}

	// ----------------------------------------------------
	// 方向一：使用 gopsutil 库采集 (快速、标准)
	// ----------------------------------------------------
	err := c.collectViaLib(metrics)
	if err != nil {
		log.Printf("collectViaLib error: %s", err)
		fmt.Printf("Error collecting via lib: %v\n", err)
		// 注意：这里我们记录错误但不一定直接返回，可能想继续尝试命令采集
	}

	// ----------------------------------------------------
	// 方向二：手动执行系统命令采集 (硬核、底层)
	// ----------------------------------------------------
	// 暂时注释掉，等我们写完 collectViaLib 再来逐个攻破这个
	// c.collectViaCommand(metrics)

	return metrics, nil
}

// collectViaLib 使用 gopsutil 库填充指标
func (c *MacOSCollector) collectViaLib(m *model.Metrics) error {
	// 1. 实现 CPU 采集
	perCPUUsagePrecentList, err := cpu.Percent(0, true)
	if err != nil {
		log.Printf("cpu.Precent error: %s", err)
	}

	var totalUsage float64
	for _, perCPUUsage := range perCPUUsagePrecentList {
		totalUsage += perCPUUsage
	}
	avgUsagePrecent := totalUsage / float64(len(perCPUUsagePrecentList)) // 计算总利用率

	CPUCores, _ := cpu.Counts(true)

	Load, _ := load.Avg()

	m.CPU = model.CPUStat{
		Cores:        CPUCores,
		UsagePercent: avgUsagePrecent,
		PerCPUUsage:  perCPUUsagePrecentList,
		Load1:        Load.Load1,
		Load5:        Load.Load5,
		Load15:       Load.Load15,
	}
	// 2. 实现 内存 采集
	memory, err := mem.VirtualMemory()
	if err != nil {
		log.Printf("mem.VirtualMemory error: %s", err)
	}

	swap, err := mem.SwapMemory()
	if err != nil {
		log.Printf("mem.SwapMemory error: %s", err)
	}

	m.Mem = model.MemoryStat{
		Total:           memory.Total,
		Free:            memory.Free,
		Used:            memory.Used,
		UsedPercent:     memory.UsedPercent,
		SwapTotal:       swap.Total,
		SwapUsed:        swap.Used,
		SwapFree:        swap.Free,
		SwapUsedPercent: swap.UsedPercent,
	}
	// 3. 实现 磁盘 采集
	partitions, err := disk.Partitions(false)
	if err != nil {
		log.Printf("disk.Partitions error: %s", err)
	}

	var diskStatsList []model.DiskStat
	for _, partition := range partitions {
		diskStat, err := disk.Usage(partition.Mountpoint)
		if err != nil {
			log.Printf("disk.Usage error: %s", err)
			continue
		}

		diskIO, err := disk.IOCounters(partition.Mountpoint)
		if err != nil {
			log.Printf("disk.IOCounters error: %s", err)
			continue
		}

		d := model.DiskStat{
			MountPoint:        partition.Mountpoint,
			Total:             diskStat.Total,
			Free:              diskStat.Free,
			Used:              diskStat.Used,
			UsedPercent:       diskStat.UsedPercent,
			InodesTotal:       diskStat.InodesTotal,
			InodesUsed:        diskStat.InodesUsed,
			InodesFree:        diskStat.InodesFree,
			InodesUsedPercent: diskStat.InodesUsedPercent,
			Read:              diskIO[partition.Mountpoint].ReadBytes,
			Write:             diskIO[partition.Mountpoint].WriteBytes,
		}
		diskStatsList = append(diskStatsList, d)
	}
	m.Disk = diskStatsList

	// 4. 实现 网络 采集
	netStat, err := net.IOCounters(true)
	if err != nil {
		log.Printf("net.IOCounters error: %s", err)
	}

	var netStatsList = make([]model.NetStat, len(netStat))
	for _, netStat := range netStat {
		if netStat.Name == "" {
			continue
		}
		n := model.NetStat{
			Name:      netStat.Name,
			RxBytes:   netStat.BytesRecv,
			RxPackets: netStat.PacketsRecv,
			RxErrors:  netStat.Errin,
			RxDropped: netStat.Dropin,
			TxBytes:   netStat.BytesSent,
			TxPackets: netStat.PacketsSent,
			TxErrors:  netStat.Errout,
			TxDropped: netStat.Dropout,
		}
		netStatsList = append(netStatsList, n)
	}
	m.Net = netStatsList
	return nil
}

// collectViaCommand 使用 os/exec 执行 macOS 命令 (sysctl, vm_stat 等) 填充指标
func (c *MacOSCollector) collectViaCommand(m *model.Metrics) error {
	// 预留位置，后续实现
	return nil
}
