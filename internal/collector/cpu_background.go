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

type cpuSampler struct {
	buffer cpuRingBuffer
}

// run 启动当前 LinuxCollector 专属的 CPU 采样循环。
// sampleInterval: 采样间隔（如 100ms）
// ctx: 上下文，用于优雅停止
func (s *cpuSampler) run(ctx context.Context, sampleInterval time.Duration) {
	if sampleInterval <= 0 {
		sampleInterval = 100 * time.Millisecond
	}

	ticker := time.NewTicker(sampleInterval)
	defer ticker.Stop()

	// 立即执行一次采集
	s.collect(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.collect(ctx)
		}
	}
}

// 采集快照并存入环形缓冲区
func (s *cpuSampler) collect(ctx context.Context) {
	snapshot, err := s.readSnapshot(ctx)
	if err != nil {
		if err != context.Canceled && err != context.DeadlineExceeded {
			log.Printf("[WARN] CPU background collect failed: %v", err)
		}
		return // 静默失败，不阻塞主流程
	}

	s.buffer.mu.Lock()
	// 写入当前 index 位置，然后切换到下一个位置
	s.buffer.snapshots[s.buffer.index] = snapshot
	s.buffer.index = (s.buffer.index + 1) % 2 // 0 -> 1 -> 0 循环

	// 记录核心数
	if s.buffer.cores == 0 && len(snapshot.perCPU) > 0 {
		s.buffer.cores = len(snapshot.perCPU)
	}
	s.buffer.mu.Unlock()
}

// 读取单次快照（不计算，用于后台采集）
func (s *cpuSampler) readSnapshot(ctx context.Context) (cpuCacheSnapshot, error) {
	s.buffer.mu.RLock()
	cores := s.buffer.cores
	s.buffer.mu.RUnlock()
	overall, perCPU, err := readCPUSnapshots(ctx, cores)
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
func (s *cpuSampler) usage() ([]float64, uint64, uint64, error) {
	snap0, snap1 := s.snapshots()

	// 检查是否有有效数据（至少一个非零时间戳）
	if snap0.timestamp.IsZero() && snap1.timestamp.IsZero() {
		return nil, 0, 0, nil
	}

	// 确保 snap0 是较早的快照，snap1 是较新的
	if snap1.timestamp.Before(snap0.timestamp) {
		snap0, snap1 = snap1, snap0
	}

	// 检查 CPU 核心数是否变化
	if len(snap0.perCPU) != len(snap1.perCPU) {
		// 核心数变化时，跳过 perCPU 计算，只返回总体
		log.Printf("[WARN] CPU core count changed: %d -> %d, skip perCPU",
			len(snap0.perCPU), len(snap1.perCPU))
	}

	// 计算总体的 CPU 使用率
	diffTotal := snap1.total - snap0.total
	diffIdle := snap1.idle - snap0.idle

	var totalTicks, idleTicks uint64
	if diffTotal > 0 {
		totalTicks = uint64(diffTotal)
		idleTicks = uint64(diffIdle)
	}

	// 计算每个 CPU 核心的使用率（仅当核心数一致时）
	perCPUUsage := make([]float64, 0, len(snap1.perCPU))
	if len(snap0.perCPU) == len(snap1.perCPU) && len(snap1.perCPU) > 0 {
		for i := 0; i < len(snap1.perCPU); i++ {
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
	}

	if len(perCPUUsage) == 0 {
		return nil, totalTicks, idleTicks, nil
	}

	return perCPUUsage, totalTicks, idleTicks, nil
}

func (s *cpuSampler) snapshots() (cpuCacheSnapshot, cpuCacheSnapshot) {
	s.buffer.mu.RLock()
	defer s.buffer.mu.RUnlock()
	return cloneCPUCacheSnapshot(s.buffer.snapshots[0]), cloneCPUCacheSnapshot(s.buffer.snapshots[1])
}

func cloneCPUCacheSnapshot(src cpuCacheSnapshot) cpuCacheSnapshot {
	dst := src
	dst.perCPU = append([]cpuCacheSnapshot(nil), src.perCPU...)
	return dst
}
