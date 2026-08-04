package exporter

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"

	"tisminSRETool/internal/engine"
	"tisminSRETool/internal/model"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type PrometheusExporter struct {
	runner   *engine.Runner
	registry *prometheus.Registry

	collectMu      sync.Mutex
	lastObservedAt time.Time

	// Collection state
	lastSuccessTimestamp *prometheus.GaugeVec
	lastCollectionError  *prometheus.GaugeVec
	collectionErrors     *prometheus.CounterVec

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
	netRxSpeed   *prometheus.GaugeVec
	netTxSpeed   *prometheus.GaugeVec
}

func NewPrometheusExporter(runner *engine.Runner) *PrometheusExporter {
	registry := prometheus.NewRegistry()
	registry.MustRegister(
		prometheus.NewGoCollector(),
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
	)
	factory := promauto.With(registry)
	e := &PrometheusExporter{
		runner:   runner,
		registry: registry,
	}

	// Collection state. These metrics are updated even when the data metrics
	// remain fail-closed on a partial collection.
	e.lastSuccessTimestamp = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_collector_last_success_timestamp_seconds",
		Help: "最近一次完整成功采集的 Unix 时间戳（秒）",
	}, []string{"host"})

	e.lastCollectionError = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_collector_last_collection_error",
		Help: "最近一次采集是否失败（1 表示失败，0 表示完整成功）",
	}, []string{"host"})

	e.collectionErrors = factory.NewCounterVec(prometheus.CounterOpts{
		Name: "system_collector_collection_errors_total",
		Help: "按子系统统计的采集错误总数",
	}, []string{"host", "subsystem"})

	// CPU
	e.cpuUsage = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_cpu_usage_percent",
		Help: "CPU 使用率百分比",
	}, []string{"host"})

	e.cpuCoresUsage = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_cpu_core_usage_percent",
		Help: "每个 CPU 核心的使用率百分比",
	}, []string{"host", "core"})

	e.loadAvg1 = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_load_avg_1min",
		Help: "1 分钟平均负载",
	}, []string{"host"})

	e.loadAvg5 = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_load_avg_5min",
		Help: "5 分钟平均负载",
	}, []string{"host"})

	e.loadAvg15 = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_load_avg_15min",
		Help: "15 分钟平均负载",
	}, []string{"host"})

	// Memory
	e.memTotal = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_memory_total_bytes",
		Help: "内存总量",
	}, []string{"host"})

	e.memFree = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_memory_free_bytes",
		Help: "空闲内存",
	}, []string{"host"})

	e.memAvailable = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_memory_available_bytes",
		Help: "可用内存",
	}, []string{"host"})

	e.memUsed = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_memory_used_bytes",
		Help: "已用内存",
	}, []string{"host"})

	e.memUsedPercent = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_memory_used_percent",
		Help: "内存使用率百分比",
	}, []string{"host"})

	e.swapTotal = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_swap_total_bytes",
		Help: "Swap 总量",
	}, []string{"host"})

	e.swapFree = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_swap_free_bytes",
		Help: "Swap 空闲",
	}, []string{"host"})

	e.swapUsed = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_swap_used_bytes",
		Help: "Swap 已用",
	}, []string{"host"})

	e.swapUsedPercent = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_swap_used_percent",
		Help: "Swap 使用率百分比",
	}, []string{"host"})

	// Disk
	e.diskTotal = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_disk_total_bytes",
		Help: "磁盘总容量",
	}, []string{"host", "mount"})

	e.diskFree = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_disk_free_bytes",
		Help: "磁盘空闲容量",
	}, []string{"host", "mount"})

	e.diskUsed = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_disk_used_bytes",
		Help: "磁盘已用容量",
	}, []string{"host", "mount"})

	e.diskUsedPercent = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_disk_used_percent",
		Help: "磁盘使用率百分比",
	}, []string{"host", "mount"})

	e.diskInodesTotal = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_disk_inodes_total",
		Help: "Inodes 总数",
	}, []string{"host", "mount"})

	e.diskInodesUsed = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_disk_inodes_used",
		Help: "Inodes 已用",
	}, []string{"host", "mount"})

	e.diskInodesFree = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_disk_inodes_free",
		Help: "Inodes 空闲",
	}, []string{"host", "mount"})

	e.diskInodesUsedPercent = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_disk_inodes_used_percent",
		Help: "Inodes 使用率百分比",
	}, []string{"host", "mount"})

	e.diskReadBytes = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_disk_read_bytes_total",
		Help: "磁盘读取字节总数",
	}, []string{"host", "device"})

	e.diskWriteBytes = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_disk_write_bytes_total",
		Help: "磁盘写入字节总数",
	}, []string{"host", "device"})

	e.diskAwait = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_disk_await_ms",
		Help: "磁盘平均等待时间(毫秒)",
	}, []string{"host", "device"})

	e.diskUtil = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_disk_util_percent",
		Help: "磁盘利用率百分比",
	}, []string{"host", "device"})

	// Network
	e.netRxBytes = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_network_receive_bytes_total",
		Help: "网络接收字节总数",
	}, []string{"host", "interface"})

	e.netTxBytes = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_network_transmit_bytes_total",
		Help: "网络发送字节总数",
	}, []string{"host", "interface"})

	e.netRxPackets = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_network_receive_packets_total",
		Help: "网络接收包总数",
	}, []string{"host", "interface"})

	e.netTxPackets = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_network_transmit_packets_total",
		Help: "网络发送包总数",
	}, []string{"host", "interface"})

	e.netRxErrors = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_network_receive_errors_total",
		Help: "网络接收错误总数",
	}, []string{"host", "interface"})

	e.netTxErrors = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_network_transmit_errors_total",
		Help: "网络发送错误总数",
	}, []string{"host", "interface"})

	e.netRxDropped = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_network_receive_dropped_total",
		Help: "网络接收丢包总数",
	}, []string{"host", "interface"})

	e.netTxDropped = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_network_transmit_dropped_total",
		Help: "网络发送丢包总数",
	}, []string{"host", "interface"})

	e.netRxSpeed = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_network_receive_bytes_per_second",
		Help: "网络每秒接收字节数",
	}, []string{"host", "interface"})

	e.netTxSpeed = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_network_transmit_bytes_per_second",
		Help: "网络每秒发送字节数",
	}, []string{"host", "interface"})

	return e
}

func (e *PrometheusExporter) Handler() http.Handler {
	return promhttp.HandlerFor(e.registry, promhttp.HandlerOpts{})
}

func (e *PrometheusExporter) StartMetricsCollector(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	e.collectMetrics()

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
	e.collectMu.Lock()
	defer e.collectMu.Unlock()

	metrics, errs, at := e.runner.Snapshot()
	if at.IsZero() {
		return
	}

	host := "unknown"
	if metrics != nil && metrics.Host != "" {
		host = metrics.Host
	}
	e.recordCollectionState(host, metrics != nil && (errs == nil || !errs.HasError()), errs, at)

	if metrics == nil || (errs != nil && errs.HasError()) {
		return
	}

	// CPU
	e.cpuUsage.WithLabelValues(host).Set(metrics.CPU.UsagePercent)
	e.loadAvg1.WithLabelValues(host).Set(metrics.CPU.Load1)
	e.loadAvg5.WithLabelValues(host).Set(metrics.CPU.Load5)
	e.loadAvg15.WithLabelValues(host).Set(metrics.CPU.Load15)

	e.cpuCoresUsage.DeletePartialMatch(prometheus.Labels{"host": host})
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
	e.netRxSpeed.DeletePartialMatch(prometheus.Labels{"host": host})
	e.netTxSpeed.DeletePartialMatch(prometheus.Labels{"host": host})

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
		e.netRxSpeed.WithLabelValues(host, iface).Set(net.RxSpeed)
		e.netTxSpeed.WithLabelValues(host, iface).Set(net.TxSpeed)
	}
}

func (e *PrometheusExporter) recordCollectionState(host string, success bool, errs *model.CollectErrors, at time.Time) {
	if at.Equal(e.lastObservedAt) {
		return
	}
	e.lastObservedAt = at

	if success {
		e.lastCollectionError.WithLabelValues(host).Set(0)
		e.lastSuccessTimestamp.WithLabelValues(host).Set(float64(at.UnixNano()) / float64(time.Second))
	} else {
		e.lastCollectionError.WithLabelValues(host).Set(1)
	}

	counts := []struct {
		subsystem string
		count     int
	}{
		{subsystem: "context"},
		{subsystem: "cpu"},
		{subsystem: "memory"},
		{subsystem: "disk"},
		{subsystem: "network"},
	}
	if errs != nil {
		counts[0].count = len(errs.Context)
		counts[1].count = len(errs.CPU)
		counts[2].count = len(errs.Mem)
		counts[3].count = len(errs.Disk)
		counts[4].count = len(errs.Net)
	}
	for _, item := range counts {
		e.collectionErrors.WithLabelValues(host, item.subsystem).Add(float64(item.count))
	}
}
