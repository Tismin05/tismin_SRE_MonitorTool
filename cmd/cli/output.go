package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"tisminSRETool/internal/model"
)

// ====================== 输出格式化（彩色+高亮+进度条，对应你的设计） ======================
func outputConsole(metrics *model.Metrics, alarms []string) {
	fmt.Println("\n=== System Inspection Report ===")

	// CPU
	if metrics.CPU > 0 {
		fmt.Printf("\n[CPU]  Usage: %.1f%%  Load: %.2f %.2f %.2f\n", metrics.CPU, metrics.Load1, metrics.Load5, metrics.Load15)
		fmt.Printf("       Cores: %d  %s\n", metrics.Cores, renderProgress(metrics.CPU))
	}

	// Memory
	if metrics.Mem > 0 {
		fmt.Printf("\n[Memory]  Total: %.1fGi  Used: %.1fGi  Available: %.1fGi\n", metrics.MemTotal, metrics.MemUsed, metrics.MemAvail)
		fmt.Printf("          Usage: %.1f%%  %s\n", metrics.MemUsage, renderProgress(metrics.MemUsage))
	}

	// Disk
	for _, disk := range metrics.Disks {
		fmt.Printf("\n[Disk]  %s : %.1f%%  %s\n", disk.Path, disk.Usage, renderProgress(disk.Usage))
	}

	// Net
	for _, net := range metrics.Nets {
		fmt.Printf("\n[Net]  %s: RX=%.0fMiB/s  TX=%.0fMiB/s  Errors=%d  Dropped=%d\n",
			net.Iface, net.Rx, net.Tx, net.Errors, net.Dropped)
	}

	// Proc
	if len(procList) > 0 {
		fmt.Printf("\n[Procs] ")
		var procStatus []string
		for _, p := range metrics.Procs {
			procStatus = append(procStatus, fmt.Sprintf("%s: running (pid %d)", p.Name, p.Pid))
		}
		fmt.Println(strings.Join(procStatus, "\n        "))
	}

	// Port
	if len(portList) > 0 {
		fmt.Printf("\n[Ports] ")
		var portStatus []string
		for _, p := range metrics.Ports {
			portStatus = append(portStatus, fmt.Sprintf("%d: %s", p.Port, p.Status))
		}
		fmt.Println(strings.Join(portStatus, "  "))
	}

	// Alarms 告警高亮
	if len(alarms) > 0 {
		fmt.Println("\n=== Alarms ===")
		for _, a := range alarms {
			fmt.Printf("  \033[33mWARNING: %s\033[0m\n", a) // 黄色警告
		}
	} else {
		fmt.Println("\n✅ 全部指标正常")
	}

	fmt.Println("\n=== Done ===")
}

// JSON输出
func outputJSON(metrics *model.Metrics, alarms []string) {
	data := map[string]interface{}{
		"metrics": metrics,
		"alarms":  alarms,
		"time":    time.Now().Format(time.RFC3339),
	}
	bs, _ := json.MarshalIndent(data, "", "  ")
	fmt.Println(string(bs))
}

// 进度条渲染
func renderProgress(percent float64) string {
	full := int(percent / 5)
	empty := 20 - full
	return fmt.Sprintf("[%s%s] %.1f%%",
		strings.Repeat("█", full),
		strings.Repeat("░", empty),
		percent,
	)
}
