package collector

import (
	"context"
	"time"

	"tisminSRETool/internal/model"
)

// CollectNetinfo 采集网络信息
func CollectNetinfo(ctx context.Context) ([]model.NetStat, error) {
	return collectNetinfo(ctx, nil)
}

func collectNetinfo(ctx context.Context, sampler *netSampler) ([]model.NetStat, error) {
	// 尝试从环形缓存读取
	var snap0, snap1 []netSnapshot
	var t0, t1 time.Time
	if sampler != nil {
		snap0, snap1, t0, t1 = sampler.snapshots()
	}

	if len(snap0) > 0 && len(snap1) > 0 && !t0.IsZero() && !t1.IsZero() {
		// 使用缓存计算速率
		elapsed := t1.Sub(t0).Seconds()
		if elapsed > 0 {
			return calcNetStatsFromSnapshots(snap0, snap1, elapsed)
		}
	}

	// 缓存无效，回退到直接读取。保留合法网卡数据和聚合解析错误。
	snapshots, err := readNetSnapshotWithContext(ctx)
	m := make([]model.NetStat, 0, len(snapshots))
	for _, snapshot := range snapshots {
		m = append(m, model.NetStat{
			Name:      snapshot.iface,
			RxBytes:   snapshot.rxBytes,
			RxPackets: snapshot.rxPackets,
			RxErrors:  snapshot.rxErrors,
			RxDropped: snapshot.rxDrops,
			TxBytes:   snapshot.txBytes,
			TxPackets: snapshot.txPackets,
			TxErrors:  snapshot.txErrors,
			TxDropped: snapshot.txDrops,
		})
	}
	return m, err
}

// calcNetStatsFromSnapshots 从缓存快照计算网络速率
func calcNetStatsFromSnapshots(snap0, snap1 []netSnapshot, elapsed float64) ([]model.NetStat, error) {
	snap0Map := make(map[string]netSnapshot)
	for _, s := range snap0 {
		snap0Map[s.iface] = s
	}

	var result []model.NetStat
	for _, s1 := range snap1 {
		s0, ok := snap0Map[s1.iface]
		if !ok {
			continue
		}

		rxBytesDelta := uint64Diff(s1.rxBytes, s0.rxBytes)
		txBytesDelta := uint64Diff(s1.txBytes, s0.txBytes)

		result = append(result, model.NetStat{
			Name:      s1.iface,
			RxBytes:   s1.rxBytes,
			TxBytes:   s1.txBytes,
			RxPackets: s1.rxPackets,
			TxPackets: s1.txPackets,
			RxErrors:  s1.rxErrors,
			TxErrors:  s1.txErrors,
			RxDropped: s1.rxDrops,
			TxDropped: s1.txDrops,
			RxSpeed:   float64(rxBytesDelta) / elapsed,
			TxSpeed:   float64(txBytesDelta) / elapsed,
		})
	}

	return result, nil
}
