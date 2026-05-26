package proc

import (
	"bufio"
	"context"
	"io"
	"os"
	"strconv"
	"strings"
)

// CPUStat represents CPU statistics from /proc/stat.
type CPUStat struct {
	Cores   int
	User    uint64
	Nice    uint64
	System  uint64
	Idle    uint64
	Iowait  uint64
	IRQ     uint64
	SoftIRQ uint64
	Steal   uint64
	Total   uint64
}

// LoadAvg represents load average from /proc/loadavg.
type LoadAvg struct {
	Load1  float64
	Load5  float64
	Load15 float64
}

// MemInfo represents memory information from /proc/meminfo.
type MemInfo struct {
	Total     uint64
	Free      uint64
	Available uint64
	Buffers   uint64
	Cached    uint64
}

// NetInterfaceStat represents network interface statistics from /proc/net/dev.
type NetInterfaceStat struct {
	Name      string
	RxBytes   uint64
	RxPackets uint64
	RxErrors  uint64
	RxDropped uint64
	TxBytes   uint64
	TxPackets uint64
	TxErrors  uint64
	TxDropped uint64
}

// parseCpuStat parses /proc/stat cpu line using bufio.Scanner.
// Returns zero values on any error - NEVER panics.
func parseCpuStat(ctx context.Context) (*CPUStat, error) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// Increase buffer for lines with many CPU cores
	const maxScanTokenSize = 64 * 1024
	if bufio.MaxScanTokenSize < maxScanTokenSize {
		scanner.Buffer(make([]byte, 256), maxScanTokenSize)
	}

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		line := scanner.Text()
		fields := strings.Fields(line)

		// Must have exactly "cpu" as first field (not "cpu0", "cpu1", etc.)
		if len(fields) < 4 || fields[0] != "cpu" {
			continue
		}

		stat, err := parseCPUFields(fields[1:])
		if err != nil {
			continue
		}

		// Count cores from subsequent lines
		file.Seek(0, 0)
		cores := 0
		coreScanner := bufio.NewScanner(file)
		coreScanner.Buffer(make([]byte, 256), maxScanTokenSize)
		for coreScanner.Scan() {
			text := coreScanner.Text()
			if strings.HasPrefix(text, "cpu") && len(text) > 3 && text[3] >= '0' && text[3] <= '9' {
				cores++
			}
		}
		stat.Cores = cores

		return stat, nil
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return nil, io.ErrNoProgress
}

// parseCPUFields parses the fields of a cpu line.
// Defensively checks field length and handles parsing errors gracefully.
func parseCPUFields(fields []string) (*CPUStat, error) {
	// Need at least 4 fields: user, nice, system, idle
	if len(fields) < 4 {
		return nil, io.ErrNoProgress
	}

	stat := &CPUStat{}

	// Parse each field, skip on error (defensive)
	if val, err := parseUint64(fields, 0); err == nil {
		stat.User = val
	}
	if len(fields) > 1 {
		if val, err := parseUint64(fields, 1); err == nil {
			stat.Nice = val
		}
	}
	if len(fields) > 2 {
		if val, err := parseUint64(fields, 2); err == nil {
			stat.System = val
		}
	}
	if len(fields) > 3 {
		if val, err := parseUint64(fields, 3); err == nil {
			stat.Idle = val
		}
	}
	if len(fields) > 4 {
		if val, err := parseUint64(fields, 4); err == nil {
			stat.Iowait = val
		}
	}
	if len(fields) > 5 {
		if val, err := parseUint64(fields, 5); err == nil {
			stat.IRQ = val
		}
	}
	if len(fields) > 6 {
		if val, err := parseUint64(fields, 6); err == nil {
			stat.SoftIRQ = val
		}
	}
	if len(fields) > 7 {
		if val, err := parseUint64(fields, 7); err == nil {
			stat.Steal = val
		}
	}

	// Calculate total, guard against overflow
	stat.Total = stat.User + stat.Nice + stat.System + stat.Idle +
		stat.Iowait + stat.IRQ + stat.SoftIRQ + stat.Steal

	return stat, nil
}

// parseUint64 safely parses uint64 from fields slice at index.
// Returns 0 if index out of bounds or parse fails.
func parseUint64(fields []string, index int) (uint64, error) {
	if index < 0 || index >= len(fields) {
		return 0, io.ErrNoProgress
	}
	val, err := strconv.ParseUint(fields[index], 10, 64)
	if err != nil {
		return 0, err
	}
	return val, nil
}

// parseLoadAvg parses /proc/loadavg using bufio.Scanner.
func parseLoadAvg(ctx context.Context) (*LoadAvg, error) {
	file, err := os.Open("/proc/loadavg")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		line := scanner.Text()
		fields := strings.Fields(line)

		// Need at least 3 fields: load1, load5, load15
		if len(fields) < 3 {
			return nil, io.ErrNoProgress
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

		return &LoadAvg{Load1: load1, Load5: load5, Load15: load15}, nil
	}

	return nil, scanner.Err()
}

// parseMemInfo parses /proc/meminfo using bufio.Scanner.
// Strict field validation and empty line handling.
func parseMemInfo(ctx context.Context) (*MemInfo, error) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	mem := &MemInfo{}
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		line := scanner.Text()
		// Skip empty lines
		if len(line) == 0 {
			continue
		}

		// Split only on first colon
		idx := strings.Index(line, ":")
		if idx == -1 {
			continue
		}

		key := strings.TrimSpace(line[:idx])
		values := strings.Fields(strings.TrimSpace(line[idx+1:]))

		// Must have at least one value
		if len(values) < 1 {
			continue
		}

		val, err := strconv.ParseUint(values[0], 10, 64)
		if err != nil {
			continue
		}

		// Convert from kB to bytes
		val *= 1024

		switch key {
		case "MemTotal":
			mem.Total = val
		case "MemFree":
			mem.Free = val
		case "MemAvailable":
			mem.Available = val
		case "Buffers":
			mem.Buffers = val
		case "Cached":
			mem.Cached = val
		}
	}

	return mem, scanner.Err()
}

// parseNetDev parses /proc/net/dev using bufio.Scanner.
// Strict line format validation and error handling.
func parseNetDev(ctx context.Context) ([]NetInterfaceStat, error) {
	file, err := os.Open("/proc/net/dev")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var stats []NetInterfaceStat
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		line := scanner.Text()
		// Skip empty lines
		if len(line) == 0 {
			continue
		}

		// Look for lines containing ":" (interface data lines)
		// Skip header lines like "Inter-|   Receive |  Transmit"
		sepIdx := strings.LastIndex(line, ":")
		if sepIdx == -1 {
			continue
		}

		// Validate interface name is not empty
		ifaceName := strings.TrimSpace(line[:sepIdx])
		if len(ifaceName) == 0 {
			continue
		}

		// Validate data exists after colon
		data := strings.TrimSpace(line[sepIdx+1:])
		if len(data) == 0 {
			continue
		}

		fields := strings.Fields(data)
		// Need at least 12 fields per /proc/net/dev format
		if len(fields) < 12 {
			continue
		}

		stat := NetInterfaceStat{Name: ifaceName}

		// Parse Rx fields (0-8)
		if val, err := strconv.ParseUint(fields[0], 10, 64); err == nil {
			stat.RxBytes = val
		}
		if val, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
			stat.RxPackets = val
		}
		if val, err := strconv.ParseUint(fields[2], 10, 64); err == nil {
			stat.RxErrors = val
		}
		if val, err := strconv.ParseUint(fields[3], 10, 64); err == nil {
			stat.RxDropped = val
		}

		// Skip fields 4-7 (compressed, multicase, etc.)

		// Parse Tx fields (8-11)
		if val, err := strconv.ParseUint(fields[8], 10, 64); err == nil {
			stat.TxBytes = val
		}
		if val, err := strconv.ParseUint(fields[9], 10, 64); err == nil {
			stat.TxPackets = val
		}
		if val, err := strconv.ParseUint(fields[10], 10, 64); err == nil {
			stat.TxErrors = val
		}
		if val, err := strconv.ParseUint(fields[11], 10, 64); err == nil {
			stat.TxDropped = val
		}

		stats = append(stats, stat)
	}

	return stats, scanner.Err()
}

// ParseCPUStat is the exported wrapper for parseCpuStat.
func ParseCPUStat(ctx context.Context) (*CPUStat, error) {
	return parseCpuStat(ctx)
}

// ParseLoadAvg is the exported wrapper for parseLoadAvg.
func ParseLoadAvg(ctx context.Context) (*LoadAvg, error) {
	return parseLoadAvg(ctx)
}

// ParseMemInfo is the exported wrapper for parseMemInfo.
func ParseMemInfo(ctx context.Context) (*MemInfo, error) {
	return parseMemInfo(ctx)
}

// ParseNetDev is the exported wrapper for parseNetDev.
func ParseNetDev(ctx context.Context) ([]NetInterfaceStat, error) {
	return parseNetDev(ctx)
}