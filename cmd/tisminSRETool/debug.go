package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
	"tisminSRETool/internal/collector"
)

func main() {
	fmt.Println("🚀 正在启动 tisminSRETool 监控采集测试...")

	// 1. 初始化采集器
	// 注意：确保你的 internal/collector/local_MacOS.go 中定义了 MacOSCollector 结构体
	c := &collector.MacOSCollector{}

	// 2. 根上下文 + 子上下文（超时）
	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	collectCtx, cancel := context.WithTimeout(rootCtx, 5*time.Second)
	defer cancel()

	// 3. 执行采集
	startTime := time.Now()
	fmt.Printf("📊 正在采集系统指标，请稍候...%s\n", startTime)
	metrics, collectErrs := c.Collect(collectCtx)
	if collectErrs != nil && collectErrs.HasError() {
		log.Printf("⚠️ 采集过程中发生错误: %v", collectErrs)
	}

	// 4. 格式化输出结果
	// 我们将对象转换为美化的 JSON 格式，这样看得最清楚
	output, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		log.Fatalf("❌ 解析结果失败: %v", err)
	}

	fmt.Println("\n✅ 采集成功！当前系统指标如下：")
	fmt.Println(string(output))

	fmt.Printf("\n⏱️ 采集完成时间: %s\n", metrics.UpdateTimestamp)
}
