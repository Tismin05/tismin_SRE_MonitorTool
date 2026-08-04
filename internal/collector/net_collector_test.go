package collector

import (
	"context"
	"errors"
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

func TestCollectNetinfoFallsBackToDirectSnapshotRead(t *testing.T) {
	wantErr := errors.New("one malformed interface")
	readerCalled := false
	stats, err := collectNetinfoWithReader(context.Background(), &netSampler{}, func(context.Context) ([]netSnapshot, error) {
		readerCalled = true
		return []netSnapshot{{
			iface: "eth0", rxBytes: 1000, txBytes: 2000,
			rxPackets: 10, txPackets: 20, rxErrors: 1, txErrors: 2,
			rxDrops: 3, txDrops: 4,
		}}, wantErr
	})

	if !readerCalled {
		t.Fatal("collectNetinfo did not use the direct-read fallback")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("collectNetinfo error = %v, want %v", err, wantErr)
	}
	if len(stats) != 1 {
		t.Fatalf("collectNetinfo returned %d valid interfaces, want 1", len(stats))
	}
	got := stats[0]
	if got.Name != "eth0" || got.RxBytes != 1000 || got.TxBytes != 2000 ||
		got.RxPackets != 10 || got.TxPackets != 20 || got.RxErrors != 1 ||
		got.TxErrors != 2 || got.RxDropped != 3 || got.TxDropped != 4 {
		t.Fatalf("direct-read fallback did not preserve valid counters: %#v", got)
	}
}

func TestCollectNetinfoPrefersValidSamplerCache(t *testing.T) {
	older := time.Unix(100, 0)
	newer := older.Add(2 * time.Second)
	sampler := &netSampler{}
	sampler.buffer.snapshots[0] = []netSnapshot{{
		iface: "eth0", timestamp: older, rxBytes: 1000, txBytes: 2000,
	}}
	sampler.buffer.snapshots[1] = []netSnapshot{{
		iface: "eth0", timestamp: newer, rxBytes: 1600, txBytes: 2800,
	}}

	readerCalled := false
	stats, err := collectNetinfoWithReader(context.Background(), sampler, func(context.Context) ([]netSnapshot, error) {
		readerCalled = true
		return nil, errors.New("direct reader should not be called")
	})
	if err != nil {
		t.Fatalf("collectNetinfo returned an error from valid cache: %v", err)
	}
	if readerCalled {
		t.Fatal("collectNetinfo bypassed a valid sampler cache")
	}
	if len(stats) != 1 || stats[0].RxSpeed != 300 || stats[0].TxSpeed != 400 {
		t.Fatalf("unexpected cached network stats: %#v", stats)
	}
}

func TestCollectNetinfoFallsBackWhenSamplerWindowIsInvalid(t *testing.T) {
	at := time.Now()
	sampler := &netSampler{}
	sampler.buffer.snapshots[0] = []netSnapshot{{iface: "eth0", timestamp: at, rxBytes: 1000}}
	sampler.buffer.snapshots[1] = []netSnapshot{{iface: "eth0", timestamp: at, rxBytes: 2000}}

	readerCalled := false
	stats, err := collectNetinfoWithReader(context.Background(), sampler, func(context.Context) ([]netSnapshot, error) {
		readerCalled = true
		return []netSnapshot{{iface: "eth1", rxBytes: 3000}}, nil
	})
	if err != nil {
		t.Fatalf("collectNetinfo fallback returned an error: %v", err)
	}
	if !readerCalled {
		t.Fatal("collectNetinfo used a cache with a zero-duration sample window")
	}
	if len(stats) != 1 || stats[0].Name != "eth1" || stats[0].RxBytes != 3000 {
		t.Fatalf("unexpected fallback network stats: %#v", stats)
	}
}
