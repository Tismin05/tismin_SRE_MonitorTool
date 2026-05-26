package collector

import (
	"context"
	"log"
	"sync"
	"time"
)

// cpuSnapshot CPU快照结构（无时间戳，供 readCPUSnapshots 使用）
type cpuSnapshot struct {
	total float64
	idle  float64
}

// CPU 缓存快照结构（带时间戳）
type cpuCacheSnapshot struct {
	timestamp time.Time
	total     float64
	idle      float64
	perCPU    []cpuCacheSnapshot // 每个 CPU 核心的快照
}

// 环形缓冲区：只保留最近2次快照
type cpuRingBuffer struct {
	snapshots [2]cpuCacheSnapshot
	index     int // 当前写入位置 (0 或 1)
	mu        sync.RWMutex
	cores     int
}

// 全局 CPU 环形缓存
var cpuBuffer *cpuRingBuffer

// 初始化环形缓存
func initCPUBuffer() {
	cpuBuffer = &cpuRingBuffer{
		snapshots: [2]cpuCacheSnapshot{
			{timestamp: time.Now()},
			{timestamp: time.Now()},
		},
		index: 0,
		cores: 0,
	}
}

// 启动后台 CPU 采集 goroutine
// sampleInterval: 采样间隔（如 100ms）
// ctx: 上下文，用于优雅停止
func StartCPUCollector(ctx context.Context, sampleInterval time.Duration) {
	if cpuBuffer == nil {
		initCPUBuffer()
	}

	ticker := time.NewTicker(sampleInterval)
	defer ticker.Stop()

	// 立即执行一次采集
	collectToBuffer(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			collectToBuffer(ctx)
		}
	}
}

// 采集快照并存入环形缓冲区
func collectToBuffer(ctx context.Context) {
	snapshot, err := readCPUSnapshotWithContext(ctx)
	if err != nil {
		if err != context.Canceled && err != context.DeadlineExceeded {
			log.Printf("[WARN] CPU background collect failed: %v", err)
		}
		return // 静默失败，不阻塞主流程
	}

	cpuBuffer.mu.Lock()
	// 写入当前 index 位置，然后切换到下一个位置
	cpuBuffer.snapshots[cpuBuffer.index] = snapshot
	cpuBuffer.index = (cpuBuffer.index + 1) % 2 // 0 -> 1 -> 0 循环

	// 记录核心数
	if cpuBuffer.cores == 0 && len(snapshot.perCPU) > 0 {
		cpuBuffer.cores = len(snapshot.perCPU)
	}
	cpuBuffer.mu.Unlock()
}

// 读取单次快照（不计算，用于后台采集）
func readCPUSnapshotWithContext(ctx context.Context) (cpuCacheSnapshot, error) {
	overall, perCPU, err := readCPUSnapshots(ctx, cpuBuffer.cores)
	if err != nil {
		return cpuCacheSnapshot{}, err
	}

	// 转换为简化格式
	perCPUSnapshots := make([]cpuCacheSnapshot, len(perCPU))
	for i, p := range perCPU {
		perCPUSnapshots[i] = cpuCacheSnapshot{
			timestamp: time.Now(),
			total:     p.total,
			idle:      p.idle,
		}
	}

	return cpuCacheSnapshot{
		timestamp: time.Now(),
		total:     overall.total,
		idle:      overall.idle,
		perCPU:    perCPUSnapshots,
	}, nil
}

// 从环形缓冲区读取并计算 CPU 使用率（主流程调用）
// 返回: perCPU使用率, 总tick, 空闲tick, 错误
func GetCPUUsageFromBuffer() ([]float64, uint64, uint64, error) {
	if cpuBuffer == nil {
		return nil, 0, 0, nil // 返回空，不阻塞
	}

	cpuBuffer.mu.RLock()
	snap0 := cpuBuffer.snapshots[0]
	snap1 := cpuBuffer.snapshots[1]
	cpuBuffer.mu.RUnlock()

	// 检查是否有有效数据
	if snap0.timestamp.IsZero() && snap1.timestamp.IsZero() {
		return nil, 0, 0, nil
	}

	// 确保 snap0 是较早的快照，snap1 是较新的
	if snap1.timestamp.Before(snap0.timestamp) {
		snap0, snap1 = snap1, snap0
	}

	// 计算总体的 CPU 使用率
	diffTotal := snap1.total - snap0.total

	var totalTicks, idleTicks uint64
	if diffTotal > 0 {
		totalTicks = uint64(snap1.total)
		idleTicks = uint64(snap1.idle)
	}

	// 计算每个 CPU 核心的使用率
	perCPUUsage := make([]float64, 0, len(snap1.perCPU))
	for i := 0; i < len(snap1.perCPU) && i < len(snap0.perCPU); i++ {
		diffCpuTotal := snap1.perCPU[i].total - snap0.perCPU[i].total
		diffCpuIdle := snap1.perCPU[i].idle - snap0.perCPU[i].idle

		if diffCpuTotal <= 0 {
			perCPUUsage = append(perCPUUsage, 0)
			continue
		}

		usage := (diffCpuTotal - diffCpuIdle) / diffCpuTotal * 100
		if usage < 0 {
			usage = 0
		}
		if usage > 100 {
			usage = 100
		}
		perCPUUsage = append(perCPUUsage, usage)
	}

	if len(perCPUUsage) == 0 {
		return nil, totalTicks, idleTicks, nil
	}

	return perCPUUsage, totalTicks, idleTicks, nil
}
