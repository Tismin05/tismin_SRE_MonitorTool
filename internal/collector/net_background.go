package collector

import (
	"context"
	"errors"
	"fmt"
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

type netSampler struct {
	buffer netRingBuffer
}

// 启动后台网络采集 goroutine
func (s *netSampler) run(ctx context.Context, sampleInterval time.Duration) {
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

// 采集网络快照并存入环形缓冲区
func (s *netSampler) collect(ctx context.Context) {
	s.collectWithReader(ctx, readNetSnapshotWithContext)
}

func (s *netSampler) collectWithReader(ctx context.Context, read func(context.Context) ([]netSnapshot, error)) {
	snapshot, err := read(ctx)
	if err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			log.Printf("[WARN] network background sample skipped: %v", err)
		}
		return
	}

	s.buffer.mu.Lock()
	s.buffer.snapshots[s.buffer.index] = snapshot
	s.buffer.index = (s.buffer.index + 1) % 2
	s.buffer.mu.Unlock()
}

// 读取网络快照
func readNetSnapshotWithContext(ctx context.Context) ([]netSnapshot, error) {
	lines, err := utils.ReadLinesOffsetNWithContext(ctx, "/proc/net/dev", 2, -1)
	if err != nil {
		return nil, err
	}
	return parseNetSnapshotLines(ctx, lines, time.Now())
}

func parseNetSnapshotLines(ctx context.Context, lines []string, timestamp time.Time) ([]netSnapshot, error) {
	snapshots := make([]netSnapshot, 0, len(lines))
	var parseErrors []error

	for _, line := range lines {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		snapshot, err := parseNetSnapshotLine(line, timestamp)
		if err != nil {
			parseErrors = append(parseErrors, err)
			continue
		}
		snapshots = append(snapshots, snapshot)
	}

	if len(snapshots) == 0 {
		parseErrors = append(parseErrors, fmt.Errorf("parse /proc/net/dev: no valid interface rows"))
	}
	return snapshots, errors.Join(parseErrors...)
}

func parseNetSnapshotLine(line string, timestamp time.Time) (netSnapshot, error) {
	separation := strings.LastIndex(line, ":")
	if separation == -1 {
		return netSnapshot{}, fmt.Errorf("parse /proc/net/dev line %q: missing interface separator", line)
	}

	iface := strings.TrimSpace(line[:separation])
	if iface == "" {
		return netSnapshot{}, fmt.Errorf("parse /proc/net/dev line %q: empty interface name", line)
	}

	fields := strings.Fields(line[separation+1:])
	if len(fields) < 12 {
		return netSnapshot{}, fmt.Errorf("parse /proc/net/dev interface %q: expected at least 12 fields, got %d", iface, len(fields))
	}

	fieldIndexes := []struct {
		name  string
		index int
	}{
		{name: "rx_bytes", index: 0},
		{name: "rx_packets", index: 1},
		{name: "rx_errors", index: 2},
		{name: "rx_drops", index: 3},
		{name: "tx_bytes", index: 8},
		{name: "tx_packets", index: 9},
		{name: "tx_errors", index: 10},
		{name: "tx_drops", index: 11},
	}
	values := make([]uint64, len(fieldIndexes))
	for i, field := range fieldIndexes {
		value, err := parseNetUint(iface, field.name, fields[field.index])
		if err != nil {
			return netSnapshot{}, err
		}
		values[i] = value
	}

	return netSnapshot{
		timestamp: timestamp,
		iface:     iface,
		rxBytes:   values[0],
		rxPackets: values[1],
		rxErrors:  values[2],
		rxDrops:   values[3],
		txBytes:   values[4],
		txPackets: values[5],
		txErrors:  values[6],
		txDrops:   values[7],
	}, nil
}

func parseNetUint(iface, field, raw string) (uint64, error) {
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse /proc/net/dev interface %q field %s value %q: %w", iface, field, raw, err)
	}
	return value, nil
}

// 从网络环形缓冲区读取原始快照（主流程调用）
// 返回两个快照和时间差，由 CollectNetinfo 计算速率
func (s *netSampler) snapshots() ([]netSnapshot, []netSnapshot, time.Time, time.Time) {
	s.buffer.mu.RLock()
	snap0 := append([]netSnapshot(nil), s.buffer.snapshots[0]...)
	snap1 := append([]netSnapshot(nil), s.buffer.snapshots[1]...)
	s.buffer.mu.RUnlock()

	var t0, t1 time.Time
	if len(snap0) > 0 {
		t0 = snap0[0].timestamp
	}
	if len(snap1) > 0 {
		t1 = snap1[0].timestamp
	}
	if !t0.IsZero() && !t1.IsZero() && t1.Before(t0) {
		snap0, snap1 = snap1, snap0
		t0, t1 = t1, t0
	}

	return snap0, snap1, t0, t1
}
