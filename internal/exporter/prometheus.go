package exporter

import (
	"context"
	"strconv"
	"sync"
	"time"
	"tisminSRETool/internal/engine"
	"tisminSRETool/internal/model"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type PrometheusExporter struct {
	runner *engine.Runner

	// CPU
	cpuUsage      *prometheus.GaugeVec
	cpuCoresUsage *prometheus.GaugeVec
	loadAvg1      *prometheus.GaugeVec
	loadAvg5      *prometheus.GaugeVec
	loadAvg15     *prometheus.GaugeVec

	// Memory
	memTotal        *prometheus.GaugeVec
	memUsed         *prometheus.GaugeVec
	memFree         *prometheus.GaugeVec
	memAvailable    *prometheus.GaugeVec
	memUsedPercent  *prometheus.GaugeVec
	swapTotal       *prometheus.GaugeVec
	swapUsed        *prometheus.GaugeVec
	swapFree        *prometheus.GaugeVec
	swapUsedPercent *prometheus.GaugeVec

	// Disk
	diskTotal             *prometheus.GaugeVec
	diskUsed              *prometheus.GaugeVec
	diskFree              *prometheus.GaugeVec
	diskUsedPercent       *prometheus.GaugeVec
	diskInodesTotal       *prometheus.GaugeVec
	diskInodesUsed        *prometheus.GaugeVec
	diskInodesFree        *prometheus.GaugeVec
	diskInodesUsedPercent *prometheus.GaugeVec
	diskReadBytes         *prometheus.GaugeVec
	diskWriteBytes        *prometheus.GaugeVec
	diskAwait             *prometheus.GaugeVec
	diskUtil              *prometheus.GaugeVec

	// Net
	netRxBytes   *prometheus.GaugeVec
	netTxBytes   *prometheus.GaugeVec
	netRxPackets *prometheus.GaugeVec
	netTxPackets *prometheus.GaugeVec
	netRxErrors  *prometheus.GaugeVec
	netTxErrors  *prometheus.GaugeVec
	netRxDropped *prometheus.GaugeVec
	netTxDropped *prometheus.GaugeVec

	// alert
	alertCount    *prometheus.GaugeVec
	lastAlertTime *prometheus.GaugeVec

	mu         sync.RWMutex
	metrics    *model.Metrics
	lastAlerts int
}

func NewPrometheusExporter(runner *engine.Runner) *PrometheusExporter {
	e := &PrometheusExporter{
		runner: runner,
	}

	// CPU
	e.cpuUsage = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_cpu_usage_percent",
		Help: "CPU 使用率百分比",
	}, []string{"host"})

	e.cpuCoresUsage = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_cpu_core_usage_percent",
		Help: "每个 CPU 核心的使用率百分比",
	}, []string{"host", "core"})

	e.loadAvg1 = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_load_avg_1min",
		Help: "1 分钟平均负载",
	}, []string{"host"})

	e.loadAvg5 = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_load_avg_5min",
		Help: "5 分钟平均负载",
	}, []string{"host"})

	e.loadAvg15 = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_load_avg_15min",
		Help: "15 分钟平均负载",
	}, []string{"host"})

	// Memory
	e.memTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_memory_total_bytes",
		Help: "内存总量",
	}, []string{"host"})

	e.memFree = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_memory_free_bytes",
		Help: "空闲内存",
	}, []string{"host"})

	e.memAvailable = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_memory_available_bytes",
		Help: "可用内存",
	}, []string{"host"})

	e.memUsed = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_memory_used_bytes",
		Help: "已用内存",
	}, []string{"host"})

	e.memUsedPercent = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_memory_used_percent",
		Help: "内存使用率百分比",
	}, []string{"host"})

	e.swapTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_swap_total_bytes",
		Help: "Swap 总量",
	}, []string{"host"})

	e.swapFree = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_swap_free_bytes",
		Help: "Swap 空闲",
	}, []string{"host"})

	e.swapUsed = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_swap_used_bytes",
		Help: "Swap 已用",
	}, []string{"host"})

	e.swapUsedPercent = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_swap_used_percent",
		Help: "Swap 使用率百分比",
	}, []string{"host"})

	// Disk
	e.diskTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_disk_total_bytes",
		Help: "磁盘总容量",
	}, []string{"host", "mount"})

	e.diskFree = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_disk_free_bytes",
		Help: "磁盘空闲容量",
	}, []string{"host", "mount"})

	e.diskUsed = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_disk_used_bytes",
		Help: "磁盘已用容量",
	}, []string{"host", "mount"})

	e.diskUsedPercent = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_disk_used_percent",
		Help: "磁盘使用率百分比",
	}, []string{"host", "mount"})

	e.diskInodesTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_disk_inodes_total",
		Help: "Inodes 总数",
	}, []string{"host", "mount"})

	e.diskInodesUsed = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_disk_inodes_used",
		Help: "Inodes 已用",
	}, []string{"host", "mount"})

	e.diskInodesFree = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_disk_inodes_free",
		Help: "Inodes 空闲",
	}, []string{"host", "mount"})

	e.diskInodesUsedPercent = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_disk_inodes_used_percent",
		Help: "Inodes 使用率百分比",
	}, []string{"host", "mount"})

	e.diskReadBytes = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_disk_read_bytes_total",
		Help: "磁盘读取字节总数",
	}, []string{"host", "device"})

	e.diskWriteBytes = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_disk_write_bytes_total",
		Help: "磁盘写入字节总数",
	}, []string{"host", "device"})

	e.diskAwait = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_disk_await_ms",
		Help: "磁盘平均等待时间(毫秒)",
	}, []string{"host", "device"})

	e.diskUtil = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_disk_util_percent",
		Help: "磁盘利用率百分比",
	}, []string{"host", "device"})

	// Network
	e.netRxBytes = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_network_receive_bytes_total",
		Help: "网络接收字节总数",
	}, []string{"host", "interface"})

	e.netTxBytes = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_network_transmit_bytes_total",
		Help: "网络发送字节总数",
	}, []string{"host", "interface"})

	e.netRxPackets = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_network_receive_packets_total",
		Help: "网络接收包总数",
	}, []string{"host", "interface"})

	e.netTxPackets = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_network_transmit_packets_total",
		Help: "网络发送包总数",
	}, []string{"host", "interface"})

	e.netRxErrors = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_network_receive_errors_total",
		Help: "网络接收错误总数",
	}, []string{"host", "interface"})

	e.netTxErrors = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_network_transmit_errors_total",
		Help: "网络发送错误总数",
	}, []string{"host", "interface"})

	e.netRxDropped = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_network_receive_dropped_total",
		Help: "网络接收丢包总数",
	}, []string{"host", "interface"})

	e.netTxDropped = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_network_transmit_dropped_total",
		Help: "网络发送丢包总数",
	}, []string{"host", "interface"})

	// Alert
	e.alertCount = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "tismin_alerts_triggered_total",
		Help: "触发的告警总数",
	}, []string{"host"})

	e.lastAlertTime = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "tismin_last_alert_timestamp",
		Help: "最后告警时间戳",
	}, []string{"host"})

	return e
}

func (e *PrometheusExporter) StartMetricsCollector(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.collectMetrics()
		}
	}
}

func (e *PrometheusExporter) collectMetrics() {
	metrics, _, _ := e.runner.Snapshot()
	if metrics == nil {
		return
	}

	e.mu.Lock()
	e.metrics = metrics
	e.mu.Unlock()

	host := metrics.Host
	if host == "" {
		host = "unknown"
	}

	// CPU
	e.cpuUsage.WithLabelValues(host).Set(metrics.CPU.UsagePercent)
	e.loadAvg1.WithLabelValues(host).Set(metrics.CPU.Load1)
	e.loadAvg5.WithLabelValues(host).Set(metrics.CPU.Load5)
	e.loadAvg15.WithLabelValues(host).Set(metrics.CPU.Load15)

	for i, coreUsage := range metrics.CPU.PerCPUUsage {
		e.cpuCoresUsage.WithLabelValues(host, strconv.Itoa(i)).Set(coreUsage)
	}

	// Memory
	e.memTotal.WithLabelValues(host).Set(float64(metrics.Mem.Total))
	e.memFree.WithLabelValues(host).Set(float64(metrics.Mem.Free))
	e.memAvailable.WithLabelValues(host).Set(float64(metrics.Mem.Available))
	e.memUsed.WithLabelValues(host).Set(float64(metrics.Mem.Used))
	e.memUsedPercent.WithLabelValues(host).Set(metrics.Mem.UsedPercent)

	e.swapTotal.WithLabelValues(host).Set(float64(metrics.Mem.SwapTotal))
	e.swapFree.WithLabelValues(host).Set(float64(metrics.Mem.SwapFree))
	e.swapUsed.WithLabelValues(host).Set(float64(metrics.Mem.SwapUsed))
	e.swapUsedPercent.WithLabelValues(host).Set(metrics.Mem.SwapUsedPercent)

	// Disk - 清理旧指标
	e.diskTotal.DeletePartialMatch(prometheus.Labels{"host": host})
	e.diskFree.DeletePartialMatch(prometheus.Labels{"host": host})
	e.diskUsed.DeletePartialMatch(prometheus.Labels{"host": host})
	e.diskUsedPercent.DeletePartialMatch(prometheus.Labels{"host": host})
	e.diskInodesTotal.DeletePartialMatch(prometheus.Labels{"host": host})
	e.diskInodesUsed.DeletePartialMatch(prometheus.Labels{"host": host})
	e.diskInodesFree.DeletePartialMatch(prometheus.Labels{"host": host})
	e.diskInodesUsedPercent.DeletePartialMatch(prometheus.Labels{"host": host})
	e.diskReadBytes.DeletePartialMatch(prometheus.Labels{"host": host})
	e.diskWriteBytes.DeletePartialMatch(prometheus.Labels{"host": host})
	e.diskAwait.DeletePartialMatch(prometheus.Labels{"host": host})
	e.diskUtil.DeletePartialMatch(prometheus.Labels{"host": host})

	for _, disk := range metrics.Disk {
		mount := disk.MountPoint
		device := disk.Device

		e.diskTotal.WithLabelValues(host, mount).Set(float64(disk.Total))
		e.diskFree.WithLabelValues(host, mount).Set(float64(disk.Free))
		e.diskUsed.WithLabelValues(host, mount).Set(float64(disk.Used))
		e.diskUsedPercent.WithLabelValues(host, mount).Set(disk.UsedPercent)

		e.diskInodesTotal.WithLabelValues(host, mount).Set(float64(disk.InodesTotal))
		e.diskInodesUsed.WithLabelValues(host, mount).Set(float64(disk.InodesUsed))
		e.diskInodesFree.WithLabelValues(host, mount).Set(float64(disk.InodesFree))
		e.diskInodesUsedPercent.WithLabelValues(host, mount).Set(disk.InodesUsedPercent)

		e.diskReadBytes.WithLabelValues(host, device).Set(float64(disk.Read))
		e.diskWriteBytes.WithLabelValues(host, device).Set(float64(disk.Write))
		e.diskAwait.WithLabelValues(host, device).Set(disk.Await)
		e.diskUtil.WithLabelValues(host, device).Set(disk.Util)
	}

	// Network - 清理旧指标
	e.netRxBytes.DeletePartialMatch(prometheus.Labels{"host": host})
	e.netTxBytes.DeletePartialMatch(prometheus.Labels{"host": host})
	e.netRxPackets.DeletePartialMatch(prometheus.Labels{"host": host})
	e.netTxPackets.DeletePartialMatch(prometheus.Labels{"host": host})
	e.netRxErrors.DeletePartialMatch(prometheus.Labels{"host": host})
	e.netTxErrors.DeletePartialMatch(prometheus.Labels{"host": host})
	e.netRxDropped.DeletePartialMatch(prometheus.Labels{"host": host})
	e.netTxDropped.DeletePartialMatch(prometheus.Labels{"host": host})

	for _, net := range metrics.Net {
		iface := net.Name

		e.netRxBytes.WithLabelValues(host, iface).Set(float64(net.RxBytes))
		e.netTxBytes.WithLabelValues(host, iface).Set(float64(net.TxBytes))
		e.netRxPackets.WithLabelValues(host, iface).Set(float64(net.RxPackets))
		e.netTxPackets.WithLabelValues(host, iface).Set(float64(net.TxPackets))
		e.netRxErrors.WithLabelValues(host, iface).Set(float64(net.RxErrors))
		e.netTxErrors.WithLabelValues(host, iface).Set(float64(net.TxErrors))
		e.netRxDropped.WithLabelValues(host, iface).Set(float64(net.RxDropped))
		e.netTxDropped.WithLabelValues(host, iface).Set(float64(net.TxDropped))
	}
}

func (e *PrometheusExporter) RecordAlert(count int) {
	e.mu.Lock()
	e.lastAlerts = count
	e.mu.Unlock()

	metrics, _, _ := e.runner.Snapshot()
	host := "unknown"
	if metrics != nil && metrics.Host != "" {
		host = metrics.Host
	}

	e.alertCount.WithLabelValues(host).Set(float64(count))
	if count > 0 {
		e.lastAlertTime.WithLabelValues(host).Set(float64(time.Now().Unix()))
	}
}
