package cli

import (
	"context"
	"time"
	"tisminSRETool/internal/model"

	"github.com/spf13/cobra"
)

func NewCpuCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cpu",
		Short: "CPU 使用率/负载/核数巡检",
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			runInspect(ctx, func() (*model.Metrics, *model.CollectErrors) {
				return collector.NewLinuxCollector().CollectCPU(ctx)
			})
		},
	}
}

func NewMemCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mem",
		Short: "内存总量/使用率/Swap巡检",
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			runInspect(ctx, func() (*model.Metrics, *model.CollectErrors) {
				return collector.NewLinuxCollector().CollectMem(ctx)
			})
		},
	}
}

func NewDiskCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disk",
		Short: "磁盘挂载点/使用率/IO巡检",
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			runInspect(ctx, func() (*model.Metrics, *model.CollectErrors) {
				return collector.NewLinuxCollector().CollectDisk(ctx)
			})
		},
	}
}

func NewNetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "net",
		Short: "网卡流量/丢包/错误巡检",
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			runInspect(ctx, func() (*model.Metrics, *model.CollectErrors) {
				return collector.NewLinuxCollector().CollectNet(ctx)
			})
		},
	}
}

func NewProcCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "proc",
		Short: "检查指定进程是否运行",
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			runInspect(ctx, func() (*model.Metrics, *model.CollectErrors) {
				return collector.NewLinuxCollector().CollectProc(ctx, procList...)
			})
		},
	}
}

func NewPortCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "port",
		Short: "检查指定端口是否监听",
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			runInspect(ctx, func() (*model.Metrics, *model.CollectErrors) {
				return collector.NewLinuxCollector().CollectPort(ctx, portList...)
			})
		},
	}
}

func NewAllCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "all",
		Short: "执行全维度服务器巡检",
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			runInspect(ctx, func() (*model.Metrics, *model.CollectErrors) {
				// 全量采集：复用你现有的Collect()方法
				return collector.NewLinuxCollector().Collect(ctx)
			})
		},
	}
}
