package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	// 替换为你项目的真实包路径
	"tisminSRETool/internal/alert"
	"tisminSRETool/internal/model"
)

// ====================== 全局变量（对应你的Flags） ======================
var (
	thresholdJSON string   // --threshold/-t
	jsonOutput    bool     // --json
	verbose       bool     // --verbose/-v
	portList      []int    // port命令 -p
	procList      []string // proc命令 -n
)

// ====================== 根命令：tisminSRETool inspect ======================
var inspectCmd = &cobra.Command{
	Use:   "inspect",
	Short: "SRE 服务器自动化巡检工具",
	Long:  `复用Linux采集器，支持CPU/内存/磁盘/网络/进程/端口巡检，支持自定义阈值与JSON输出`,
}

func main() {
	// 根命令：tisminSRETool
	rootCmd := &cobra.Command{Use: "tisminSRETool"}
	// 注册子命令：inspect
	rootCmd.AddCommand(inspectCmd)

	// 注册巡检子命令（严格按你的设计）
	inspectCmd.AddCommand(
		NewCpuCmd(),
		NewMemCmd(),
		NewDiskCmd(),
		NewNetCmd(),
		NewProcCmd(),
		NewPortCmd(),
		NewAllCmd(),
	)

	// 执行命令
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "执行失败: %v\n", err)
		os.Exit(1)
	}
}

// ====================== 初始化全局Flag（所有子命令通用） ======================
func init() {
	// 全局Flag：threshold/json/verbose
	inspectCmd.PersistentFlags().StringVarP(&thresholdJSON, "threshold", "t", "", "JSON格式覆写默认阈值")
	inspectCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "以JSON格式输出巡检结果")
	inspectCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "开启详细输出模式")

	// Port命令专用Flag：-p 端口
	portCmd := NewPortCmd()
	portCmd.Flags().IntSliceVarP(&portList, "port", "p", []int{}, "指定监听端口，可重复：-p 80 -p 443")
	_ = portCmd.MarkFlagRequired("port")

	// Proc命令专用Flag：-n 进程名
	procCmd := NewProcCmd()
	procCmd.Flags().StringSliceVarP(&procList, "name", "n", []string{}, "指定进程名，可重复：-n nginx -n docker")
	_ = procCmd.MarkFlagRequired("name")
}

// ====================== 核心通用函数：复用Collector + AlertChecker ======================
func runInspect(ctx context.Context, collectFunc func() (*model.Metrics, *model.CollectErrors)) {
	// 1. 复用采集器：采集指标
	metrics, errs := collectFunc()
	if errs != nil && verbose {
		fmt.Printf("采集警告: %+v\n", errs)
	}

	// 2. 复用告警检查器：阈值判断
	checker := alert.NewRuleChecker()
	// 覆写自定义阈值
	if thresholdJSON != "" {
		_ = checker.LoadThresholdFromJSON(thresholdJSON)
	}
	alarms := checker.Check(metrics)

	// 3. 输出格式：JSON / 彩色控制台
	if jsonOutput {
		outputJSON(metrics, alarms)
	} else {
		outputConsole(metrics, alarms)
	}
}
