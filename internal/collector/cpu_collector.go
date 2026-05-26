package collector

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"tisminSRETool/internal/model"
	"tisminSRETool/pkg/utils"
)

// collectCPUCores 获取CPU核心数
func collectCPUCores(ctx context.Context) (cores int, err error) {
	lines, err := utils.ReadLinesOffsetNWithContext(ctx, "/proc/cpuinfo", 0, -1)
	if err != nil {
		return 0, err
	}

	cpuCores := 0
	for _, line := range lines {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		field := strings.SplitN(line, ":", 2)
		if len(field) != 2 {
			continue
		}
		key := strings.TrimSpace(field[0])
		if key == "processor" {
			cpuCores++
		}
	}
	if cpuCores == 0 {
		return 0, fmt.Errorf("no processor entries found in /proc/cpuinfo")
	}
	return cpuCores, nil
}

// CollectCPUStat 整合CPU逻辑
func CollectCPUStat(ctx context.Context) (model.CPUStat, error) {
	cores, err := collectCPUCores(ctx)
	if err != nil {
		return model.CPUStat{}, err
	}

	// 从环形缓存读取 CPU 使用率（无阻塞）
	perCPU, totalTicks, idleTicks, err := GetCPUUsageFromBuffer()
	if err != nil || len(perCPU) == 0 {
		// 如果缓存中没有数据，回退到旧的采集方式
		perCPU, totalTicks, idleTicks, err = collectCPUInfo(ctx, cores)
		if err != nil {
			return model.CPUStat{}, err
		}
	}
	if len(perCPU) > 0 {
		cores = len(perCPU)
	}

	avg := 0.0
	for _, v := range perCPU {
		avg += v
	}
	if len(perCPU) > 0 {
		avg /= float64(len(perCPU))
	}

	loads, err := collectLoadAvg(ctx)
	if err != nil {
		return model.CPUStat{}, err
	}
	if len(loads) < 3 {
		return model.CPUStat{}, fmt.Errorf("invalid loadavg length: %d", len(loads))
	}

	return model.CPUStat{
		Cores:        cores,
		UsagePercent: avg,
		PerCPUUsage:  perCPU,
		Load1:        loads[0],
		Load5:        loads[1],
		Load15:       loads[2],
		TotalTicks:   totalTicks,
		IdleTicks:    idleTicks,
	}, nil
}

const cpuSampleWindow = 200 * time.Millisecond

// collectCPUInfo 采集CPU信息
func collectCPUInfo(ctx context.Context, cores int) (perCPU []float64, totalTicks uint64, idleTicks uint64, err error) {
	_, startPerCPU, err := readCPUSnapshots(ctx, cores)
	if err != nil {
		return nil, 0, 0, err
	}

	timer := time.NewTimer(cpuSampleWindow)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, 0, 0, ctx.Err()
	case <-timer.C:
	}

	endOverall, endPerCPU, err := readCPUSnapshots(ctx, cores)
	if err != nil {
		return nil, 0, 0, err
	}
	if endOverall.total > 0 {
		totalTicks = uint64(endOverall.total)
	}
	if endOverall.idle > 0 {
		idleTicks = uint64(endOverall.idle)
	}

	n := len(startPerCPU)
	if len(endPerCPU) < n {
		n = len(endPerCPU)
	}
	if n == 0 {
		return nil, totalTicks, idleTicks, fmt.Errorf("no cpu core stats found in /proc/stat")
	}

	perCPUUsage := make([]float64, 0, n)
	for i := 0; i < n; i++ {
		diffTotal := endPerCPU[i].total - startPerCPU[i].total
		diffIdle := endPerCPU[i].idle - startPerCPU[i].idle
		if diffTotal <= 0 {
			continue
		}
		usage := (diffTotal - diffIdle) / diffTotal * 100
		if usage < 0 {
			usage = 0
		}
		if usage > 100 {
			usage = 100
		}
		perCPUUsage = append(perCPUUsage, usage)
	}
	if len(perCPUUsage) == 0 {
		return nil, totalTicks, idleTicks, fmt.Errorf("failed to calculate cpu usage from /proc/stat")
	}
	return perCPUUsage, totalTicks, idleTicks, nil
}

// readCPUSnapshots 读取CPU快照
func readCPUSnapshots(ctx context.Context, cores int) (overall cpuSnapshot, perCPU []cpuSnapshot, err error) {
	lines, err := utils.ReadLinesOffsetNWithContext(ctx, "/proc/stat", 0, -1)
	if err != nil {
		return cpuSnapshot{}, nil, err
	}
	for _, line := range lines {
		if err := ctx.Err(); err != nil {
			return cpuSnapshot{}, nil, err
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if !strings.HasPrefix(fields[0], "cpu") {
			if len(perCPU) > 0 {
				break
			}
			continue
		}

		snapshot, ok := parseCPUSnapshot(fields[1:])
		if !ok {
			log.Printf("invalid format of /proc/stat: %s", line)
			continue
		}

		if fields[0] == "cpu" {
			overall = snapshot
			continue
		}
		perCPU = append(perCPU, snapshot)

		if cores > 0 && len(perCPU) >= cores {
			break
		}
	}
	if len(perCPU) == 0 {
		return cpuSnapshot{}, nil, fmt.Errorf("no cpu core stats found in /proc/stat")
	}
	return overall, perCPU, nil
}

// parseCPUSnapshot 解析CPU快照
func parseCPUSnapshot(fields []string) (cpuSnapshot, bool) {
	if len(fields) < 4 {
		return cpuSnapshot{}, false
	}

	total := 0.0
	idle := 0.0
	for i, raw := range fields {
		val, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return cpuSnapshot{}, false
		}
		total += val
		if i == 3 || i == 4 {
			idle += val
		}
	}
	if total <= 0 {
		return cpuSnapshot{}, false
	}
	return cpuSnapshot{total: total, idle: idle}, true
}

// collectLoadAvg 采集负载平均值
func collectLoadAvg(ctx context.Context) ([]float64, error) {
	lines, err := utils.ReadLinesOffsetNWithContext(ctx, "/proc/loadavg", 0, -1)
	if err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return nil, fmt.Errorf("empty /proc/loadavg")
	}
	fields := strings.Fields(lines[0])
	if len(fields) < 3 {
		return nil, fmt.Errorf("invalid /proc/loadavg: %s", lines[0])
	}
	load1, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return nil, err
	}
	load5, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return nil, err
	}
	load15, err := strconv.ParseFloat(fields[2], 64)
	if err != nil {
		return nil, err
	}
	return []float64{load1, load5, load15}, nil
}
