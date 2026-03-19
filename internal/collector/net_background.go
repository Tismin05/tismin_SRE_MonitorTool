package collector

import (
	"context"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"tisminSRETool/pkg/utils"
)

// 网络接口快照结构
type netSnapshot struct {
	timestamp time.Time
	iface     string
	rxBytes   uint64
	txBytes   uint64
	rxPackets uint64
	txPackets uint64
	rxErrors  uint64
	txErrors  uint64
	rxDrops   uint64
	txDrops   uint64
}

// 网络环形缓冲区：只保留最近2次快照
type netRingBuffer struct {
	snapshots [2][]netSnapshot
	index     int
	mu        sync.RWMutex
}

// 全局网络环形缓存
var netBuffer *netRingBuffer

// 初始化网络环形缓存
func initNetBuffer() {
	netBuffer = &netRingBuffer{
		snapshots: [2][]netSnapshot{},
		index:     0,
	}
}

// 启动后台网络采集 goroutine
func StartNetCollector(ctx context.Context, sampleInterval time.Duration) {
	if netBuffer == nil {
		initNetBuffer()
	}

	ticker := time.NewTicker(sampleInterval)
	defer ticker.Stop()

	// 立即执行一次采集
	collectNetToBuffer(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			collectNetToBuffer(ctx)
		}
	}
}

// 采集网络快照并存入环形缓冲区
func collectNetToBuffer(ctx context.Context) {
	snapshot, err := readNetSnapshotWithContext(ctx)
	if err != nil {
		if err != context.Canceled && err != context.DeadlineExceeded {
			log.Printf("[WARN] Net background collect failed: %v", err)
		}
		return // 静默失败
	}

	netBuffer.mu.Lock()
	netBuffer.snapshots[netBuffer.index] = snapshot
	netBuffer.index = (netBuffer.index + 1) % 2
	netBuffer.mu.Unlock()
}

// 读取网络快照
func readNetSnapshotWithContext(ctx context.Context) ([]netSnapshot, error) {
	lines, err := utils.ReadLinesOffsetNWithContext(ctx, "/proc/net/dev", 2, -1)
	if err != nil {
		return nil, err
	}

	var snapshots []netSnapshot
	now := time.Now()

	for _, line := range lines {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		separation := strings.LastIndex(line, ":")
		if separation == -1 {
			continue
		}

		iface := strings.TrimSpace(line[:separation])
		if iface == "" {
			continue
		}

		fields := strings.Fields(line[separation+1:])
		if len(fields) < 12 {
			continue
		}

		rxBytes, _ := strconv.ParseUint(fields[0], 10, 64)
		rxPackets, _ := strconv.ParseUint(fields[1], 10, 64)
		rxErrors, _ := strconv.ParseUint(fields[2], 10, 64)
		rxDrops, _ := strconv.ParseUint(fields[3], 10, 64)
		txBytes, _ := strconv.ParseUint(fields[8], 10, 64)
		txPackets, _ := strconv.ParseUint(fields[9], 10, 64)
		txErrors, _ := strconv.ParseUint(fields[10], 10, 64)
		txDrops, _ := strconv.ParseUint(fields[11], 10, 64)

		snapshots = append(snapshots, netSnapshot{
			timestamp: now,
			iface:     iface,
			rxBytes:   rxBytes,
			txBytes:   txBytes,
			rxPackets: rxPackets,
			txPackets: txPackets,
			rxErrors:  rxErrors,
			txErrors:  txErrors,
			rxDrops:   rxDrops,
			txDrops:   txDrops,
		})
	}

	return snapshots, nil
}

// 从网络环形缓冲区读取原始快照（主流程调用）
// 返回两个快照和时间差，由 CollectNetinfo 计算速率
func GetNetSnapshots() ([]netSnapshot, []netSnapshot, time.Time, time.Time) {
	if netBuffer == nil {
		return nil, nil, time.Time{}, time.Time{}
	}

	netBuffer.mu.RLock()
	snap0 := netBuffer.snapshots[0]
	snap1 := netBuffer.snapshots[1]
	netBuffer.mu.RUnlock()

	var t0, t1 time.Time
	if len(snap0) > 0 {
		t0 = snap0[0].timestamp
	}
	if len(snap1) > 0 {
		t1 = snap1[0].timestamp
	}

	return snap0, snap1, t0, t1
}
