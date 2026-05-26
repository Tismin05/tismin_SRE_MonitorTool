package pipeline

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Renderer defines the interface for rendering metrics.
type Renderer interface {
	Render(data MetricData)
}

// ConsoleRenderer renders metrics to the terminal with optional color support.
type ConsoleRenderer struct {
	mu      sync.Mutex
	verbose bool
}

// NewConsoleRenderer creates a new ConsoleRenderer.
func NewConsoleRenderer(verbose bool) *ConsoleRenderer {
	return &ConsoleRenderer{verbose: verbose}
}

// Render outputs the metric data to the terminal.
func (r *ConsoleRenderer) Render(data MetricData) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.renderHeader(data)
	r.renderCPU(data)
	r.renderMem(data)
	r.renderNet(data)
	r.renderTCP(data)
	r.renderSnmp(data)
}

func (r *ConsoleRenderer) renderHeader(data MetricData) {
	fmt.Printf("\n[%s] Host: %s\n", data.TS.Format(time.Stamp), data.Host)
	fmt.Println(strings.Repeat("-", 60))
}

func (r *ConsoleRenderer) renderCPU(data MetricData) {
	fmt.Printf("CPU    : %.1f%% (%d cores) | Load: %.2f / %.2f / %.2f\n",
		data.CPU.UsagePercent, data.CPU.Cores,
		data.CPU.Load1, data.CPU.Load5, data.CPU.Load15)
}

func (r *ConsoleRenderer) renderMem(data MetricData) {
	fmt.Printf("Memory : %.1f%% (%.2f GB / %.2f GB)\n",
		data.Mem.UsedPercent,
		float64(data.Mem.Used)/1e9,
		float64(data.Mem.Total)/1e9)
}

func (r *ConsoleRenderer) renderNet(data MetricData) {
	for _, net := range data.Net {
		if isRelevantInterface(net.Name) {
			fmt.Printf("Net[%s]: RX: %s TX: %s | Err: RX:%d TX:%d Drop: RX:%d TX:%d\n",
				net.Name,
				formatBytes(net.RxBytes),
				formatBytes(net.TxBytes),
				net.RxErrors, net.TxErrors,
				net.RxDropped, net.TxDropped)
		}
	}
}

func (r *ConsoleRenderer) renderTCP(data MetricData) {
	if !r.verbose && len(data.TCP.States) == 0 {
		return
	}

	fmt.Println("TCP    :")
	for state, count := range data.TCP.States {
		if count > 0 {
			fmt.Printf("  %-12s: %d\n", state.String(), count)
		}
	}
}

func (r *ConsoleRenderer) renderSnmp(data MetricData) {
	if !r.verbose {
		return
	}

	fmt.Printf("Snmp   : RetransSegs: %d\n", data.Snmp.RetransSegs)
}

// isRelevantInterface checks if the network interface should be displayed.
func isRelevantInterface(name string) bool {
	// Filter out loopback and bonding interfaces by default
	if name == "lo" || strings.HasPrefix(name, "bond") {
		return false
	}
	return true
}

// formatBytes converts bytes to human-readable format.
func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// RenderAlerts renders alerts to the terminal.
func RenderAlerts(alerts []Alert) {
	if len(alerts) == 0 {
		return
	}

	fmt.Println("\n=== ALERTS ===")
	for _, alert := range alerts {
		levelStr := "INFO"
		switch alert.Level {
		case AlertLevelWarning:
			levelStr = "WARN"
		case AlertLevelCritical:
			levelStr = "CRIT"
		}
		fmt.Printf("[%s] %s\n", levelStr, alert.Message)
	}
	fmt.Println("=============")
}