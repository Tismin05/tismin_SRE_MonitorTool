package proc

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Mock /proc/net/tcp data for testing
const mockTCPData = `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:0035 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12345 0
   1: 0100007F:0036 0100007F:0036 01 00000000:00000000 00:00000000 00000000     0        0 12346 0
   2: 0100007F:0037 0100007F:0037 01 00000000:00000000 00:00000000 00000000     0        0 12347 0
   3: 0100007F:0038 0100007F:0038 06 00000000:00000000 00:00000000 00000000     0        0 12348 0
   4: 0100007F:0039 0100007F:0039 06 00000000:00000000 00:00000000 00000000     0        0 12349 0
   5: 0100007F:003A 0100007F:003A 08 00000000:00000000 00:00000000 00000000     0        0 12350 0`

// Mock /proc/net/tcp6 data
const mockTCP6Data = `  sl  local_address                         rem_address                       st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000000000000000000000000001:0035 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12345 0
   1: 00000000000000000000000000000001:0036 00000000000000000000000000000001:0036 01 00000000:00000000 00:00000000 00000000     0        0 12346 0`

// Mock /proc/net/snmp data
const mockSnmpData = `Ip: Forwarding DefaultTTL IcmpInMsgs IcmpInErrors IpInReceives IpInHdrErrors IpInAddrErrors IcmpOutMsgs IcmpOutErrors IcmpDestUnreachs IcmpTimeExcds IcmpParmProbs IcmpSrcQuenchs IcmpRedirects IcmpOutEchos IcmpOutEchoReps IpOutRequests IpOutDiscards IpOutNoRoutes IpReasmReqds IpReasmOKs IpReasmFails IpFragOKs IpFragFails IpFragCreates IcmpInCsumErrors IcmpInExtEchoReqs IcmpInExtEchoReps IcmpInExtMaskReqs IcmpInExtMaskRepls IcmpOutExtEchoReqs IcmpOutExtEchoReps IcmpOutExtMaskReqs IcmpOutExtMaskRepls
Tcp: RtoAlgorithm RtoMin RtoMax MaxConn ActiveOpens PassiveOpens AttemptFails EstabResets CurrEstab InSegs OutSegs RetransSegs TcpExt: SyncookiesSent SyncookiesRecv SyncookiesFailed SyncookiesFailed
Tcp: 2 100 200 1000 500 100 5 10 50 1000 2000 3000
Ip: 1 64 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0`

// Mock malformed TCP data
const malformedTCPData = `
   0:
   1: 0100007F:0035
   2: 0100007F:0035 0200007F:0035 GG
   3: 0100007F:0035 0200007F:0035 01 00000000:00000000 00:00000000 00000000
   4: garbage line
   5: 0100007F:0035 0200007F:0035 01 invalid extra : data here`

func TestParseTCPWithMockData(t *testing.T) {
	ctx := context.Background()

	// Create a temporary test by parsing a single line
	tests := []struct {
		name        string
		line        string
		wantState   TCPState
		wantPanic   bool
	}{
		{"ESTABLISHED", "0: 0100007F:0035 0200007F:0035 01 00000000:00000000 00:00000000 00000000 0 0 12345 0", TCPStateEstablished, false},
		{"LISTEN", "1: 00000000:0035 00000000:0000 0A 00000000:00000000 00:00000000 00000000 0 0 12345 0", TCPStateListen, false},
		{"TIME_WAIT", "2: 0100007F:0035 0200007F:0035 06 00000000:00000000 00:00000000 00000000 0 0 12345 0", TCPStateTimeWait, false},
		{"CLOSE_WAIT", "3: 0100007F:0035 0200007F:0035 08 00000000:00000000 00:00000000 00000000 0 0 12345 0", TCPStateCloseWait, false},
		{"Empty_line", "", 0, false},
		{"Only_spaces", "   ", 0, false},
		{"Too_few_fields", "0: 0100007F:0035", 0, false},
		{"Invalid_state", "0: 0100007F:0035 0200007F:0035 GG 00000000:00000000", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					if !tt.wantPanic {
						t.Errorf("parseTCPLine panicked when it should not have")
					}
				}
			}()

			result := parseTCPLine(tt.line, ctx)
			if tt.wantPanic {
				t.Error("Expected panic but did not occur")
			}
			if tt.wantState != 0 {
				count := result.States[tt.wantState]
				if count != 1 {
					t.Errorf("got state count %d, want 1", count)
				}
			}
		})
	}
}

// parseTCPLine parses a single TCP line without file I/O
func parseTCPLine(line string, ctx context.Context) TCPStat {
	states := make(map[TCPState]int)

	if len(strings.TrimSpace(line)) == 0 {
		return TCPStat{States: states}
	}

	fields := strings.Fields(line)
	if len(fields) < 4 {
		return TCPStat{States: states}
	}

	stateStr := fields[3]
	if len(stateStr) == 0 {
		return TCPStat{States: states}
	}

	state := parseTCPState(stateStr)
	states[state]++

	return TCPStat{States: states}
}

func TestParseTCPStateRecognition(t *testing.T) {
	// Test all hex values for TCP states
	stateTests := []struct {
		hex    string
		expect TCPState
	}{
		{"01", TCPStateEstablished},
		{"0x01", TCPStateEstablished},
		{"1", TCPStateEstablished},
		{"02", TCPStateSynSent},
		{"03", TCPStateSynRecv},
		{"04", TCPStateFinWait1},
		{"05", TCPStateFinWait2},
		{"06", TCPStateTimeWait},
		{"07", TCPStateClose},
		{"08", TCPStateCloseWait},
		{"09", TCPStateLastAck},
		{"0A", TCPStateListen},
		{"0a", TCPStateListen},
		{"0B", TCPStateClosing},
		{"0b", TCPStateClosing},
		{"FF", 0},       // Invalid
		{"", 0},         // Empty
		{"GG", 0},       // Invalid chars
		{" 01", TCPStateEstablished}, // Leading space is trimmed by parseTCPState
	}

	for _, tt := range stateTests {
		t.Run(tt.hex, func(t *testing.T) {
			result := parseTCPState(tt.hex)
			if result != tt.expect {
				t.Errorf("parseTCPState(%q) = %v, want %v", tt.hex, result, tt.expect)
			}
		})
	}
}

func TestParseSnmpWithMockData(t *testing.T) {
	tests := []struct {
		name            string
		line            string
		wantRetransSegs uint64
		wantOutSegs     uint64
		wantInSegs      uint64
	}{
		{
			name:            "Tcp_line_parsing",
			line:            "Tcp: 2 100 200 1000 500 100 5 10 50 1000 2000 3000",
			wantRetransSegs: 3000,
			wantOutSegs:     2000,
			wantInSegs:      1000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := parseSnmpLine(tt.line)
			if stats.RetransSegs != tt.wantRetransSegs {
				t.Errorf("RetransSegs = %d, want %d", stats.RetransSegs, tt.wantRetransSegs)
			}
			if stats.OutSegs != tt.wantOutSegs {
				t.Errorf("OutSegs = %d, want %d", stats.OutSegs, tt.wantOutSegs)
			}
			if stats.InSegs != tt.wantInSegs {
				t.Errorf("InSegs = %d, want %d", stats.InSegs, tt.wantInSegs)
			}
		})
	}
}

// parseSnmpLine parses a single SNMP line without file I/O
// Test data format: "Tcp: 2 100 200 1000 500 100 5 10 50 1000 2000 3000"
// Fields: 0=Tcp:, 1=RtoAlgo, 2=RtoMin, 3=RtoMax, 4=MaxConn, 5=ActiveOpens,
// 6=PassiveOpens, 7=AttemptFails, 8=EstabResets, 9=CurrEstab, 10=InSegs, 11=OutSegs, 12=RetransSegs
func parseSnmpLine(line string) *SnmpStats {
	stats := &SnmpStats{}

	if len(line) == 0 || !strings.HasPrefix(line, "Tcp:") {
		return stats
	}

	fields := strings.Fields(line)
	if len(fields) < 13 { // Need indices 0-12
		return stats
	}

	if v, err := parseUint64(fields, 10); err == nil {
		stats.InSegs = v
	}
	if v, err := parseUint64(fields, 11); err == nil {
		stats.OutSegs = v
	}
	if v, err := parseUint64(fields, 12); err == nil {
		stats.RetransSegs = v
	}

	return stats
}

func TestMalformedTCPData(t *testing.T) {
	lines := strings.Split(malformedTCPData, "\n")
	ctx := context.Background()

	for i, line := range lines {
		lineNum := i
		t.Run("Line_"+strconv.Itoa(i), func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Line %d: unexpected panic: %v", lineNum, r)
				}
			}()

			result := parseTCPLine(line, ctx)
			// Should not panic, should return valid (possibly empty) stat
			_ = result.States
		})
	}
}

func TestEmptyLinesTCP(t *testing.T) {
	ctx := context.Background()
	emptyLines := []string{"", "   ", "\t", "\n", "\r\n"}

	for i, line := range emptyLines {
		t.Run("EmptyLine_"+string(rune('0'+i)), func(t *testing.T) {
			result := parseTCPLine(line, ctx)
			total := 0
			for _, count := range result.States {
				total += count
			}
			if total != 0 {
				t.Errorf("Empty line %d should produce 0 connections, got %d", i, total)
			}
		})
	}
}

func TestSnmpCounterReset(t *testing.T) {
	// Simulate counter reset (e.g., after TCP stack restart)
	previous := &SnmpStats{
		RetransSegs: 10000,
		OutSegs:     50000,
	}
	current := &SnmpStats{
		RetransSegs: 100, // Counter reset to small value
		OutSegs:     1000,
	}

	rate := CalcRetransRate(current, previous)
	if rate != 0 {
		t.Errorf("Counter reset should return 0 rate, got %v", rate)
	}
}

func TestSnmpOverflow(t *testing.T) {
	// Simulate overflow (unlikely but possible)
	previous := &SnmpStats{
		RetransSegs: 0,
		OutSegs:     0,
	}
	current := &SnmpStats{
		RetransSegs: ^uint64(0), // Max uint64
		OutSegs:     100,
	}

	rate := CalcRetransRate(current, previous)
	// Should handle overflow gracefully
	if rate < 0 || rate > 100 {
		t.Errorf("Overflow should be capped, got %v", rate)
	}
}

func TestContextTimeoutTCP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	time.Sleep(10 * time.Millisecond)

	_, err := parseTCPStat(ctx)
	if err == nil {
		t.Error("Expected timeout error")
	}
}

func TestContextCancelTCP(t *testing.T) {
	if _, err := os.Stat("/proc/net/tcp"); os.IsNotExist(err) {
		t.Skip("Skipping test on non-Linux system")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := parseTCPStat(ctx)
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}

func TestTCPMultipleConnectionsPerState(t *testing.T) {
	// Parse multiple lines with same state
	lines := []string{
		"0: 0100007F:0035 0200007F:0035 01 00000000:00000000 00:00000000 00000000 0 0 12345 0",
		"1: 0100007F:0036 0200007F:0036 01 00000000:00000000 00:00000000 00000000 0 0 12346 0",
		"2: 0100007F:0037 0200007F:0037 01 00000000:00000000 00:00000000 00000000 0 0 12347 0",
		"3: 0100007F:0038 0200007F:0038 06 00000000:00000000 00:00000000 00000000 0 0 12348 0", // TIME_WAIT
		"4: 0100007F:0039 0200007F:0039 06 00000000:00000000 00:00000000 00000000 0 0 12349 0", // TIME_WAIT
	}

	ctx := context.Background()
	combined := TCPStat{States: make(map[TCPState]int)}

	for _, line := range lines {
		result := parseTCPLine(line, ctx)
		for state, count := range result.States {
			combined.States[state] += count
		}
	}

	if combined.States[TCPStateEstablished] != 3 {
		t.Errorf("ESTABLISHED count = %d, want 3", combined.States[TCPStateEstablished])
	}
	if combined.States[TCPStateTimeWait] != 2 {
		t.Errorf("TIME_WAIT count = %d, want 2", combined.States[TCPStateTimeWait])
	}
}

func TestParseNetDevLine(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		wantName  string
		wantRxErr uint64
	}{
		{"Valid_eth0", "eth0: 1234 5678 9 10 11 12 13 14 15 5678 9012 13 14 15", "eth0", 9},
		{"Valid_lo", "lo: 1234 5678 9 10 11 12 13 14 15 5678 9012 13 14 15", "lo", 9},
		{"Empty_name", ": 1234 5678 9 10 11 12 13 14 15 5678 9012 13 14 15", "", 0},
		{"No_colon", "eth0 1234 5678 9 10 11 12 13 14 15 5678 9012 13 14 15", "", 0},
		{"Too_few_fields", "eth0: 1234", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseNetDevLine(tt.line)
			if tt.wantName != "" && result.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", result.Name, tt.wantName)
			}
			if tt.wantRxErr != 0 && result.RxErrors != tt.wantRxErr {
				t.Errorf("RxErrors = %d, want %d", result.RxErrors, tt.wantRxErr)
			}
		})
	}
}

// parseNetDevLine parses a single /proc/net/dev line
func parseNetDevLine(line string) NetInterfaceStat {
	var stat NetInterfaceStat

	if len(line) == 0 {
		return stat
	}

	sepIdx := strings.LastIndex(line, ":")
	if sepIdx == -1 {
		return stat
	}

	ifaceName := strings.TrimSpace(line[:sepIdx])
	if len(ifaceName) == 0 {
		return stat
	}

	data := strings.TrimSpace(line[sepIdx+1:])
	if len(data) == 0 {
		return stat
	}

	fields := strings.Fields(data)
	if len(fields) < 12 {
		return stat
	}

	stat.Name = ifaceName

	if v, err := parseUint64(fields, 0); err == nil {
		stat.RxBytes = v
	}
	if v, err := parseUint64(fields, 2); err == nil {
		stat.RxErrors = v
	}
	if v, err := parseUint64(fields, 8); err == nil {
		stat.TxBytes = v
	}

	return stat
}

func TestNetDevInterfaceNameEdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		valid bool
	}{
		{"Normal", "eth0: 1234 5678 9 10 11 12 13 14 15 5678 9012 13 14 15", true},
		{"With_colons_in_name", "docker0: 1234 5678 9 10 11 12 13 14 15 5678 9012 13 14 15", true},
		{"Empty_name", ": 1234 5678 9 10 11 12 13 14 15 5678 9012 13 14 15", false},
		{"Spaces_before_colon", "eth0 : 1234 5678 9 10 11 12 13 14 15 5678 9012 13 14 15", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseNetDevLine(tt.line)
			if tt.valid && result.Name == "" {
				t.Errorf("Expected valid interface for line: %s", tt.line)
			}
		})
	}
}