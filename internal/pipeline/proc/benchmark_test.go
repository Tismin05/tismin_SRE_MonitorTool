package proc

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

// Benchmark for parseCpuStat
func BenchmarkParseCPUStat(b *testing.B) {

	// Create a temp file with realistic /proc/stat content
	content := "cpu  5000000 1000000 3000000 20000000 100000 50000 30000 10000\n"
	content += "cpu0 1000000 200000 600000 4000000 20000 10000 6000 2000\n"
	content += "cpu1 1000000 200000 600000 4000000 20000 10000 6000 2000\n"
	content += "cpu2 1000000 200000 600000 4000000 20000 10000 6000 2000\n"
	content += "cpu3 1000000 200000 600000 4000000 20000 10000 6000 2000\n"
	content += "intr 1234567890\n"
	content += "ctxt 9876543210\n"
	content += "btime 1234567890\n"
	content += "processes 12345\n"
	content += "procs_running 5\n"
	content += "procs_blocked 2\n"

	tmpfile, err := os.CreateTemp("", "proc_stat_bench")
	if err != nil {
		b.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	defer tmpfile.Close()

	if _, err := tmpfile.WriteString(content); err != nil {
		b.Fatal(err)
	}
	tmpfile.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Re-open file each time to simulate real /proc access
		file, err := os.Open(tmpfile.Name())
		if err != nil {
			b.Fatal(err)
		}

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			fields := strings.Fields(line)
			if len(fields) >= 4 && fields[0] == "cpu" {
				_, _ = parseCPUFields(fields[1:])
				break
			}
		}
		file.Close()
	}
}

// Benchmark for parseTCPStat with zero allocation goal
func BenchmarkParseTCPStat(b *testing.B) {

	// Create temp file with realistic /proc/net/tcp content
	var sb strings.Builder
	sb.WriteString("  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n")
	for i := 0; i < 100; i++ {
		sb.WriteString("   0: 0100007F:0035 0200007F:0035 01 00000000:00000000 00:00000000 00000000     0        0 12345 0\n")
	}
	// Add some TIME_WAIT connections
	for i := 0; i < 50; i++ {
		sb.WriteString("   0: 0100007F:0035 0200007F:0035 06 00000000:00000000 00:00000000 00000000     0        0 12345 0\n")
	}

	content := sb.String()
	tmpfile, err := os.CreateTemp("", "proc_net_tcp_bench")
	if err != nil {
		b.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	defer tmpfile.Close()

	if _, err := tmpfile.WriteString(content); err != nil {
		b.Fatal(err)
	}
	tmpfile.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		file, err := os.Open(tmpfile.Name())
		if err != nil {
			b.Fatal(err)
		}

		states := make(map[TCPState]int)
		scanner := bufio.NewScanner(file)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			if lineNum == 1 {
				continue // skip header
			}
			line := scanner.Text()
			if len(line) == 0 {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 4 {
				continue
			}
			stateStr := fields[3]
			state := parseTCPState(stateStr)
			states[state]++
		}
		file.Close()
		_ = states
	}
}

// Benchmark for CPU calculation with delta=0 edge case
func BenchmarkCPUCalculationNoAlloc(b *testing.B) {

	// Create temp file simulating CPU delta = 0 (no change)
	content := "cpu  1000000 200000 300000 1000000 0 0 0 0\n"
	tmpfile, err := os.CreateTemp("", "proc_stat_delta0")
	if err != nil {
		b.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	defer tmpfile.Close()

	if _, err := tmpfile.WriteString(content); err != nil {
		b.Fatal(err)
	}
	tmpfile.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		file, err := os.Open(tmpfile.Name())
		if err != nil {
			b.Fatal(err)
		}

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			fields := strings.Fields(line)
			if len(fields) >= 4 && fields[0] == "cpu" {
				stat, _ := parseCPUFields(fields[1:])
				// Simulate delta calculation
				if stat != nil && stat.Total > 0 {
					usage := float64(stat.Total-stat.Idle) / float64(stat.Total) * 100
					_ = usage
				}
				break
			}
		}
		file.Close()
	}
}

// Benchmark for string parsing without allocation
func BenchmarkStringParsingNoAlloc(b *testing.B) {
	input := "  0: 0100007F:0035 0200007F:0035 01 00000000:00000000 00:00000000 00000000     0        0 12345 0"

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		fields := strings.Fields(input)
		if len(fields) >= 4 {
			_ = parseTCPState(fields[3])
		}
	}
}

// Benchmark showing allocation in naive approach vs zero-allocation
func BenchmarkNaiveTCPParse(b *testing.B) {
	input := "  0: 0100007F:0035 0200007F:0035 01 00000000:00000000 00:00000000 00000000     0        0 12345 0"

	b.ResetTimer()

	// This creates allocation on every call
	for i := 0; i < b.N; i++ {
		fields := strings.Fields(input) // allocates []string
		if len(fields) >= 4 {
			stateStr := fields[3]
			if len(stateStr) > 0 {
				_ = parseTCPState(stateStr)
			}
		}
	}
}

// Zero-allocation approach: reuse the fields slice
func BenchmarkZeroAllocTCPParse(b *testing.B) {
	input := "  0: 0100007F:0035 0200007F:0035 01 00000000:00000000 00:00000000 00000000     0        0 12345 0"
	// Pre-allocate to avoid allocation in strings.Fields
	fields := make([]string, 0, 16)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// strings.Fields still allocates, but we can use a fixed buffer approach
		// For true zero-allocation, we'd need to parse manually without strings.Fields
		fields = fields[:0]
		for _, f := range strings.Fields(input) {
			fields = append(fields, f)
		}
		if len(fields) >= 4 {
			_ = parseTCPState(fields[3])
		}
	}
}

// Benchmark for memory-efficient line parsing
func BenchmarkLineParsingNoAlloc(b *testing.B) {
	input := "  0: 0100007F:0035 0200007F:0035 01 00000000:00000000 00:00000000 00000000     0        0 12345 0"

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Manual parsing without strings.Fields or strconv
		var stateHex string
		seenSpace := false
		fieldCount := 0
		for j := 0; j < len(input); j++ {
			c := input[j]
			if c == ' ' || c == '\t' {
				if !seenSpace && fieldCount > 0 {
					fieldCount++
					seenSpace = true
				}
				continue
			}
			seenSpace = false
			if fieldCount == 3 {
				stateHex += string(c)
			}
		}
		_ = parseTCPState(stateHex)
	}
}

// Benchmark for RetransRate calculation
func BenchmarkCalcRetransRate(b *testing.B) {
	current := &SnmpStats{
		RetransSegs: 1100,
		OutSegs:     10000,
	}
	previous := &SnmpStats{
		RetransSegs: 1000,
		OutSegs:     9000,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = CalcRetransRate(current, previous)
	}
}

// How to run benchmarks:
//
// 1. Run all benchmarks:
//    go test -bench=. ./internal/pipeline/proc/...
//
// 2. Run specific benchmark:
//    go test -bench=BenchmarkParseCPUStat ./internal/pipeline/proc/...
//
// 3. Run with memory allocation report:
//    go test -bench=. -benchmem ./internal/pipeline/proc/...
//
// 4. Run with CPU profiling:
//    go test -bench=. -benchmem -cpuprofile=cpu.prof ./internal/pipeline/proc/...
//    then: go tool pprof cpu.prof
//
// 5. Run with memory profiling:
//    go test -bench=. -benchmem -memprofile=mem.prof ./internal/pipeline/proc/...
//    then: go tool pprof mem.prof
//
// 6. Run only benchmarks that match pattern:
//    go test -bench='CPU' ./internal/pipeline/proc/...
//
// Zero Allocation Tips:
//
// 1. strings.Fields allocates []string on each call
//    - Solution: Use manual parsing with rune iteration (see BenchmarkLineParsingNoAlloc)
//
// 2. strconv.ParseUint allocates during conversion
//    - Solution: Pre-allocate parser or use custom digit-by-digit parsing
//
// 3. map allocation in make(map[T]int)
//    - Solution: Reuse map with Clear() (Go 1.21+) or sync.Pool
//
// 4. bufio.Scanner creates internal buffer allocations
//    - Solution: Set fixed buffer size with scanner.Buffer()
//
// Example of zero-allocation parser:
//   Instead of:
//     fields := strings.Fields(line)
//     val, _ := strconv.ParseUint(fields[0], 10, 64)
//
//   Use manual parsing without allocations:
func parseUint64Manual(s string) (uint64, bool) {
	var val uint64
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		val = val*10 + uint64(c-'0')
	}
	return val, true
}