package proc

import (
	"bufio"
	"context"
	"os"
	"strconv"
	"strings"
)

// TCPState represents TCP connection state enum.
type TCPState int

const (
	TCPStateEstablished TCPState = iota + 1
	TCPStateSynSent
	TCPStateSynRecv
	TCPStateFinWait1
	TCPStateFinWait2
	TCPStateTimeWait
	TCPStateClose
	TCPStateCloseWait
	TCPStateLastAck
	TCPStateListen
	TCPStateClosing
)

// String returns the string representation of TCPState.
func (s TCPState) String() string {
	switch s {
	case TCPStateEstablished:
		return "ESTABLISHED"
	case TCPStateSynSent:
		return "SYN_SENT"
	case TCPStateSynRecv:
		return "SYN_RECV"
	case TCPStateFinWait1:
		return "FIN_WAIT1"
	case TCPStateFinWait2:
		return "FIN_WAIT2"
	case TCPStateTimeWait:
		return "TIME_WAIT"
	case TCPStateClose:
		return "CLOSE"
	case TCPStateCloseWait:
		return "CLOSE_WAIT"
	case TCPStateLastAck:
		return "LAST_ACK"
	case TCPStateListen:
		return "LISTEN"
	case TCPStateClosing:
		return "CLOSING"
	default:
		return "UNKNOWN"
	}
}

// TCPStat represents TCP connection statistics by state.
type TCPStat struct {
	States map[TCPState]int
}

// parseTCPState converts hex state value to TCPState enum.
// Invalid values return 0 (UNKNOWN).
func parseTCPState(stateHex string) TCPState {
	// Remove leading/trailing spaces
	stateHex = strings.TrimSpace(stateHex)
	if len(stateHex) == 0 {
		return 0
	}

	// Convert to uppercase for consistent hex parsing (0a, 0A -> 0A)
	stateHex = strings.ToUpper(stateHex)

	// Remove optional 0x prefix if present
	if strings.HasPrefix(stateHex, "0X") {
		stateHex = stateHex[2:]
	}

	// Parse as hex (base 16) since TCP states are always hex
	val, err := strconv.ParseUint(stateHex, 16, 32)
	if err != nil {
		return 0
	}

	state := int(val)
	switch state {
	case 0x01:
		return TCPStateEstablished
	case 0x02:
		return TCPStateSynSent
	case 0x03:
		return TCPStateSynRecv
	case 0x04:
		return TCPStateFinWait1
	case 0x05:
		return TCPStateFinWait2
	case 0x06:
		return TCPStateTimeWait
	case 0x07:
		return TCPStateClose
	case 0x08:
		return TCPStateCloseWait
	case 0x09:
		return TCPStateLastAck
	case 0x0A:
		return TCPStateListen
	case 0x0B:
		return TCPStateClosing
	default:
		return 0
	}
}

// parseTCPStat parses /proc/net/tcp using bufio.Scanner.
// Returns connection state counts - NEVER panics.
func parseTCPStat(ctx context.Context) (TCPStat, error) {
	file, err := os.Open("/proc/net/tcp")
	if err != nil {
		return TCPStat{}, err
	}
	defer file.Close()

	states := make(map[TCPState]int)
	scanner := bufio.NewScanner(file)
	// Increase buffer for large connection tables
	const maxScanTokenSize = 64 * 1024
	if bufio.MaxScanTokenSize < maxScanTokenSize {
		scanner.Buffer(make([]byte, 256), maxScanTokenSize)
	}

	lineNum := 0
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return TCPStat{}, err
		}

		lineNum++
		line := scanner.Text()

		// Skip header line (first line)
		if lineNum == 1 {
			continue
		}

		// Skip empty lines
		if len(line) == 0 {
			continue
		}

		fields := strings.Fields(line)

		// /proc/net/tcp format: sl local_address rem_address st tx_queue rx_queue ...
		// Minimum 4 fields: local_address, rem_address, st, tx_queue
		if len(fields) < 4 {
			continue
		}

		// State is the 4th field (index 3)
		stateStr := fields[3]
		if len(stateStr) == 0 {
			continue
		}

		state := parseTCPState(stateStr)
		states[state]++
	}

	if err := scanner.Err(); err != nil {
		return TCPStat{}, err
	}

	return TCPStat{States: states}, nil
}

// parseTCP6Stat parses /proc/net/tcp6 using bufio.Scanner.
// IPv6 variant of parseTCPStat.
func parseTCP6Stat(ctx context.Context) (TCPStat, error) {
	file, err := os.Open("/proc/net/tcp6")
	if err != nil {
		return TCPStat{}, err
	}
	defer file.Close()

	states := make(map[TCPState]int)
	scanner := bufio.NewScanner(file)
	const maxScanTokenSize = 64 * 1024
	if bufio.MaxScanTokenSize < maxScanTokenSize {
		scanner.Buffer(make([]byte, 256), maxScanTokenSize)
	}

	lineNum := 0
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return TCPStat{}, err
		}

		lineNum++
		line := scanner.Text()

		if lineNum == 1 {
			continue
		}

		if len(line) == 0 {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		stateStr := fields[3]
		if len(stateStr) == 0 {
			continue
		}

		state := parseTCPState(stateStr)
		states[state]++
	}

	if err := scanner.Err(); err != nil {
		return TCPStat{}, err
	}

	return TCPStat{States: states}, nil
}

// ParseTCPStat is the exported wrapper for parseTCPStat.
func ParseTCPStat(ctx context.Context) (TCPStat, error) {
	return parseTCPStat(ctx)
}

// ParseTCP6Stat is the exported wrapper for parseTCP6Stat.
func ParseTCP6Stat(ctx context.Context) (TCPStat, error) {
	return parseTCP6Stat(ctx)
}