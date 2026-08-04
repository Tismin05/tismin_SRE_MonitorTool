package collector

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"
	"time"

	"tisminSRETool/internal/model"
)

func TestParseNetSnapshotLine(t *testing.T) {
	now := time.Unix(123, 0)
	snapshot, err := parseNetSnapshotLine(" eth0: 100 10 2 3 0 0 0 0 200 20 4 5 0 0 0 0", now)
	if err != nil {
		t.Fatalf("parse valid network line: %v", err)
	}
	if snapshot.iface != "eth0" || snapshot.timestamp != now ||
		snapshot.rxBytes != 100 || snapshot.rxPackets != 10 ||
		snapshot.rxErrors != 2 || snapshot.rxDrops != 3 ||
		snapshot.txBytes != 200 || snapshot.txPackets != 20 ||
		snapshot.txErrors != 4 || snapshot.txDrops != 5 {
		t.Fatalf("unexpected network snapshot: %#v", snapshot)
	}
}

func TestParseNetSnapshotLineReportsFieldContext(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "invalid", raw: "bad"},
		{name: "overflow", raw: "18446744073709551616"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := " eth0: " + tt.raw + " 10 2 3 0 0 0 0 200 20 4 5 0 0 0 0"
			_, err := parseNetSnapshotLine(line, time.Time{})
			if err == nil {
				t.Fatal("expected parse error")
			}
			message := err.Error()
			for _, want := range []string{"eth0", "rx_bytes", tt.raw} {
				if !strings.Contains(message, want) {
					t.Fatalf("error %q does not contain %q", message, want)
				}
			}
		})
	}
}

func TestParseNetSnapshotLinesKeepsValidInterfaces(t *testing.T) {
	lines := []string{
		" eth0: bad 10 2 3 0 0 0 0 200 20 4 5 0 0 0 0",
		" eth1: 300 30 0 0 0 0 0 0 400 40 0 0 0 0 0 0",
	}
	snapshots, err := parseNetSnapshotLines(context.Background(), lines, time.Now())
	if err == nil {
		t.Fatal("expected aggregate network parse error")
	}
	if len(snapshots) != 1 || snapshots[0].iface != "eth1" {
		t.Fatalf("valid interface was not preserved: %#v", snapshots)
	}

	allInvalid, err := parseNetSnapshotLines(context.Background(), lines[:1], time.Now())
	if err == nil || len(allInvalid) != 0 {
		t.Fatalf("all-invalid result = %#v, %v; want no snapshots and an error", allInvalid, err)
	}
}

func TestParseNetSnapshotLinesRejectsEmptyInput(t *testing.T) {
	snapshots, err := parseNetSnapshotLines(context.Background(), nil, time.Now())
	if err == nil || len(snapshots) != 0 {
		t.Fatalf("empty network input = %#v, %v; want no snapshots and an error", snapshots, err)
	}
}

func TestNetSamplerDoesNotUpdateBufferOnPartialParseError(t *testing.T) {
	sampler := &netSampler{}
	original := []netSnapshot{{iface: "existing", rxBytes: 7}}
	sampler.buffer.snapshots[0] = append([]netSnapshot(nil), original...)
	sampler.collectWithReader(context.Background(), func(context.Context) ([]netSnapshot, error) {
		return []netSnapshot{{iface: "eth0", rxBytes: 99}}, errors.New("invalid rx_bytes")
	})

	if sampler.buffer.index != 0 || len(sampler.buffer.snapshots[0]) != 1 ||
		sampler.buffer.snapshots[0][0].iface != "existing" ||
		len(sampler.buffer.snapshots[1]) != 0 {
		t.Fatalf("failed sample changed ring buffer: index=%d snapshots=%#v", sampler.buffer.index, sampler.buffer.snapshots)
	}
}

func TestNetSamplerDoesNotWarnForContextCancellation(t *testing.T) {
	var output bytes.Buffer
	previousWriter := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previousWriter) })

	sampler := &netSampler{}
	sampler.collectWithReader(context.Background(), func(context.Context) ([]netSnapshot, error) {
		return nil, context.Canceled
	})
	if output.Len() != 0 {
		t.Fatalf("context cancellation produced warning log: %q", output.String())
	}
}

func TestParseDiskStatsLine(t *testing.T) {
	stat, err := parseDiskStatsLine("8 0 sda 10 0 20 0 30 0 40 0 0 50 0")
	if err != nil {
		t.Fatalf("parse valid diskstats line: %v", err)
	}
	want := DiskIOStat{Name: "sda", ReadIOs: 10, ReadSectors: 20, WriteIOs: 30, WriteSectors: 40, IOQueuesTime: 50}
	if stat != want {
		t.Fatalf("disk stat = %#v, want %#v", stat, want)
	}
}

func TestParseDiskStatsLineRejectsInvalidFields(t *testing.T) {
	tests := []struct {
		name  string
		index int
	}{
		{name: "read_ios", index: 3},
		{name: "read_sectors", index: 5},
		{name: "write_ios", index: 7},
		{name: "write_sectors", index: 9},
		{name: "io_queue_time", index: 12},
	}
	base := strings.Fields("8 0 sda 10 0 20 0 30 0 40 0 0 50 0")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fields := append([]string(nil), base...)
			fields[tt.index] = "bad"
			_, err := parseDiskStatsLine(strings.Join(fields, " "))
			if err == nil {
				t.Fatal("expected parse error")
			}
			message := err.Error()
			for _, want := range []string{"sda", tt.name, "bad"} {
				if !strings.Contains(message, want) {
					t.Fatalf("error %q does not contain %q", message, want)
				}
			}
		})
	}

	if _, err := parseDiskStatsLine("8 0 sda 10"); err == nil {
		t.Fatal("expected short diskstats line to fail")
	}
	fields := append([]string(nil), base...)
	fields[3] = "18446744073709551616"
	if _, err := parseDiskStatsLine(strings.Join(fields, " ")); err == nil {
		t.Fatal("expected uint64 overflow to fail")
	}
}

func TestParseDiskStatsLinesKeepsValidDevices(t *testing.T) {
	lines := []string{
		"8 0 sda bad 0 20 0 30 0 40 0 0 50 0",
		"8 16 sdb 11 0 21 0 31 0 41 0 0 51 0",
	}
	stats, err := parseDiskStatsLines(context.Background(), lines)
	if err == nil {
		t.Fatal("expected aggregate disk parse error")
	}
	if len(stats) != 1 || stats["sdb"].ReadIOs != 11 {
		t.Fatalf("valid device was not preserved: %#v", stats)
	}
	if _, exists := stats["sda"]; exists {
		t.Fatalf("invalid device produced a zero-value stat: %#v", stats["sda"])
	}

	allInvalid, err := parseDiskStatsLines(context.Background(), lines[:1])
	if err == nil || len(allInvalid) != 0 {
		t.Fatalf("all-invalid result = %#v, %v; want no stats and an error", allInvalid, err)
	}
}

func TestParseDiskStatsLinesRejectsEmptyOrFilteredInput(t *testing.T) {
	for name, lines := range map[string][]string{
		"empty":    nil,
		"filtered": {"7 0 loop0 10 0 20 0 30 0 40 0 0 50 0"},
	} {
		t.Run(name, func(t *testing.T) {
			stats, err := parseDiskStatsLines(context.Background(), lines)
			if err == nil || len(stats) != 0 {
				t.Fatalf("invalid disk input = %#v, %v; want no stats and an error", stats, err)
			}
		})
	}
}

func TestDiskSamplerDoesNotAdvanceBaselineOnPartialFailure(t *testing.T) {
	sampler := &diskSampler{}
	current := map[string]DiskIOStat{
		"sda": {Name: "sda", ReadIOs: 10, IOQueuesTime: 100},
	}
	var readErr error
	now := time.Unix(100, 0)
	readMountsFn := func(context.Context) (map[string]string, error) {
		return map[string]string{"sda": "/data"}, nil
	}
	readStatsFn := func(context.Context) (map[string]DiskIOStat, error) {
		return cloneDiskIOStats(current), readErr
	}
	statFSFn := func(string) (uint64, uint64, uint64, uint64, uint64, error) {
		return 1000, 400, 300, 100, 40, nil
	}
	nowFn := func() time.Time { return now }

	if _, err := collectDiskWithDeps(context.Background(), sampler, readMountsFn, readStatsFn, statFSFn, nowFn); err != nil {
		t.Fatalf("initial disk collection: %v", err)
	}

	now = now.Add(time.Second)
	current["sda"] = DiskIOStat{Name: "sda", ReadIOs: 20, IOQueuesTime: 180}
	readErr = errors.New("another device was malformed")
	partial, err := collectDiskWithDeps(context.Background(), sampler, readMountsFn, readStatsFn, statFSFn, nowFn)
	if err == nil || len(partial) != 1 {
		t.Fatalf("partial disk collection = %#v, %v; want data and error", partial, err)
	}
	_, sampledAt := sampler.previous()
	if !sampledAt.Equal(time.Unix(100, 0)) {
		t.Fatalf("partial failure advanced baseline to %v", sampledAt)
	}

	now = now.Add(time.Second)
	current["sda"] = DiskIOStat{Name: "sda", ReadIOs: 30, IOQueuesTime: 220}
	readErr = nil
	complete, err := collectDiskWithDeps(context.Background(), sampler, readMountsFn, readStatsFn, statFSFn, nowFn)
	if err != nil || len(complete) != 1 {
		t.Fatalf("complete disk collection = %#v, %v", complete, err)
	}
	if complete[0].Await != 6 || complete[0].Util != 6 {
		t.Fatalf("rates used failed baseline: await=%v util=%v, want 6 and 6", complete[0].Await, complete[0].Util)
	}
}

func TestDiskSamplerDoesNotAdvanceBaselineOnStatFSError(t *testing.T) {
	sampler := &diskSampler{}
	initial := time.Unix(200, 0)
	sampler.commit(map[string]DiskIOStat{"sda": {Name: "sda", ReadIOs: 10}}, initial)

	metrics, err := collectDiskWithDeps(
		context.Background(),
		sampler,
		func(context.Context) (map[string]string, error) {
			return map[string]string{"sda": "/broken"}, nil
		},
		func(context.Context) (map[string]DiskIOStat, error) {
			return map[string]DiskIOStat{"sda": {Name: "sda", ReadIOs: 20}}, nil
		},
		func(string) (uint64, uint64, uint64, uint64, uint64, error) {
			return 0, 0, 0, 0, 0, errors.New("I/O error")
		},
		func() time.Time { return initial.Add(time.Second) },
	)
	if err == nil || len(metrics) != 0 || !strings.Contains(err.Error(), `device "sda"`) || !strings.Contains(err.Error(), `mount "/broken"`) {
		t.Fatalf("statfs failure = %#v, %v; want contextual error and no metrics", metrics, err)
	}
	_, sampledAt := sampler.previous()
	if !sampledAt.Equal(initial) {
		t.Fatalf("statfs failure advanced baseline to %v", sampledAt)
	}
}

func TestDiskSamplerInstancesAreIsolated(t *testing.T) {
	first := &diskSampler{}
	second := &diskSampler{}
	first.commit(map[string]DiskIOStat{"sda": {Name: "sda", ReadIOs: 10}}, time.Unix(300, 0))

	stats, sampledAt := second.previous()
	if len(stats) != 0 || !sampledAt.IsZero() {
		t.Fatalf("second sampler observed first baseline: %#v at %v", stats, sampledAt)
	}
}

func TestLinuxCollectorPreservesPartialDataWithErrors(t *testing.T) {
	parseErr := errors.New("one device failed")
	c := &LinuxCollector{
		collectCPU: func(context.Context, *cpuSampler) (model.CPUStat, error) {
			return model.CPUStat{}, nil
		},
		collectMem: func(context.Context) (*model.MemoryStat, error) {
			return &model.MemoryStat{}, nil
		},
		collectDisk: func(context.Context, *diskSampler) ([]model.DiskStat, error) {
			return []model.DiskStat{{Device: "sdb", Read: 42}}, parseErr
		},
		collectNet: func(context.Context, *netSampler) ([]model.NetStat, error) {
			return []model.NetStat{{Name: "eth1", RxBytes: 24}}, parseErr
		},
	}

	metrics, collectErrs := c.Collect(context.Background())
	if len(metrics.Disk) != 1 || metrics.Disk[0].Device != "sdb" ||
		len(metrics.Net) != 1 || metrics.Net[0].Name != "eth1" {
		t.Fatalf("partial metrics were discarded: %#v", metrics)
	}
	if collectErrs == nil || len(collectErrs.Disk) != 1 || len(collectErrs.Net) != 1 ||
		!errors.Is(collectErrs.Disk[0], parseErr) || !errors.Is(collectErrs.Net[0], parseErr) {
		t.Fatalf("partial parse errors were not exposed: %#v", collectErrs)
	}
}
