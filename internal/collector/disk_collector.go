package collector

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"tisminSRETool/internal/model"
	"tisminSRETool/pkg/utils"

	"golang.org/x/sys/unix"
)

// DiskIOStat 磁盘IO统计
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

// readMounts 读取 /proc/mounts 获取真实挂载点（过滤虚拟文件系统）
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

// isVirtualFS 判断是否为虚拟文件系统
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



// statFS statfs 取容量
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

// readDiskStats 读取 /proc/diskstats (IO 计数)，只保留物理磁盘
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
			// Failed to statfs, usually due to hung mount point
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

// snapshotPrevDiskStats 快照上一次磁盘统计
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

// uint64Diff 计算uint64差值
func uint64Diff(cur, prev uint64) uint64 {
	if cur < prev {
		return 0
	}
	return cur - prev
}
