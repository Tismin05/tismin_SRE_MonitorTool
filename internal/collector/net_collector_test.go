package collector

import (
	"testing"
	"time"
)

func TestCalcNetStatsPreservesCountersAndCalculatesSpeed(t *testing.T) {
	previous := []netSnapshot{{iface: "eth0", rxBytes: 1000, txBytes: 2000, rxPackets: 10, txPackets: 20}}
	current := []netSnapshot{{
		iface: "eth0", rxBytes: 1600, txBytes: 2600, rxPackets: 16, txPackets: 26,
		rxErrors: 2, txErrors: 3, rxDrops: 4, txDrops: 5,
	}}

	stats, err := calcNetStatsFromSnapshots(previous, current, 2)
	if err != nil {
		t.Fatalf("calcNetStatsFromSnapshots returned an error: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("got %d interfaces, want 1", len(stats))
	}
	got := stats[0]
	if got.RxBytes != 1600 || got.TxBytes != 2600 {
		t.Fatalf("counters were converted into rates: rx=%d tx=%d", got.RxBytes, got.TxBytes)
	}
	if got.RxSpeed != 300 || got.TxSpeed != 300 {
		t.Fatalf("unexpected speeds: rx=%v tx=%v", got.RxSpeed, got.TxSpeed)
	}
	if got.RxErrors != 2 || got.TxErrors != 3 || got.RxDropped != 4 || got.TxDropped != 5 {
		t.Fatalf("error/drop counters were not preserved: %#v", got)
	}
}

func TestNetSamplerReturnsChronologicalOrder(t *testing.T) {
	older := time.Now().Add(-time.Second)
	newer := time.Now()
	sampler := &netSampler{}

	sampler.buffer.snapshots[0] = []netSnapshot{{iface: "eth0", timestamp: newer, rxBytes: 20}}
	sampler.buffer.snapshots[1] = []netSnapshot{{iface: "eth0", timestamp: older, rxBytes: 10}}

	first, second, firstAt, secondAt := sampler.snapshots()
	if !firstAt.Equal(older) || !secondAt.Equal(newer) {
		t.Fatalf("timestamps are not chronological: %v then %v", firstAt, secondAt)
	}
	if first[0].rxBytes != 10 || second[0].rxBytes != 20 {
		t.Fatalf("snapshots were not reordered with their timestamps")
	}
}
