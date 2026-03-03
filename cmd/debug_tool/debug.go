package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
	"tisminSRETool/internal/alert"
	"tisminSRETool/internal/collector"
	"tisminSRETool/internal/engine"
	"tisminSRETool/internal/model"
)

func main() {
	fmt.Println("🚀 启动 tisminSRETool 常驻调试模式...")

	// 1) 初始化 Linux 采集器 + Runner
	c := &collector.LinuxCollector{}
	logger := log.New(os.Stdout, "[debug] ", log.LstdFlags|log.Lshortfile)
	r := engine.NewRunner(c, 5*time.Second, logger)
	alertCfg := model.AlertConfig{
		Enabled:                    true,
		CPUThreshold:               80,
		MemoryThreshold:            80,
		DiskThreshold:              85,
		DiskAwaitThreshold:         50,
		DiskUtilThreshold:          80,
		InodesThreshold:            80,
		NetworkPacketLossThreshold: 0.01,
	}
	checker := alert.NewRuleChecker(alertCfg)
	sender := alert.NewEmailSender(2)
	emailCfg := buildEmailConfigFromEnv()
	r.SetAlerting(checker, sender, emailCfg)

	// 2) 根上下文，接收退出信号
	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 3) 启动常驻采集
	go r.Run(rootCtx)

	// 4) 调试输出：定期打印最近一次采集状态
	printTicker := time.NewTicker(10 * time.Second)
	defer printTicker.Stop()

	for {
		select {
		case <-rootCtx.Done():
			fmt.Println("🛑 收到退出信号，调试模式停止")
			return
		case <-printTicker.C:
			metrics, collectErrs, at := r.Snapshot()
			if metrics == nil {
				fmt.Printf("⏳ 尚未拿到采集结果，snapshot_at=%s\n", at.Format(time.RFC3339))
				continue
			}
			if collectErrs != nil && collectErrs.HasError() {
				fmt.Printf("⚠️ 最近一次采集有错误，host=%s at=%s errs=%+v\n",
					metrics.Host, at.Format(time.RFC3339), collectErrs)
				continue
			}
			fmt.Printf("✅ 采集正常，host=%s ts=%s cpu=%.2f%% mem=%.2f%% disks=%d nets=%d\n",
				metrics.Host,
				metrics.UpdateTimestamp,
				metrics.CPU.UsagePercent,
				metrics.Mem.UsedPercent,
				len(metrics.Disk),
				len(metrics.Net),
			)
		}
	}
}

func buildEmailConfigFromEnv() model.EmailConfig {
	to := []string{}
	for _, addr := range strings.Split(strings.TrimSpace(os.Getenv("TISMIN_ALERT_EMAIL_TO")), ",") {
		addr = strings.TrimSpace(addr)
		if addr != "" {
			to = append(to, addr)
		}
	}

	return model.EmailConfig{
		Host:     strings.TrimSpace(os.Getenv("TISMIN_ALERT_SMTP_HOST")),
		Port:     getenvInt("TISMIN_ALERT_SMTP_PORT", 0),
		Username: strings.TrimSpace(os.Getenv("TISMIN_ALERT_SMTP_USERNAME")),
		Password: strings.TrimSpace(os.Getenv("TISMIN_ALERT_SMTP_PASSWORD")),
		From:     strings.TrimSpace(os.Getenv("TISMIN_ALERT_EMAIL_FROM")),
		To:       to,
	}
}

func getenvInt(key string, def int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return v
}
