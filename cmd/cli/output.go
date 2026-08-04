package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"tisminSRETool/internal/model"
)

// ====================== 输出格式化（彩色+高亮+进度条，对应你的设计） ======================
func outputConsole(metrics *model.Metrics, alarms interface{}) {
	if metrics == nil {
		fmt.Println("No metrics data available")
		return
	}

	fmt.Println("\n=== System Inspection Report ===")

	// CPU
	fmt.Printf("\n[CPU]  Usage: %.1f%%  Load: %.2f %.2f %.2f\n",
		metrics.CPU.UsagePercent, metrics.CPU.Load1, metrics.CPU.Load5, metrics.CPU.Load15)
	fmt.Printf("       Cores: %d  %s\n", metrics.CPU.Cores, renderProgress(metrics.CPU.UsagePercent))

	// Memory
	fmt.Printf("\n[Memory]  Total: %.1fGiB  Used: %.1fGiB  Available: %.1fGiB\n",
		float64(metrics.Mem.Total)/1e9, float64(metrics.Mem.Used)/1e9, float64(metrics.Mem.Available)/1e9)
	fmt.Printf("          Usage: %.1f%%  %s\n", metrics.Mem.UsedPercent, renderProgress(metrics.Mem.UsedPercent))

	// Disk
	for _, disk := range metrics.Disk {
		fmt.Printf("\n[Disk]  %s : %.1f%%  %s\n", disk.MountPoint, disk.UsedPercent, renderProgress(disk.UsedPercent))
	}

	// Net
	for _, net := range metrics.Net {
		fmt.Printf("\n[Net]  %s: RX=%d  TX=%d  Errors=RX:%d TX:%d  Dropped=RX:%d TX:%d\n",
			net.Name, net.RxBytes, net.TxBytes, net.RxErrors, net.TxErrors, net.RxDropped, net.TxDropped)
	}

	// Proc
	if len(metrics.Procs) > 0 {
		fmt.Printf("\n[Procs] ")
		var procStatus []string
		for _, p := range metrics.Procs {
			procStatus = append(procStatus, fmt.Sprintf("%s: running (pid %d)", p.Name, p.PID))
		}
		fmt.Println(strings.Join(procStatus, "\n        "))
	}

	// Alarms告警高亮
	if alarms != nil && len(alarms.([]string)) > 0 {
		fmt.Println("\n=== Alarms ===")
		for _, a := range alarms.([]string) {
			fmt.Printf("  \033[33mWARNING: %s\033[0m\n", a)
		}
	} else {
		fmt.Println("\n✅ 全部指标正常")
	}

	fmt.Println("\n=== Done ===")
}

// JSON输出
func outputJSON(metrics *model.Metrics, alarms interface{}) {
	if metrics == nil {
		fmt.Println("{}")
		return
	}

	data := map[string]interface{}{
		"metrics": metrics,
		"time":    time.Now().Format(time.RFC3339),
	}
	bs, _ := json.MarshalIndent(data, "", "  ")
	fmt.Println(string(bs))
}

// 进度条渲染
func renderProgress(percent float64) string {
	full := int(percent / 5)
	if full > 20 {
		full = 20
	}
	empty := 20 - full
	return fmt.Sprintf("[%s%s] %.1f%%",
		strings.Repeat("█", full),
		strings.Repeat("░", empty),
		percent,
	)
}
