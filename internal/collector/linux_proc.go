package collector

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"
	"tisminSRETool/internal/model"
	"tisminSRETool/pkg/utils"

	"golang.org/x/sys/unix"
)

// CPU信息收集核心逻辑
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

// CollectCPUUsage is deprecated in favor of CollectCPUStat.
//func CollectCPUUsage(ctx context.Context, cores int) (usage float64, err error) {
//	var total float64
//	perCPU, err := CollectCPUInfo(ctx, cores)
//	if err != nil {
//		return 0, err
//	}
//	if len(perCPU) == 0 {
//		return 0, fmt.Errorf("no cpu usage samples")
//	}
//	for _, val := range perCPU {
//		total += val
//	}
//	return total / float64(len(perCPU)), nil
//}

// 整合CPU逻辑
func CollectCPUStat(ctx context.Context) (model.CPUStat, error) {
	cores, err := collectCPUCores(ctx)
	if err != nil {
		return model.CPUStat{}, err
	}

	perCPU, totalTicks, idleTicks, err := collectCPUInfo(ctx, cores)
	if err != nil {
		return model.CPUStat{}, err
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

type cpuSnapshot struct {
	total float64
	idle  float64
}

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

func CollectMeminfo(ctx context.Context) (m *model.MemoryStat, err error) {
	lines, err := utils.ReadLinesOffsetNWithContext(ctx, "/proc/meminfo", 0, -1)
	if err != nil {
		log.Printf("error collecting meminfo: %s", err)
		return nil, err
	}

	ret := &model.MemoryStat{}
	var buffer uint64
	var cache uint64
	var available uint64
	for _, line := range lines {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		fields := strings.SplitN(line, ":", 2)
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSpace(fields[0])
		valueFields := strings.Fields(strings.TrimSpace(fields[1]))
		if len(valueFields) == 0 {
			continue
		}
		value := valueFields[0]
		switch key {
		case "MemTotal":
			t, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				log.Printf("error collecting MemTotal: %s", err)
				return ret, err
			}
			ret.Total = t * 1024
		case "MemFree":
			t, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				log.Printf("error collecting MemFree: %s", err)
				return ret, err
			}
			ret.Free = t * 1024
		case "MemAvailable":
			t, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				log.Printf("error collecting MemAvailable: %s", err)
				return ret, err
			}
			available = t * 1024
		case "SwapTotal":
			t, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				log.Printf("error collecting SwapTotal: %s", err)
				return ret, err
			}
			ret.SwapTotal = t * 1024
		case "SwapFree":
			t, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				log.Printf("error collecting SwapFree: %s", err)
				return ret, err
			}
			ret.SwapFree = t * 1024
		case "Buffers":
			t, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				log.Printf("error collecting Buffers: %s", err)
				return ret, err
			}
			buffer = t * 1024
		case "Cached":
			t, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				log.Printf("error collecting Cached: %s", err)
				return ret, err
			}
			cache = t * 1024
		default:
			continue
		}
	}
	ret.Available = available
	if available > 0 {
		ret.Used = ret.Total - available
	} else {
		ret.Used = ret.Total - ret.Free - buffer - cache
	}
	if ret.Total > 0 {
		ret.UsedPercent = float64(ret.Used) / float64(ret.Total) * 100
	}
	if ret.SwapTotal >= ret.SwapFree {
		ret.SwapUsed = ret.SwapTotal - ret.SwapFree
		if ret.SwapTotal > 0 {
			ret.SwapUsedPercent = float64(ret.SwapUsed) / float64(ret.SwapTotal) * 100
		}
	}
	return ret, nil
}

// 读取 /proc/mounts 获取真实挂载点（过滤虚拟文件系统）
func readMounts(ctx context.Context) (map[string]string, error) {
	lines, err := utils.ReadLinesOffsetNWithContext(ctx, "/proc/mounts", 0, -1)
	if err != nil {
		return nil, err
	}
	// device -> mountPoint 映射
	mounts := make(map[string]string)
	for _, line := range lines {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		device := fields[0]
		mountPoint := fields[1]
		fsType := fields[2]

		// 过滤虚拟文件系统
		if isVirtualFS(fsType) {
			continue
		}

		// 转换设备名（如 /dev/sda -> sda）
		deviceName := strings.TrimPrefix(device, "/dev/")
		mounts[deviceName] = mountPoint
	}
	return mounts, nil
}

// 判断是否为虚拟文件系统
func isVirtualFS(fsType string) bool {
	virtualFSTypes := map[string]bool{
		"tmpfs":      true,
		"devtmpfs":   true,
		"overlay":    true,
		"aufs":       true,
		"devpts":     true,
		"sysfs":      true,
		"proc":       true,
		"cgroup":     true,
		"cgroup2":    true,
		"securityfs": true,
		"pstore":     true,
		"efivarfs":   true,
		"bpf":        true,
		"tracefs":    true,
		"hugetlbfs":  true,
		"mqueue":     true,
		"fusectl":    true,
		"configfs":   true,
		"debugfs":    true,
		"selinuxfs":  true,
	}
	return virtualFSTypes[fsType]
}

// 2) statfs 取容量
func statFS(path string) (total, free, avail, inodes, inodesFree uint64, err error) {
	var st unix.Statfs_t
	if err = unix.Statfs(path, &st); err != nil {
		return
	}
	total = st.Blocks * uint64(st.Bsize)
	free = st.Bfree * uint64(st.Bsize)
	avail = st.Bavail * uint64(st.Bsize)
	inodes = st.Files
	inodesFree = st.Ffree
	return
}

// 3) 读取 /proc/diskstats (IO 计数)，只保留物理磁盘
type DiskIOStat struct {
	Name         string
	ReadIOs      uint64 // 读 I/O 次数
	ReadSectors  uint64 // 读扇区数
	WriteIOs     uint64 // 写 I/O 次数
	WriteSectors uint64 // 写扇区数
	IOQueuesTime uint64 // I/O等待时间 ms
}

var (
	diskStateMu    sync.Mutex
	prevDiskStats  map[string]DiskIOStat
	prevDiskStatAt time.Time
)

const sectorSizeBytes uint64 = 512

func readDiskStats(ctx context.Context) (map[string]DiskIOStat, error) {
	lines, err := utils.ReadLinesOffsetNWithContext(ctx, "/proc/diskstats", 0, -1)
	if err != nil {
		return nil, err
	}
	stats := make(map[string]DiskIOStat)
	for _, line := range lines {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		fields := strings.Fields(line)
		if len(fields) < 14 {
			continue
		}
		name := strings.TrimSpace(fields[2])

		// 过滤虚拟设备和分区
		if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") {
			continue
		}
		// 如果是分区（如 sda1, nvme0n1p1），只保留整盘
		//if isPartition(name) {
		//	continue
		//}

		readIO, _ := strconv.ParseUint(fields[3], 10, 64)
		readSectors, _ := strconv.ParseUint(fields[5], 10, 64)
		writeIO, _ := strconv.ParseUint(fields[7], 10, 64)
		writeSectors, _ := strconv.ParseUint(fields[9], 10, 64)
		ioQueuesTime, _ := strconv.ParseUint(fields[12], 10, 64)

		stats[name] = DiskIOStat{
			Name:         name,
			ReadIOs:      readIO,
			ReadSectors:  readSectors,
			WriteIOs:     writeIO,
			WriteSectors: writeSectors,
			IOQueuesTime: ioQueuesTime,
		}
	}
	return stats, nil
}

// 判断是否为分区
// sda, nvme0n1 -> false (整盘)
// sda1, nvme0n1p1 -> true (分区)
/*func isPartition(name string) bool {
	// NVMe 设备: nvme0n1p1
	if strings.HasPrefix(name, "nvme") && strings.Contains(name, "p") {
		return true
	}
	// SCSI/SATA 设备: sda1, sdb2
	if len(name) > 3 {
		_, err := strconv.Atoi(name[3:])
		return err == nil
	}
	return false
}*/

// CollectDisk 组合 DiskStat 从物理磁盘出发，查找挂载点，正确匹配 IO 统计
func CollectDisk(ctx context.Context) ([]model.DiskStat, error) {
	// 获取设备名 -> 挂载点 映射
	mounts, err := readMounts(ctx)
	if err != nil {
		return nil, err
	}

	// 获取物理磁盘 IO 统计
	ioStats, err := readDiskStats(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	prevStats, elapsedSec := snapshotPrevDiskStats(ioStats, now)

	var out []model.DiskStat
	// 遍历物理磁盘
	for deviceName, ioStat := range ioStats {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		// 查找该物理磁盘挂载点
		mountPoint, ok := mounts[deviceName]
		if !ok {
			// 物理磁盘没有挂载点（如未使用的磁盘），跳过
			continue
		}

		// 获取容量信息
		total, free, _, inodes, inodesFree, err := statFS(mountPoint)
		if err != nil {
			continue
		}
		used := total - free
		usedPct := 0.0
		if total > 0 {
			usedPct = float64(used) / float64(total) * 100
		}

		readBytes := ioStat.ReadSectors * sectorSizeBytes
		writeBytes := ioStat.WriteSectors * sectorSizeBytes

		await := 0.0
		util := 0.0
		if elapsedSec > 0 && prevStats != nil {
			if prev, ok := prevStats[deviceName]; ok {
				diffReadIO := uint64Diff(ioStat.ReadIOs, prev.ReadIOs)
				diffWriteIO := uint64Diff(ioStat.WriteIOs, prev.WriteIOs)
				diffIOs := diffReadIO + diffWriteIO
				diffQueueTime := uint64Diff(ioStat.IOQueuesTime, prev.IOQueuesTime)

				if diffIOs > 0 {
					await = float64(diffQueueTime) / float64(diffIOs)
				}
				util = float64(diffQueueTime) / (elapsedSec * 1000) * 100
				if util < 0 {
					util = 0
				}
			}
		}

		out = append(out, model.DiskStat{
			MountPoint:        mountPoint,
			Device:            deviceName,
			Total:             total,
			Free:              free,
			Used:              used,
			UsedPercent:       usedPct,
			InodesTotal:       inodes,
			InodesFree:        inodesFree,
			InodesUsed:        inodes - inodesFree,
			InodesUsedPercent: utils.Pct(inodes-inodesFree, inodes),
			Read:              readBytes,
			ReadSectors:       ioStat.ReadSectors,
			Write:             writeBytes,
			WriteSectors:      ioStat.WriteSectors,
			Await:             await,
			Util:              util,
			IOQueueTime:       ioStat.IOQueuesTime,
		})
	}
	return out, nil
}

func snapshotPrevDiskStats(current map[string]DiskIOStat, now time.Time) (map[string]DiskIOStat, float64) {
	diskStateMu.Lock()
	defer diskStateMu.Unlock()

	previous := prevDiskStats
	elapsed := 0.0
	if !prevDiskStatAt.IsZero() {
		elapsed = now.Sub(prevDiskStatAt).Seconds()
	}
	prevDiskStats = current
	prevDiskStatAt = now
	return previous, elapsed
}

func uint64Diff(cur, prev uint64) uint64 {
	if cur < prev {
		return 0
	}
	return cur - prev
}

func CollectNetinfo(ctx context.Context) ([]model.NetStat, error) {
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
