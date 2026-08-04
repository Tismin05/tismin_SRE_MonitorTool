package collector

import (
	"context"
	"errors"
	"fmt"
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

type diskSampler struct {
	mu        sync.Mutex
	stats     map[string]DiskIOStat
	sampledAt time.Time
}

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
	if len(mounts) == 0 {
		return nil, fmt.Errorf("parse /proc/mounts: no valid physical mounts")
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
	return parseDiskStatsLines(ctx, lines)
}

func parseDiskStatsLines(ctx context.Context, lines []string) (map[string]DiskIOStat, error) {
	stats := make(map[string]DiskIOStat)
	var parseErrors []error
	for _, line := range lines {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		stat, err := parseDiskStatsLine(line)
		if err != nil {
			parseErrors = append(parseErrors, err)
			continue
		}
		name := stat.Name

		// 过滤虚拟设备和分区
		if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") {
			continue
		}
		// 如果是分区（如 sda1, nvme0n1p1），只保留整盘
		//if isPartition(name) {
		//	continue
		//}

		stats[name] = stat
	}
	if len(stats) == 0 {
		parseErrors = append(parseErrors, fmt.Errorf("parse /proc/diskstats: no valid physical device rows"))
	}
	return stats, errors.Join(parseErrors...)
}

func parseDiskStatsLine(line string) (DiskIOStat, error) {
	fields := strings.Fields(line)
	if len(fields) < 14 {
		return DiskIOStat{}, fmt.Errorf("parse /proc/diskstats line %q: expected at least 14 fields, got %d", line, len(fields))
	}

	device := strings.TrimSpace(fields[2])
	if device == "" {
		return DiskIOStat{}, fmt.Errorf("parse /proc/diskstats line %q: empty device name", line)
	}

	fieldIndexes := []struct {
		name  string
		index int
	}{
		{name: "read_ios", index: 3},
		{name: "read_sectors", index: 5},
		{name: "write_ios", index: 7},
		{name: "write_sectors", index: 9},
		{name: "io_queue_time", index: 12},
	}
	values := make([]uint64, len(fieldIndexes))
	for i, field := range fieldIndexes {
		value, err := parseDiskUint(device, field.name, fields[field.index])
		if err != nil {
			return DiskIOStat{}, err
		}
		values[i] = value
	}

	return DiskIOStat{
		Name:         device,
		ReadIOs:      values[0],
		ReadSectors:  values[1],
		WriteIOs:     values[2],
		WriteSectors: values[3],
		IOQueuesTime: values[4],
	}, nil
}

func parseDiskUint(device, field, raw string) (uint64, error) {
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse /proc/diskstats device %q field %s value %q: %w", device, field, raw, err)
	}
	return value, nil
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

type diskStatFSFunc func(string) (total, free, avail, inodes, inodesFree uint64, err error)

// CollectDisk 保留无状态兼容入口。LinuxCollector 使用自身的 diskSampler 维护速率基线。
func CollectDisk(ctx context.Context) ([]model.DiskStat, error) {
	return collectDisk(ctx, &diskSampler{})
}

func collectDisk(ctx context.Context, sampler *diskSampler) ([]model.DiskStat, error) {
	return collectDiskWithDeps(ctx, sampler, readMounts, readDiskStats, statFS, time.Now)
}

func collectDiskWithDeps(
	ctx context.Context,
	sampler *diskSampler,
	readMountsFn func(context.Context) (map[string]string, error),
	readDiskStatsFn func(context.Context) (map[string]DiskIOStat, error),
	statFSFn diskStatFSFunc,
	nowFn func() time.Time,
) ([]model.DiskStat, error) {
	if sampler == nil {
		sampler = &diskSampler{}
	}

	// 获取设备名 -> 挂载点 映射
	mounts, err := readMountsFn(ctx)
	if err != nil {
		return nil, err
	}

	// 获取物理磁盘 IO 统计
	ioStats, parseErr := readDiskStatsFn(ctx)
	if parseErr != nil && len(ioStats) == 0 {
		return nil, parseErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	now := nowFn()
	prevStats, prevAt := sampler.previous()
	elapsedSec := 0.0
	if !prevAt.IsZero() {
		elapsedSec = now.Sub(prevAt).Seconds()
	}

	var out []model.DiskStat
	var collectErrors []error
	if parseErr != nil {
		collectErrors = append(collectErrors, parseErr)
	}
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
		total, free, _, inodes, inodesFree, err := statFSFn(mountPoint)
		if err != nil {
			collectErrors = append(collectErrors, fmt.Errorf("statfs device %q mount %q: %w", deviceName, mountPoint, err))
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
	if len(out) == 0 {
		collectErrors = append(collectErrors, fmt.Errorf("collect disk: no valid mounted device metrics"))
	}
	collectErr := errors.Join(collectErrors...)
	if collectErr == nil {
		sampler.commit(ioStats, now)
	}
	return out, collectErr
}

func (s *diskSampler) previous() (map[string]DiskIOStat, time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneDiskIOStats(s.stats), s.sampledAt
}

func (s *diskSampler) commit(current map[string]DiskIOStat, sampledAt time.Time) {
	s.mu.Lock()
	s.stats = cloneDiskIOStats(current)
	s.sampledAt = sampledAt
	s.mu.Unlock()
}

func cloneDiskIOStats(src map[string]DiskIOStat) map[string]DiskIOStat {
	if src == nil {
		return nil
	}
	dst := make(map[string]DiskIOStat, len(src))
	for name, stat := range src {
		dst[name] = stat
	}
	return dst
}

// uint64Diff 计算uint64差值
func uint64Diff(cur, prev uint64) uint64 {
	if cur < prev {
		return 0
	}
	return cur - prev
}
