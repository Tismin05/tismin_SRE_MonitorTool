package collector

import (
	"context"
	"log"
	"strconv"
	"strings"
	"time"

	"tisminSRETool/internal/model"
	"tisminSRETool/pkg/utils"
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

	// 缓存无效，回退到直接读取
	m := make([]model.NetStat, 0)
	lines, err := utils.ReadLinesOffsetNWithContext(ctx, "/proc/net/dev", 2, -1)
	if err != nil {
		log.Printf("error collecting net io: %s", err)
		return nil, err
	}

	for _, line := range lines {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		separation := strings.LastIndex(line, ":")
		if separation == -1 {
			continue
		}
		parts := make([]string, 2)
		parts[0] = line[:separation]
		parts[1] = line[separation+1:]

		interfaceName := strings.TrimSpace(parts[0])
		if interfaceName == "" {
			continue
		}

		fields := strings.Fields(parts[1])
		if len(fields) < 12 {
			log.Printf("invalid format of /proc/net/dev: %s", line)
			continue
		}
		recvBytes, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			log.Printf("error collecting %s net io: %s", interfaceName, err)
			return nil, err
		}

		recvPackets, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			log.Printf("error collecting %s net io: %s", interfaceName, err)
			return nil, err
		}

		recvErrors, err := strconv.ParseUint(fields[2], 10, 64)
		if err != nil {
			log.Printf("error collecting %s net io: %s", interfaceName, err)
			return nil, err
		}

		recvDrops, err := strconv.ParseUint(fields[3], 10, 64)
		if err != nil {
			log.Printf("error collecting %s net io: %s", interfaceName, err)
			return nil, err
		}

		sendBytes, err := strconv.ParseUint(fields[8], 10, 64)
		if err != nil {
			log.Printf("error collecting %s net io: %s", interfaceName, err)
			return nil, err
		}

		sendPackets, err := strconv.ParseUint(fields[9], 10, 64)
		if err != nil {
			log.Printf("error collecting %s net io: %s", interfaceName, err)
			return nil, err
		}

		sendErrors, err := strconv.ParseUint(fields[10], 10, 64)
		if err != nil {
			log.Printf("error collecting %s net io: %s", interfaceName, err)
			return nil, err
		}

		sendDrops, err := strconv.ParseUint(fields[11], 10, 64)
		if err != nil {
			log.Printf("error collecting %s net io: %s", interfaceName, err)
			return nil, err
		}
		netStat := model.NetStat{
			Name:      interfaceName,
			RxBytes:   recvBytes,
			RxPackets: recvPackets,
			RxErrors:  recvErrors,
			RxDropped: recvDrops,
			TxBytes:   sendBytes,
			TxPackets: sendPackets,
			TxErrors:  sendErrors,
			TxDropped: sendDrops,
		}
		m = append(m, netStat)
	}
	return m, nil
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
