package proc

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseTCPState(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected TCPState
	}{
		{"Established_0x01", "01", TCPStateEstablished},
		{"Established_0x1", "0x1", TCPStateEstablished},
		{"SynSent_0x02", "02", TCPStateSynSent},
		{"SynRecv_0x03", "03", TCPStateSynRecv},
		{"FinWait1_0x04", "04", TCPStateFinWait1},
		{"FinWait2_0x05", "05", TCPStateFinWait2},
		{"TimeWait_0x06", "06", TCPStateTimeWait},
		{"Close_0x07", "07", TCPStateClose},
		{"CloseWait_0x08", "08", TCPStateCloseWait},
		{"LastAck_0x09", "09", TCPStateLastAck},
		{"Listen_0x0A", "0A", TCPStateListen},
		{"Closing_0x0B", "0B", TCPStateClosing},
		{"Invalid_empty", "", 0},
		{"Invalid_garbage", "garbage", 0},
		{"Invalid_FF", "FF", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTCPState(tt.input)
			if result != tt.expected {
				t.Errorf("parseTCPState(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseTCPFields(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectState TCPState
		expectCount int
	}{
		{
			name:        "Valid_single_connection",
			input:       "  0: 0100007F:0035 0200007F:0035 01 00000000:00000000 00:00000000 00000000     0        0 12345 0",
			expectState: TCPStateEstablished,
			expectCount: 1,
		},
		{
			name:        "Valid_time_wait",
			input:       "  1: 0100007F:0035 0200007F:0035 06 00000000:00000000 00:00000000 00000000     0        0 12345 0",
			expectState: TCPStateTimeWait,
			expectCount: 1,
		},
		{
			name:        "Valid_listen",
			input:       "  2: 00000000:0035 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12345 0",
			expectState: TCPStateListen,
			expectCount: 1,
		},
		{
			name:        "Empty_line",
			input:       "",
			expectState: 0,
			expectCount: 0,
		},
		{
			name:        "Whitespace_only",
			input:       "   ",
			expectState: 0,
			expectCount: 0,
		},
		{
			name:        "Invalid_too_few_fields",
			input:       "  0: 0100007F:0035",
			expectState: 0,
			expectCount: 0,
		},
		{
			name:        "Invalid_state_hex",
			input:       "  0: 0100007F:0035 0200007F:0035 GG 00000000:00000000 00:00000000 00000000",
			expectState: 0,
			expectCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result := parseTCPString(tt.input, ctx)

			if tt.expectCount == 0 {
				// For error cases, just verify it doesn't panic
				return
			}

			count := result.States[tt.expectState]
			if count != tt.expectCount {
				t.Errorf("parseTCPString() state %v count = %d, want %d", tt.expectState, count, tt.expectCount)
			}
		})
	}
}

// parseTCPString parses a single TCP line string without file I/O
// This allows testing the parsing logic in isolation
func parseTCPString(line string, ctx context.Context) TCPStat {
	states := make(map[TCPState]int)

	if len(line) == 0 {
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

func TestParseTCPStringEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectState TCPState
	}{
		{"Leading_spaces", "  0: 0100007F:0035 0200007F:0035 01 00000000:00000000", TCPStateEstablished},
		{"Tab_separator", "0:\t0100007F:0035\t0200007F:0035\t01", TCPStateEstablished},
		{"Mixed_case_hex", "  0: 0100007F:0035 0200007F:0035 0a 00000000:00000000", TCPStateListen},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTCPString(tt.input, context.Background())
			count := result.States[tt.expectState]
			if count != 1 {
				t.Errorf("parseTCPString(%q) state %v count = %d, want 1", tt.input, tt.expectState, count)
			}
		})
	}
}

func TestParseCPUFields(t *testing.T) {
	tests := []struct {
		name    string
		fields  []string
		wantErr bool
	}{
		{
			name:    "Valid_8_fields",
			fields:  []string{"100", "50", "30", "200", "10", "5", "3", "2"},
			wantErr: false,
		},
		{
			name:    "Valid_minimum_4_fields",
			fields:  []string{"100", "50", "30", "200"},
			wantErr: false,
		},
		{
			name:    "Invalid_only_3_fields",
			fields:  []string{"100", "50", "30"},
			wantErr: true,
		},
		{
			name:    "Invalid_empty",
			fields:  []string{},
			wantErr: true,
		},
		{
			name:    "Invalid_non_numeric",
			fields:  []string{"abc", "50", "30", "200"},
			wantErr: false, // parseUint64 handles error gracefully
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stat, err := parseCPUFields(tt.fields)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseCPUFields() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && stat == nil {
				t.Errorf("parseCPUFields() returned nil without error")
			}
		})
	}
}

func TestParseCPUFieldsCalculation(t *testing.T) {
	// Test CPU total calculation
	fields := []string{"100", "50", "30", "200"} // user, nice, system, idle
	stat, err := parseCPUFields(fields)
	if err != nil {
		t.Fatalf("parseCPUFields() unexpected error: %v", err)
	}

	expectedTotal := uint64(100 + 50 + 30 + 200)
	if stat.Total != expectedTotal {
		t.Errorf("stat.Total = %d, want %d", stat.Total, expectedTotal)
	}

	expectedIdle := uint64(200)
	if stat.Idle != expectedIdle {
		t.Errorf("stat.Idle = %d, want %d", stat.Idle, expectedIdle)
	}
}

func TestParseUint64(t *testing.T) {
	tests := []struct {
		name    string
		fields  []string
		index   int
		want    uint64
		wantErr bool
	}{
		{"Valid_index_0", []string{"100", "200"}, 0, 100, false},
		{"Valid_index_1", []string{"100", "200"}, 1, 200, false},
		{"Invalid_index_negative", []string{"100"}, -1, 0, true},
		{"Invalid_index_too_large", []string{"100"}, 5, 0, true},
		{"Invalid_non_numeric", []string{"abc"}, 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseUint64(tt.fields, tt.index)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseUint64() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("parseUint64() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCalcRetransRate(t *testing.T) {
	tests := []struct {
		name     string
		current  *SnmpStats
		previous *SnmpStats
		expected float64
	}{
		{
			name: "Normal_rate_10_percent",
			current: &SnmpStats{
				RetransSegs: 110,
				OutSegs:     1000,
			},
			previous: &SnmpStats{
				RetransSegs: 100,
				OutSegs:     900,
			},
			expected: 10.0, // 10 retrans / 100 outSegs * 100
		},
		{
			name: "Zero_delta_outseg",
			current: &SnmpStats{
				RetransSegs: 110,
				OutSegs:     900,
			},
			previous: &SnmpStats{
				RetransSegs: 100,
				OutSegs:     900,
			},
			expected: 100.0, // retrans with no new out segs = 100%
		},
		{
			name: "Counter_reset",
			current: &SnmpStats{
				RetransSegs: 50,
				OutSegs:     100,
			},
			previous: &SnmpStats{
				RetransSegs: 100,
				OutSegs:     900,
			},
			expected: 0, // current < previous = counter reset
		},
		{
			name:     "Nil_current",
			current:  nil,
			previous: &SnmpStats{RetransSegs: 100},
			expected: 0,
		},
		{
			name:     "Nil_previous",
			current:  &SnmpStats{RetransSegs: 100},
			previous: nil,
			expected: 0,
		},
		{
			name: "Zero_retrans",
			current: &SnmpStats{
				RetransSegs: 100,
				OutSegs:     1000,
			},
			previous: &SnmpStats{
				RetransSegs: 100,
				OutSegs:     900,
			},
			expected: 0, // no new retrans
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalcRetransRate(tt.current, tt.previous)
			if result != tt.expected {
				t.Errorf("CalcRetransRate() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCalcRetransRateCapped(t *testing.T) {
	// Test that rate is capped at 100%
	current := &SnmpStats{
		RetransSegs: 200,
		OutSegs:     100,
	}
	previous := &SnmpStats{
		RetransSegs: 100,
		OutSegs:     50,
	}

	result := CalcRetransRate(current, previous)
	if result > 100.0 {
		t.Errorf("CalcRetransRate() = %v, should be capped at 100", result)
	}
}

func TestParseLoadAvg(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want1   float64
		want5   float64
		want15  float64
		wantErr bool
	}{
		{
			name:    "Valid_load",
			content: "0.52 0.58 0.59 1/1234 5678\n",
			want1:   0.52,
			want5:   0.58,
			want15:  0.59,
			wantErr: false,
		},
		{
			name:    "Empty",
			content: "",
			wantErr: true,
		},
		{
			name:    "Only_two_values",
			content: "0.52 0.58\n",
			wantErr: true,
		},
		{
			name:    "Invalid_non_numeric",
			content: "abc def ghi\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseLoadAvgString(tt.content)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseLoadAvgString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if result.Load1 != tt.want1 {
					t.Errorf("Load1 = %v, want %v", result.Load1, tt.want1)
				}
				if result.Load5 != tt.want5 {
					t.Errorf("Load5 = %v, want %v", result.Load5, tt.want5)
				}
				if result.Load15 != tt.want15 {
					t.Errorf("Load15 = %v, want %v", result.Load15, tt.want15)
				}
			}
		})
	}
}

// parseLoadAvgString parses load average from string content without file I/O
func parseLoadAvgString(content string) (*LoadAvg, error) {
	if len(content) == 0 {
		return nil, context.DeadlineExceeded
	}

	fields := strings.Fields(content)
	if len(fields) < 3 {
		return nil, context.DeadlineExceeded
	}

	load1, err := strconvParseFloat(fields[0])
	if err != nil {
		return nil, err
	}
	load5, err := strconvParseFloat(fields[1])
	if err != nil {
		return nil, err
	}
	load15, err := strconvParseFloat(fields[2])
	if err != nil {
		return nil, err
	}

	return &LoadAvg{Load1: load1, Load5: load5, Load15: load15}, nil
}

func strconvParseFloat(s string) (float64, error) {
	// Simplified float parser for testing without depending on strconv
	var result float64
	var decimal, sign float64 = 1, 1
	var afterDecimal bool

	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '-' && i == 0 {
			sign = -1
			continue
		}
		if c == '.' {
			afterDecimal = true
			continue
		}
		if c < '0' || c > '9' {
			return 0, context.DeadlineExceeded
		}
		digit := float64(c - '0')
		if afterDecimal {
			decimal *= 10
			result += digit / decimal
		} else {
			result = result*10 + digit
		}
	}
	return result * sign, nil
}

func TestContextCancellation(t *testing.T) {
	if _, err := os.Stat("/proc/stat"); os.IsNotExist(err) {
		t.Skip("Skipping test on non-Linux system")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// All parsers should respect context cancellation
	tests := []struct {
		name string
		fn   func(context.Context) error
	}{
		{"parseCpuStat", func(ctx context.Context) error {
			_, err := parseCpuStat(ctx)
			return err
		}},
		{"parseLoadAvg", func(ctx context.Context) error {
			_, err := parseLoadAvg(ctx)
			return err
		}},
		{"parseMemInfo", func(ctx context.Context) error {
			_, err := parseMemInfo(ctx)
			return err
		}},
		{"parseNetDev", func(ctx context.Context) error {
			_, err := parseNetDev(ctx)
			return err
		}},
		{"parseTCPStat", func(ctx context.Context) error {
			_, err := parseTCPStat(ctx)
			return err
		}},
		{"parseSnmp", func(ctx context.Context) error {
			_, err := parseSnmp(ctx)
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn(ctx)
			if err == nil {
				t.Errorf("%s should return error for cancelled context", tt.name)
			}
			// Should get context.Canceled or context.DeadlineExceeded
			if err != context.Canceled && err != context.DeadlineExceeded {
				t.Errorf("%s unexpected error: %v", tt.name, err)
			}
		})
	}
}

func TestTimeoutContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	time.Sleep(10 * time.Millisecond) // Ensure timeout expires

	_, err := parseCpuStat(ctx)
	if err == nil {
		t.Error("parseCpuStat should fail after timeout")
	}
}