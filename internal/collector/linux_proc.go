//go:build proc_refactor
// +build proc_refactor

package collector

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
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

	perCPU, err := collectCPUInfo(ctx, cores)
	if err != nil {
		return model.CPUStat{}, err
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
	}, nil
}

func collectCPUInfo(ctx context.Context, cores int) (perCPU []float64, err error) {
	var perCPUUsage []float64
	lines, err := utils.ReadLinesOffsetNWithContext(ctx, "/proc/stat", 0, -1)
	if err != nil {
		return nil, err
	}
	for _, line := range lines[1 : cores+1] {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			log.Printf("invalid format of /proc/stat: %s", line)
			continue
		}
		if fields[0] == "cpu" {
			continue
		}
		if len(fields) < 5 {
			log.Printf("invalid format of /proc/stat: %s", line)
			continue
		}

		usr, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			log.Printf("invalid format of /proc/stat: %s", line)
			continue
		}

		nice, err := strconv.ParseFloat(fields[2], 64)
		if err != nil {
			log.Printf("invalid format of /proc/stat: %s", line)
			continue
		}

		system, err := strconv.ParseFloat(fields[3], 64)
		if err != nil {
			log.Printf("invalid format of /proc/stat: %s", line)
			continue
		}

		idle, err := strconv.ParseFloat(fields[4], 64)
		if err != nil {
			log.Printf("invalid format of /proc/stat: %s", line)
			continue
		}

		iowait := 0.0
		if len(fields) > 5 {
			iowait, _ = strconv.ParseFloat(fields[5], 64)
		}
		irq := 0.0
		if len(fields) > 6 {
			irq, _ = strconv.ParseFloat(fields[6], 64)
		}
		softirq := 0.0
		if len(fields) > 7 {
			softirq, _ = strconv.ParseFloat(fields[7], 64)
		}
		steal := 0.0
		if len(fields) > 8 {
			steal, _ = strconv.ParseFloat(fields[8], 64)
		}
		guest := 0.0
		if len(fields) > 9 {
			guest, _ = strconv.ParseFloat(fields[9], 64)
		}
		guestnice := 0.0
		if len(fields) > 10 {
			guestnice, _ = strconv.ParseFloat(fields[10], 64)
		}

		total := usr + nice + system + idle + iowait + irq + softirq + steal + guest + guestnice
		if total <= 0 {
			continue
		}
		busy := total - idle - iowait
		usage := busy / total * 100
		perCPUUsage = append(perCPUUsage, usage)
	}
	return perCPUUsage, nil
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
	ReadIOs      uint64
	ReadSectors  uint64
	WriteIOs     uint64
	WriteSectors uint64
}

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
		// - loop*: 虚拟回环设备
		// - ram*: 内存盘
		// - sda/sdb 等后面带数字的：分区（如 sda1），只保留整盘（sda）
		if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") {
			continue
		}
		// 如果是分区（如 sda1, nvme0n1p1），只保留整盘
		if isPartition(name) {
			continue
		}

		readIO, _ := strconv.ParseUint(fields[3], 10, 64)
		readSectors, _ := strconv.ParseUint(fields[5], 10, 64)
		writeIO, _ := strconv.ParseUint(fields[7], 10, 64)
		writeSectors, _ := strconv.ParseUint(fields[9], 10, 64)
		stats[name] = DiskIOStat{
			Name:         name,
			ReadIOs:      readIO,
			ReadSectors:  readSectors,
			WriteIOs:     writeIO,
			WriteSectors: writeSectors,
		}
	}
	return stats, nil
}

// 判断是否为分区
// sda, nvme0n1 -> false (整盘)
// sda1, nvme0n1p1 -> true (分区)
func isPartition(name string) bool {
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
}

// 4) 组合成你的 DiskStat
// 思路：从物理磁盘出发，查找挂载点，正确匹配 IO 统计
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

	var out []model.DiskStat
	// 遍历物理磁盘，而非挂载点
	for deviceName, ioStat := range ioStats {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		// 查找该物理磁盘的挂载点
		mountPoint, ok := mounts[deviceName]
		if !ok {
			// 物理磁盘没有挂载点（如未使用的磁盘），跳过
			continue
		}

		// 获取容量信息
		total, free, avail, inodes, inodesFree, err := statFS(mountPoint)
		if err != nil {
			continue
		}
		used := total - free
		usedPct := 0.0
		if total > 0 {
			usedPct = float64(used) / float64(total) * 100
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
			Read:              ioStat.ReadIOs,
			Write:             ioStat.WriteIOs,
		})
	}
	return out, nil
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
