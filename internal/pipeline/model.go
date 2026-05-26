package pipeline

import (
	"time"
)

// MetricData is the core data structure for pipeline metrics.
type MetricData struct {
	CPU  CPUStat
	Mem  MemoryStat
	Net  []NetStat
	TCP  TCPStat
	Snmp SnmpStat
	Host string
	TS   time.Time
}

// CPUStat represents CPU metrics.
type CPUStat struct {
	Cores        int
	UsagePercent float64
	Load1        float64
	Load5        float64
	Load15       float64
}

// MemoryStat represents memory metrics.
type MemoryStat struct {
	Total       uint64
	Free        uint64
	Available   uint64
	Used        uint64
	UsedPercent float64
}

// NetStat represents network interface metrics.
type NetStat struct {
	Name      string
	RxBytes   uint64
	TxBytes   uint64
	RxSpeed   float64
	TxSpeed   float64
	RxPackets uint64
	TxPackets uint64
	RxErrors  uint64
	TxErrors  uint64
	RxDropped uint64
	TxDropped uint64
}

// TCPState represents TCP connection state.
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

// TCPStat represents TCP connection statistics.
type TCPStat struct {
	States map[TCPState]int
}

// SnmpStat represents SNMP metrics including retransmission data.
type SnmpStat struct {
	RetransSegs  uint64
	RetransRate  float64
	OutSegs      uint64
	InSegs       uint64
	RetransSegs0 uint64 // previous value for rate calculation
}

// Alert represents an alert triggered by threshold checking.
type Alert struct {
	Level   AlertLevel
	Message string
	Time    time.Time
}

// AlertLevel represents the severity of an alert.
type AlertLevel int

const (
	AlertLevelInfo AlertLevel = iota
	AlertLevelWarning
	AlertLevelCritical
)

// ThresholdConfig holds alert threshold configuration.
type ThresholdConfig struct {
	CPUUsagePercent      float64
	MemoryUsagePercent   float64
	TCPTimeWaitThreshold int
}