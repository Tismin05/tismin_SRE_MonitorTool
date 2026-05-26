package proc

import (
	"bufio"
	"context"
	"os"
	"strconv"
	"strings"
)

// SnmpStats represents parsed SNMP statistics.
type SnmpStats struct {
	RetransSegs  uint64
	RetransSegs0 uint64 // previous value for rate calculation
	OutSegs      uint64
	InSegs       uint64
}

// parseSnmp parses /proc/net/snmp using bufio.Scanner.
// Calculates TCP retransmission rate from RetransSegs counter.
// Returns zero values on any error - NEVER panics.
func parseSnmp(ctx context.Context) (SnmpStats, error) {
	file, err := os.Open("/proc/net/snmp")
	if err != nil {
		return SnmpStats{}, err
	}
	defer file.Close()

	var stats SnmpStats
	scanner := bufio.NewScanner(file)
	// Increase buffer for large files
	const maxScanTokenSize = 64 * 1024
	if bufio.MaxScanTokenSize < maxScanTokenSize {
		scanner.Buffer(make([]byte, 256), maxScanTokenSize)
	}

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return SnmpStats{}, err
		}

		line := scanner.Text()

		// Skip empty lines
		if len(line) == 0 {
			continue
		}

		// Look for Tcp: header line only
		if !strings.HasPrefix(line, "Tcp:") {
			continue
		}

		fields := strings.Fields(line)

		// Tcp: format has many fields, need at least 14
		// Fields: Tcp RtoAlgorithm RtoMin RtoMax MaxConn ActiveOpens PassiveOpens AttemptFails EstabResets CurrEstab InSegs OutSegs RetransSegs ...
		if len(fields) < 14 {
			continue
		}

		// RetransSegs is at index 13 (after Tcp and various Rto/MaxConn/Opens fields)
		// OutSegs is at index 12
		// InSegs is at index 11
		if val, err := strconv.ParseUint(fields[13], 10, 64); err == nil {
			stats.RetransSegs = val
		}
		if val, err := strconv.ParseUint(fields[12], 10, 64); err == nil {
			stats.OutSegs = val
		}
		if val, err := strconv.ParseUint(fields[11], 10, 64); err == nil {
			stats.InSegs = val
		}

		break // Found Tcp line, no need to continue
	}

	if err := scanner.Err(); err != nil {
		return SnmpStats{}, err
	}

	return stats, nil
}

// parseSnmpRaw parses /proc/net/snmp and returns raw counter values.
// This is the low-level parser that just extracts the counters.
func parseSnmpRaw(ctx context.Context) (*SnmpStats, error) {
	file, err := os.Open("/proc/net/snmp")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	stats := &SnmpStats{}
	scanner := bufio.NewScanner(file)
	const maxScanTokenSize = 64 * 1024
	if bufio.MaxScanTokenSize < maxScanTokenSize {
		scanner.Buffer(make([]byte, 256), maxScanTokenSize)
	}

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		line := scanner.Text()
		if len(line) == 0 || !strings.HasPrefix(line, "Tcp:") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 14 {
			return nil, nil
		}

		// Defensively parse each field
		if v, err := strconv.ParseUint(fields[11], 10, 64); err == nil {
			stats.InSegs = v
		}
		if v, err := strconv.ParseUint(fields[12], 10, 64); err == nil {
			stats.OutSegs = v
		}
		if v, err := strconv.ParseUint(fields[13], 10, 64); err == nil {
			stats.RetransSegs = v
		}

		return stats, nil
	}

	return nil, scanner.Err()
}

// calcRetransRate calculates TCP retransmission rate given current and previous stats.
// Formula: RetransRate = (RetransSegs - RetransSegs0) / OutSegs * 100
// Returns 0 if OutSegs is 0 or if delta is negative (counter reset).
func CalcRetransRate(current, previous *SnmpStats) float64 {
	if current == nil || previous == nil {
		return 0
	}

	// Guard against counter reset (current < previous)
	if current.RetransSegs < previous.RetransSegs {
		return 0
	}

	deltaRetrans := current.RetransSegs - previous.RetransSegs
	deltaOut := current.OutSegs - previous.OutSegs

	// Guard: if total outgoing segments is 0 or negative, no rate calculable
	if deltaOut == 0 {
		// If there were retransmits but no new outgoing segments, show 100%
		if deltaRetrans > 0 {
			return 100.0
		}
		return 0
	}

	if deltaOut < 0 {
		return 0
	}

	// Guard: prevent division by zero (should not happen given check above)
	retransRate := float64(deltaRetrans) / float64(deltaOut) * 100.0

	// Cap at 100%
	if retransRate > 100 {
		retransRate = 100
	}

	return retransRate
}

// ParseSnmp parses /proc/net/snmp and returns SnmpStats struct.
func ParseSnmp(ctx context.Context) (SnmpStats, error) {
	return parseSnmp(ctx)
}

// ParseSnmpRaw is the exported wrapper for parseSnmpRaw.
func ParseSnmpRaw(ctx context.Context) (*SnmpStats, error) {
	return parseSnmpRaw(ctx)
}